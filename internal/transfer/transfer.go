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
	opts       TransferOptions
	mu         sync.RWMutex
	stats      Stats
	quit       chan struct{}
	done       chan struct{}
	bufferPool sync.Pool
}

type workerResult struct {
	err error
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
	tm := &TransferManager{
		opts:  opts,
		stats: Stats{Status: StatusPending},
		quit:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	tm.bufferPool.New = func() interface{} { return make([]byte, opts.BufferSize) }
	return tm
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

	var wg sync.WaitGroup
	resultCh := make(chan workerResult, tm.opts.WorkerCount)

	for i := 0; i < tm.opts.WorkerCount; i++ {
		wg.Add(1)
		go tm.worker(ctx, cancel, &wg, resultCh, baseCtx, metrics)
	}

	go func() {
		<-tm.quit
		cancel()
	}()

	wg.Wait()
	close(resultCh)

	for r := range resultCh {
		if r.err != nil {
			tm.finishWithError(StatusFailed, r.err)
			return
		}
	}

	select {
	case <-tm.quit:
		tm.mu.Lock()
		if tm.stats.Status == StatusInProgress {
			tm.stats.Status = StatusCancelled
			tm.stats.EndTime = time.Now()
		}
		tm.mu.Unlock()
	default:
		tm.mu.Lock()
		if tm.stats.Status == StatusInProgress {
			tm.stats.Status = StatusCompleted
			tm.stats.EndTime = time.Now()
		}
		tm.mu.Unlock()
	}
}

func (tm *TransferManager) worker(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, resultCh chan<- workerResult, baseCtx context.Context, metrics *otelutils.MQMetrics) {
	defer wg.Done()

	srcConn := mqutils.NewMQConnection(tm.opts.SourceConfig)
	if err := srcConn.Connect(); err != nil {
		resultCh <- workerResult{err: err}
		cancel()
		return
	}
	defer srcConn.Disconnect()

	destConn := mqutils.NewMQConnection(tm.opts.DestConfig)
	if err := destConn.Connect(); err != nil {
		resultCh <- workerResult{err: err}
		cancel()
		return
	}
	defer destConn.Disconnect()

	destQ, err := destConn.OpenQueue(tm.opts.DestQueue, false, false)
	if err != nil {
		resultCh <- workerResult{err: err}
		cancel()
		return
	}
	defer destConn.CloseQueue(destQ)

	srcQ, err := srcConn.OpenQueue(tm.opts.SourceQueue, true, tm.opts.NonSharedConnection)
	if err != nil {
		resultCh <- workerResult{err: err}
		cancel()
		return
	}
	defer srcConn.CloseQueue(srcQ)

	buffer := make([]byte, tm.opts.BufferSize)
	idle := 0
	commitCounter := 0

	for {
		select {
		case <-ctx.Done():
			if tm.opts.CommitInterval > 0 && commitCounter > 0 {
				_ = destConn.Commit()
				_ = srcConn.Commit()
			}
			resultCh <- workerResult{}
			return
		default:
		}

		start := time.Now()
		data, md, err := srcConn.GetMessage(srcQ, buffer)
		if err != nil {
			resultCh <- workerResult{err: err}
			cancel()
			return
		}
		if data == nil {
			idle++
			if idle >= 3 {
				if tm.opts.CommitInterval > 0 && commitCounter > 0 {
					_ = destConn.Commit()
					_ = srcConn.Commit()
				}
				resultCh <- workerResult{}
				return
			}
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				if tm.opts.CommitInterval > 0 && commitCounter > 0 {
					_ = destConn.Commit()
					_ = srcConn.Commit()
				}
				resultCh <- workerResult{}
				return
			}
			continue
		}
		idle = 0

		bufCopy := tm.bufferPool.Get().([]byte)
		if cap(bufCopy) < len(data) {
			bufCopy = make([]byte, len(data))
		}
		cp := bufCopy[:len(data)]
		copy(cp, data)

		if err := destConn.PutMessage(destQ, cp, md, "set"); err != nil {
			_ = srcConn.Backout()
			_ = destConn.Backout()
			tm.bufferPool.Put(cp[:cap(cp)])
			resultCh <- workerResult{err: err}
			cancel()
			return
		}
		tm.bufferPool.Put(cp[:cap(cp)])

		commitCounter++
		atomic.AddInt64(&tm.stats.MessagesTransferred, 1)
		atomic.AddInt64(&tm.stats.BytesTransferred, int64(len(data)))

		if metrics != nil {
			metrics.MessagesTransferred.Add(baseCtx, 1)
			metrics.BytesTransferred.Add(baseCtx, int64(len(data)))
			metrics.TransferDuration.Record(baseCtx, float64(time.Since(start).Milliseconds()))
		}

		if tm.opts.CommitInterval > 0 && commitCounter >= tm.opts.CommitInterval {
			if err := destConn.Commit(); err != nil {
				_ = srcConn.Backout()
				_ = destConn.Backout()
				resultCh <- workerResult{err: err}
				cancel()
				return
			}
			if err := srcConn.Commit(); err != nil {
				resultCh <- workerResult{err: err}
				cancel()
				return
			}
			if metrics != nil {
				metrics.CommitCounter.Add(baseCtx, 1)
			}
			commitCounter = 0
		}
	}
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

// SetStatsForTest allows tests to set internal stats directly.
// It has no effect on production usage.
func (tm *TransferManager) SetStatsForTest(s Stats) {
	tm.mu.Lock()
	tm.stats = s
	tm.mu.Unlock()
}
