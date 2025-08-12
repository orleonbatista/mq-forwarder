package transfer

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"mq-forwarder-go/internal/mqutils"
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
// Start begins the transfer asynchronously using a background context.
func (tm *TransferManager) Start() {
	tm.StartContext(context.Background())
}

// StartContext begins the transfer using the provided parent context.
func (tm *TransferManager) StartContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	go tm.run(ctx)
}

func (tm *TransferManager) run(parent context.Context) {
	tm.mu.Lock()
	tm.stats.Status = StatusInProgress
	tm.mu.Unlock()
	defer close(tm.done)

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var wg sync.WaitGroup
	resultCh := make(chan workerResult, tm.opts.WorkerCount)

	for i := 0; i < tm.opts.WorkerCount; i++ {
		wg.Add(1)
		go tm.worker(ctx, cancel, &wg, resultCh)
	}

	go func() {
		select {
		case <-tm.quit:
			cancel()
		case <-parent.Done():
			cancel()
		}
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

func (tm *TransferManager) worker(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, resultCh chan<- workerResult) {
	defer wg.Done()
	srcConn, destConn, srcQ, destQ, err := tm.initConnections(cancel, resultCh)
	if err != nil {
		return
	}
	defer srcConn.Disconnect()
	defer destConn.Disconnect()
	defer destConn.CloseQueue(destQ)
	defer srcConn.CloseQueue(srcQ)
	buffer := make([]byte, tm.opts.BufferSize)
	idle := 0
	commitCounter := 0

	for {
		if tm.handleContextDone(ctx, commitCounter, destConn, srcConn, resultCh) {
			return
		}

		data, md, err := srcConn.GetMessage(srcQ, buffer)
		if err != nil {
			resultCh <- workerResult{err: err}
			cancel()
			return
		}
		if data == nil {
			if tm.handleIdle(ctx, &idle, &commitCounter, destConn, srcConn, resultCh) {
				return
			}
			continue
		}
		idle = 0

		if err := tm.handleMessage(destConn, destQ, srcConn, data, md, &commitCounter, resultCh, cancel); err != nil {
			return
		}
	}
}

func (tm *TransferManager) initConnections(cancel context.CancelFunc, resultCh chan<- workerResult) (*mqutils.MQConnection, *mqutils.MQConnection, struct{}, struct{}, error) {
	srcConn := mqutils.NewMQConnection(tm.opts.SourceConfig)
	if err := srcConn.Connect(); err != nil {
		resultCh <- workerResult{err: err}
		cancel()
		return nil, nil, struct{}{}, struct{}{}, err
	}
	destConn := mqutils.NewMQConnection(tm.opts.DestConfig)
	if err := destConn.Connect(); err != nil {
		resultCh <- workerResult{err: err}
		cancel()
		srcConn.Disconnect()
		return nil, nil, struct{}{}, struct{}{}, err
	}
	destQ, err := destConn.OpenQueue(tm.opts.DestQueue, false, false)
	if err != nil {
		resultCh <- workerResult{err: err}
		cancel()
		destConn.Disconnect()
		srcConn.Disconnect()
		return nil, nil, struct{}{}, struct{}{}, err
	}
	srcQ, err := srcConn.OpenQueue(tm.opts.SourceQueue, true, tm.opts.NonSharedConnection)
	if err != nil {
		resultCh <- workerResult{err: err}
		cancel()
		destConn.CloseQueue(destQ)
		destConn.Disconnect()
		srcConn.Disconnect()
		return nil, nil, struct{}{}, struct{}{}, err
	}
	return srcConn, destConn, srcQ, destQ, nil
}

func (tm *TransferManager) handleMessage(destConn *mqutils.MQConnection, destQ struct{}, srcConn *mqutils.MQConnection, data []byte, md interface{}, commitCounter *int, resultCh chan<- workerResult, cancel context.CancelFunc) error {
	bufCopy := tm.copyBuffer(data)
	if err := destConn.PutMessage(destQ, bufCopy, md, "set"); err != nil {
		_ = srcConn.Backout()
		_ = destConn.Backout()
		tm.bufferPool.Put(bufCopy[:cap(bufCopy)])
		resultCh <- workerResult{err: err}
		cancel()
		return err
	}
	tm.bufferPool.Put(bufCopy[:cap(bufCopy)])
	*commitCounter++
	atomic.AddInt64(&tm.stats.MessagesTransferred, 1)
	atomic.AddInt64(&tm.stats.BytesTransferred, int64(len(data)))
	if tm.commitIfNeeded(commitCounter, destConn, srcConn, resultCh, cancel) {
		// commitIfNeeded already handled error if any
	}
	return nil
}

func (tm *TransferManager) handleContextDone(ctx context.Context, commitCounter int, destConn, srcConn *mqutils.MQConnection, resultCh chan<- workerResult) bool {
	select {
	case <-ctx.Done():
		tm.commitRemaining(commitCounter, destConn, srcConn)
		resultCh <- workerResult{}
		return true
	default:
		return false
	}
}

func (tm *TransferManager) handleIdle(ctx context.Context, idle, commitCounter *int, destConn, srcConn *mqutils.MQConnection, resultCh chan<- workerResult) bool {
	*idle++
	if *idle >= 3 {
		tm.commitRemaining(*commitCounter, destConn, srcConn)
		resultCh <- workerResult{}
		return true
	}
	select {
	case <-time.After(time.Second):
		return false
	case <-ctx.Done():
		tm.commitRemaining(*commitCounter, destConn, srcConn)
		resultCh <- workerResult{}
		return true
	}
}

func (tm *TransferManager) commitRemaining(count int, destConn, srcConn *mqutils.MQConnection) {
	if tm.opts.CommitInterval > 0 && count > 0 {
		_ = destConn.Commit()
		_ = srcConn.Commit()
	}
}

func (tm *TransferManager) commitIfNeeded(count *int, destConn, srcConn *mqutils.MQConnection, resultCh chan<- workerResult, cancel context.CancelFunc) bool {
	if tm.opts.CommitInterval > 0 && *count >= tm.opts.CommitInterval {
		if err := destConn.Commit(); err != nil {
			_ = srcConn.Backout()
			_ = destConn.Backout()
			resultCh <- workerResult{err: err}
			cancel()
			return true
		}
		if err := srcConn.Commit(); err != nil {
			resultCh <- workerResult{err: err}
			cancel()
			return true
		}
		*count = 0
	}
	return false
}

func (tm *TransferManager) copyBuffer(data []byte) []byte {
	bufCopy := tm.bufferPool.Get().([]byte)
	if cap(bufCopy) < len(data) {
		bufCopy = make([]byte, len(data))
	}
	cp := bufCopy[:len(data)]
	copy(cp, data)
	return cp
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
