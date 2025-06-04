package transfer

import (
	"sync"
	"time"

	"mq-transfer-go/internal/mqutils"
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
}

// NewTransferManager creates a new manager with the given options.
func NewTransferManager(opts TransferOptions) *TransferManager {
	return &TransferManager{opts: opts, stats: Stats{Status: "pending"}, quit: make(chan struct{})}
}

// Start begins the transfer asynchronously. This stub just marks the transfer
// as completed after a short delay.
func (tm *TransferManager) Start() {
	go func() {
		tm.mu.Lock()
		tm.stats.Status = "in_progress"
		tm.mu.Unlock()

		select {
		case <-time.After(100 * time.Millisecond):
			tm.mu.Lock()
			tm.stats.Status = "completed"
			tm.stats.EndTime = time.Now()
			tm.mu.Unlock()
		case <-tm.quit:
			tm.mu.Lock()
			tm.stats.Status = "cancelled"
			tm.stats.EndTime = time.Now()
			tm.mu.Unlock()
		}
	}()
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
