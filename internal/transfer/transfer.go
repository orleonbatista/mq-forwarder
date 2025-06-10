package transfer

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"mq-transfer-go/internal/mqutils"
	"mq-transfer-go/internal/otelutils"
)

// Predefined transfer statuses to avoid typos and allow consistent checks.
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCancelled  = "cancelled"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// TransferOptions defines parameters for a transfer operation.
type TransferOptions struct {
	SourceConfig        mqutils.MQConnectionConfig
	SourceQueue         string
	DestConfig          mqutils.MQConnectionConfig
	DestQueue           string
	BufferSize          int
	CommitInterval      int
	NonSharedConnection bool
	WorkerCount         int
}

// Stats holds runtime statistics of a transfer.
type Stats struct {
	MessagesTransferred int64
	BytesTransferred    int64
	Status              string
	EndTime             time.Time
	Error               string
}

// TransferManager performs message transfer. This is a minimal stub
// implementation to allow the application to compile and run tests.
type TransferManager struct {
	opts          TransferOptions
	mu            sync.RWMutex
	stats         Stats
	quit          chan struct{}
	done          chan struct{}
	srcMu         sync.Mutex
	destMu        sync.Mutex
	commitCounter int32
}

type mqMessage struct {
	data  []byte
	md    interface{}
	start time.Time
}

// NewTransferManager creates a new manager with the given options.
func NewTransferManager(opts TransferOptions) *TransferManager {
	if opts.CommitInterval <= 0 {
		opts.CommitInterval = 10
	}
	if opts.WorkerCount <= 0 {
		opts.WorkerCount = runtime.NumCPU()
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = 1024 * 1024
	}
	return &TransferManager{opts: opts, stats: Stats{Status: StatusPending}, quit: make(chan struct{}), done: make(chan struct{})}
}

// Start begins the transfer asynchronously.
func (tm *TransferManager) Start() {
	go tm.run()
}

func (tm *TransferManager) run() {
	tm.mu.Lock()
	tm.stats.Status = StatusInProgress
	tm.mu.Unlock()
	defer close(tm.done)

	metrics := otelutils.GetMetrics()
	baseCtx := context.Background()
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	srcConn := mqutils.NewMQConnection(tm.opts.SourceConfig)
	if err := srcConn.Connect(); err != nil {
		tm.finishWithError(StatusFailed, err)
		return
	}
	defer srcConn.Disconnect()

	destConn := mqutils.NewMQConnection(tm.opts.DestConfig)
	if err := destConn.Connect(); err != nil {
		tm.finishWithError(StatusFailed, err)
		return
	}
	defer destConn.Disconnect()

	destQ, err := destConn.OpenQueue(tm.opts.DestQueue, false, false)
	if err != nil {
		tm.finishWithError(StatusFailed, err)
		return
	}
	defer destConn.CloseQueue(destQ)

	srcQ, err := srcConn.OpenQueue(tm.opts.SourceQueue, true, tm.opts.NonSharedConnection)
	if err != nil {
		tm.finishWithError(StatusFailed, err)
		return
	}
	defer srcConn.CloseQueue(srcQ)

	msgCh := make(chan mqMessage, tm.opts.WorkerCount*tm.opts.CommitInterval)
	var wg sync.WaitGroup

	// Goroutine responsible for reading messages
	wg.Add(1)
	go func() {
		defer wg.Done()
		buffer := make([]byte, tm.opts.BufferSize)
		idle := 0
		for {
			select {
			case <-ctx.Done():
				close(msgCh)
				return
			default:
			}

			start := time.Now()
			tm.srcMu.Lock()
			data, md, err := srcConn.GetMessage(srcQ, buffer)
			tm.srcMu.Unlock()
			if err != nil {
				tm.finishWithError(StatusFailed, err)
				close(msgCh)
				cancel()
				return
			}
			if data == nil {
				idle++
				if idle >= 3 {
					tm.mu.Lock()
					tm.stats.Status = StatusCompleted
					tm.stats.EndTime = time.Now()
					tm.mu.Unlock()
					close(msgCh)
					cancel()
					return
				}
				time.Sleep(time.Second)
				continue
			}
			idle = 0

			cp := make([]byte, len(data))
			copy(cp, data)
			msgCh <- mqMessage{data: cp, md: md, start: start}
		}
	}()

	// Worker goroutines to put messages on the destination queue
	for i := 0; i < tm.opts.WorkerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-msgCh:
					if !ok {
						return
					}
					tm.destMu.Lock()
					err := destConn.PutMessage(destQ, msg.data, msg.md, "set")
					tm.destMu.Unlock()
					if err != nil {
						if tm.opts.CommitInterval > 0 {
							tm.srcMu.Lock()
							tm.destMu.Lock()
							_ = srcConn.Backout()
							_ = destConn.Backout()
							tm.destMu.Unlock()
							tm.srcMu.Unlock()
							atomic.StoreInt32(&tm.commitCounter, 0)
						}
						tm.finishWithError(StatusFailed, err)
						cancel()
						return
					}

					atomic.AddInt64(&tm.stats.MessagesTransferred, 1)
					atomic.AddInt64(&tm.stats.BytesTransferred, int64(len(msg.data)))

					if metrics != nil {
						metrics.MessagesTransferred.Add(baseCtx, 1)
						metrics.BytesTransferred.Add(baseCtx, int64(len(msg.data)))
						metrics.TransferDuration.Record(baseCtx, float64(time.Since(msg.start).Milliseconds()))
					}

					if tm.opts.CommitInterval > 0 && atomic.AddInt32(&tm.commitCounter, 1) >= int32(tm.opts.CommitInterval) {
						if err := tm.commitBatch(srcConn, destConn, baseCtx, metrics); err != nil {
							cancel()
							return
						}
						atomic.StoreInt32(&tm.commitCounter, 0)
					}
				}
			}
		}()
	}

	go func() {
		<-tm.quit
		cancel()
	}()

	wg.Wait()

	select {
	case <-tm.quit:
		tm.mu.Lock()
		if tm.stats.Status == StatusInProgress {
			tm.stats.Status = StatusCancelled
			tm.stats.EndTime = time.Now()
		}
		tm.mu.Unlock()
	default:
	}

	if tm.opts.CommitInterval > 0 && atomic.LoadInt32(&tm.commitCounter) > 0 && tm.stats.Status != StatusFailed {
		_ = tm.commitBatch(srcConn, destConn, baseCtx, metrics)
	}
}

func (tm *TransferManager) commitBatch(srcConn, destConn *mqutils.MQConnection, ctx context.Context, metrics *otelutils.MQMetrics) error {
	tm.destMu.Lock()
	tm.srcMu.Lock()
	if err := destConn.Commit(); err != nil {
		_ = srcConn.Backout()
		_ = destConn.Backout()
		tm.srcMu.Unlock()
		tm.destMu.Unlock()
		tm.finishWithError(StatusFailed, err)
		return err
	}
	if err := srcConn.Commit(); err != nil {
		tm.srcMu.Unlock()
		tm.destMu.Unlock()
		tm.finishWithError(StatusFailed, err)
		return err
	}
	tm.srcMu.Unlock()
	tm.destMu.Unlock()
	if metrics != nil {
		metrics.CommitCounter.Add(ctx, 1)
	}
	return nil
}

func (tm *TransferManager) finishWithError(status string, err error) {
	tm.mu.Lock()
	tm.stats.Status = status
	tm.stats.Error = err.Error()
	tm.stats.EndTime = time.Now()
	tm.mu.Unlock()
}

// Stop cancels the transfer.
func (tm *TransferManager) Stop() {
	close(tm.quit)
}

// GetStats returns a snapshot of the current stats.
func (tm *TransferManager) GetStats() Stats {
	tm.mu.RLock()
	stats := tm.stats
	tm.mu.RUnlock()
	stats.MessagesTransferred = atomic.LoadInt64(&tm.stats.MessagesTransferred)
	stats.BytesTransferred = atomic.LoadInt64(&tm.stats.BytesTransferred)
	return stats
}
