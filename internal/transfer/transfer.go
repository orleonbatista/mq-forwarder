package transfer

import (
	"context"
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
	opts  TransferOptions
	mu    sync.RWMutex
	stats Stats
	quit  chan struct{}
	done  chan struct{}
}

// NewTransferManager creates a new manager with the given options.
func NewTransferManager(opts TransferOptions) *TransferManager {
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
	ctx := context.Background()

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

	commitCounter := 0
	buffer := make([]byte, tm.opts.BufferSize)
	for {
		select {
		case <-tm.quit:
			tm.mu.Lock()
			tm.stats.Status = StatusCancelled
			tm.stats.EndTime = time.Now()
			tm.mu.Unlock()
			return
		default:
			start := time.Now()
			data, md, err := srcConn.GetMessage(srcQ, buffer, tm.opts.CommitInterval)
			if err != nil {
				tm.finishWithError(StatusFailed, err)
				return
			}
			if data == nil {
				tm.mu.Lock()
				tm.stats.Status = StatusCompleted
				tm.stats.EndTime = time.Now()
				tm.mu.Unlock()
				return
			}

			if err := destConn.PutMessage(destQ, data, md, tm.opts.CommitInterval, "set"); err != nil {
				if tm.opts.CommitInterval > 0 {
					// rollback the GET on error to avoid message loss
					_ = srcConn.Backout()
					_ = destConn.Backout()
				}
				tm.finishWithError(StatusFailed, err)
				return
			}

			atomic.AddInt64(&tm.stats.MessagesTransferred, 1)
			atomic.AddInt64(&tm.stats.BytesTransferred, int64(len(data)))

			if metrics != nil {
				metrics.MessagesTransferred.Add(ctx, 1)
				metrics.BytesTransferred.Add(ctx, int64(len(data)))
				metrics.TransferDuration.Record(ctx, float64(time.Since(start).Milliseconds()))
			}

			if tm.opts.CommitInterval > 0 {
				commitCounter++
				if commitCounter >= tm.opts.CommitInterval {
					// commit destination first so source is not lost on failure
					if err := destConn.Commit(); err != nil {
						if backErr := srcConn.Backout(); backErr != nil {
							_ = backErr
						}
						tm.finishWithError(StatusFailed, err)
						return
					}
					if err := srcConn.Commit(); err != nil {
						tm.finishWithError(StatusFailed, err)
						return
					}
					commitCounter = 0
					if metrics != nil {
						metrics.CommitCounter.Add(ctx, 1)
					}
				}
			}
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
