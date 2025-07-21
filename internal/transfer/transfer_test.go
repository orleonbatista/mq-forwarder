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

func runWorker(opts TransferOptions) error {
	tm := NewTransferManager(opts)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ch := make(chan workerResult, 1)
	wg.Add(1)
	go tm.worker(ctx, cancel, &wg, ch, context.Background(), nil)
	time.Sleep(time.Millisecond)
	cancel()
	wg.Wait()
	return (<-ch).err
}

func runWorkerSeq(opts TransferOptions, msgs []bool) error {
	mqutils.Messages = msgs
	tm := NewTransferManager(opts)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ch := make(chan workerResult, 1)
	wg.Add(1)
	go tm.worker(ctx, cancel, &wg, ch, context.Background(), nil)
	wg.Wait()
	return (<-ch).err
}

func TestWorkerFailures(t *testing.T) {
	mqutils.ResetTestState()
	mqutils.FailConnectCall = 1
	if err := runWorker(TransferOptions{CommitInterval: 1, BufferSize: 1}); err == nil {
		t.Fatalf("expected connect fail")
	}
	mqutils.ResetTestState()
	mqutils.FailConnectCall = 2
	if err := runWorker(TransferOptions{CommitInterval: 1, BufferSize: 1}); err == nil {
		t.Fatalf("expected dest connect fail")
	}
	mqutils.ResetTestState()
	mqutils.FailOpenCall = 1
	if err := runWorker(TransferOptions{CommitInterval: 1, BufferSize: 1}); err == nil {
		t.Fatalf("expected open fail")
	}
	mqutils.ResetTestState()
	mqutils.FailOpenCall = 2
	if err := runWorker(TransferOptions{CommitInterval: 1, BufferSize: 1}); err == nil {
		t.Fatalf("expected src open fail")
	}
	mqutils.ResetTestState()
	mqutils.FailPut = true
	if err := runWorker(TransferOptions{CommitInterval: 1, BufferSize: 1}); err == nil {
		t.Fatalf("expected put fail")
	}
	mqutils.ResetTestState()
	mqutils.FailCommit = true
	if err := runWorker(TransferOptions{CommitInterval: 1, BufferSize: 1}); err == nil {
		t.Fatalf("expected commit fail")
	}
	mqutils.ResetTestState()
	mqutils.FailGet = true
	if err := runWorker(TransferOptions{CommitInterval: 1, BufferSize: 1}); err == nil {
		t.Fatalf("expected get fail")
	}
}

func TestWorkerCommitOnCancel(t *testing.T) {
	mqutils.ResetTestState()
	if err := runWorker(TransferOptions{CommitInterval: 100, BufferSize: 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkerIdleCommit(t *testing.T) {
	mqutils.ResetTestState()
	err := runWorkerSeq(TransferOptions{CommitInterval: 2, BufferSize: 1}, []bool{true, false, false, false})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestRunCompletedAndFailed(t *testing.T) {
	mqutils.ResetTestState()
	mqutils.ReturnNilMessage = true
	tm := NewTransferManager(TransferOptions{WorkerCount: 1})
	tm.Start()
	<-tm.done
	if tm.GetStats().Status != StatusCompleted {
		t.Fatalf("expected completed")
	}

	mqutils.ResetTestState()
	mqutils.FailConnectCall = 1
	tm2 := NewTransferManager(TransferOptions{WorkerCount: 1})
	tm2.Start()
	<-tm2.done
	if tm2.GetStats().Status != StatusFailed {
		t.Fatalf("expected failed")
	}
	mqutils.ResetTestState()
}
