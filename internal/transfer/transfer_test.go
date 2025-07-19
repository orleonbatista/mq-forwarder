package transfer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mq-transfer-go/internal/mqutils"
)

func TestTransferCancel(t *testing.T) {
	tm := NewTransferManager(TransferOptions{})
	tm.Start()
	tm.Stop()
	<-tm.done
	stats := tm.GetStats()
	if stats.Status != StatusCancelled {
		t.Fatalf("expected %s, got %s", StatusCancelled, stats.Status)
	}
}

func TestFinishWithError(t *testing.T) {
	tm := NewTransferManager(TransferOptions{})
	tm.finishWithError(StatusFailed, fmt.Errorf("fail"))
	s := tm.GetStats()
	if s.Status != StatusFailed || s.Error != "fail" {
		t.Fatalf("unexpected stats: %+v", s)
	}
}

func TestGetStatsCounters(t *testing.T) {
	tm := NewTransferManager(TransferOptions{})
	atomic.AddInt64(&tm.stats.MessagesTransferred, 5)
	atomic.AddInt64(&tm.stats.BytesTransferred, 10)
	s := tm.GetStats()
	if s.MessagesTransferred != 5 || s.BytesTransferred != 10 {
		t.Fatalf("unexpected counts: %+v", s)
	}
}

func TestWorkerCancel(t *testing.T) {
	tm := NewTransferManager(TransferOptions{CommitInterval: 1, BufferSize: 1})
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ch := make(chan workerResult, 1)
	wg.Add(1)
	go tm.worker(ctx, cancel, &wg, ch, context.Background(), nil)
	time.Sleep(2 * time.Millisecond)
	cancel()
	wg.Wait()
}

func TestWorkerIdleExit(t *testing.T) {
	tm := NewTransferManager(TransferOptions{CommitInterval: 1, BufferSize: 1})
	mqutils.ReturnNilMessage = true
	defer func() { mqutils.ReturnNilMessage = false }()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ch := make(chan workerResult, 1)
	wg.Add(1)
	go tm.worker(ctx, cancel, &wg, ch, context.Background(), nil)
	wg.Wait()
}
