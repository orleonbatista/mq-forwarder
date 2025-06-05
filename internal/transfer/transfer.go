package transfer

import (
	"context"
	"sync"
	"time"

	"mq-transfer-go/internal/mqutils"
	"mq-transfer-go/internal/otelutils"
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
	return &TransferManager{opts: opts, stats: Stats{Status: "pending"}, quit: make(chan struct{}), done: make(chan struct{})}
}

// Start begins the transfer asynchronously.
func (tm *TransferManager) Start() {
	go tm.run()
}

func (tm *TransferManager) run() {
	tm.mu.Lock()
	tm.stats.Status = "in_progress"
	tm.mu.Unlock()
	defer close(tm.done)

	metrics := otelutils.GetMetrics()
	ctx := context.Background()

	srcConn := mqutils.NewMQConnection(tm.opts.SourceConfig)
	if err := srcConn.Connect(); err != nil {
		tm.finishWithError("failed", err)
		return
	}
	defer srcConn.Disconnect()

	destConn := mqutils.NewMQConnection(tm.opts.DestConfig)
	if err := destConn.Connect(); err != nil {
		tm.finishWithError("failed", err)
		return
	}
	defer destConn.Disconnect()

	srcQ, err := srcConn.OpenQueue(tm.opts.SourceQueue, true, tm.opts.NonSharedConnection)
	if err != nil {
		tm.finishWithError("failed", err)
		return
	}
	defer srcConn.CloseQueue(srcQ)

	destQ, err := destConn.OpenQueue(tm.opts.DestQueue, false, false)
	if err != nil {
		tm.finishWithError("failed", err)
		return
	}
	defer destConn.CloseQueue(destQ)

	commitCounter := 0
	for {
		select {
		case <-tm.quit:
			tm.mu.Lock()
			tm.stats.Status = "cancelled"
			tm.stats.EndTime = time.Now()
			tm.mu.Unlock()
			return
		default:
			start := time.Now()
			data, md, err := srcConn.GetMessage(srcQ, tm.opts.BufferSize, tm.opts.CommitInterval)
			if err != nil {
				tm.finishWithError("failed", err)
				return
			}
			if data == nil {
				tm.mu.Lock()
				tm.stats.Status = "completed"
				tm.stats.EndTime = time.Now()
				tm.mu.Unlock()
				return
			}

			if err := destConn.PutMessage(destQ, data, md, tm.opts.CommitInterval); err != nil {
				tm.finishWithError("failed", err)
				return
			}

			tm.mu.Lock()
			tm.stats.MessagesTransferred++
			tm.stats.BytesTransferred += int64(len(data))
			tm.mu.Unlock()

			if metrics != nil {
				metrics.MessagesTransferred.Add(ctx, 1)
				metrics.BytesTransferred.Add(ctx, int64(len(data)))
				metrics.TransferDuration.Record(ctx, float64(time.Since(start).Milliseconds()))
			}

			if tm.opts.CommitInterval > 0 {
				commitCounter++
				if commitCounter >= tm.opts.CommitInterval {
					if err := srcConn.Commit(); err != nil {
						tm.finishWithError("failed", err)
						return
					}
					if err := destConn.Commit(); err != nil {
						tm.finishWithError("failed", err)
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
	defer tm.mu.RUnlock()
	return tm.stats
}
