package transfer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	metricnoop "go.opentelemetry.io/otel/metric/noop"

	"mq-transfer-go/internal/mqutils"
	"mq-transfer-go/internal/otelutils"
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
	ch := make(chan workerResult, 2)
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
	ch := make(chan workerResult, 2)
	wg.Add(1)
	go tm.worker(ctx, cancel, &wg, ch, context.Background(), nil)
	wg.Wait()
}

func runWorker(opts TransferOptions) error {
	tm := NewTransferManager(opts)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ch := make(chan workerResult, 2)
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
	ch := make(chan workerResult, 2)
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

func TestCommitIfNeeded(t *testing.T) {
	tm := NewTransferManager(TransferOptions{CommitInterval: 1})
	dest := &mqutils.MQConnection{}
	src := &mqutils.MQConnection{}
	count := 1
	ch := make(chan workerResult, 1)
	ctx, cancel := context.WithCancel(context.Background())
	metrics := &otelutils.MQMetrics{}
	m := metricnoop.NewMeterProvider().Meter("m")
	c, _ := m.Int64Counter("c")
	metrics.CommitCounter = c

	mqutils.FailCommitCall = 1
	if !tm.commitIfNeeded(&count, dest, src, metrics, ctx, ch, cancel) {
		t.Fatalf("expected true")
	}
	if (<-ch).err == nil {
		t.Fatalf("expected error")
	}

	mqutils.ResetTestState()
	mqutils.FailCommitCall = 2
	count = 1
	if !tm.commitIfNeeded(&count, dest, src, metrics, ctx, ch, cancel) {
		t.Fatalf("expected true for src fail")
	}
	if (<-ch).err == nil {
		t.Fatalf("expected error 2")
	}

	mqutils.ResetTestState()
	count = 1
	if tm.commitIfNeeded(&count, dest, src, metrics, ctx, ch, cancel) {
		t.Fatalf("unexpected true")
	}
	if count != 0 {
		t.Fatalf("counter not reset")
	}
}

func TestCommitRemainingAndHelpers(t *testing.T) {
	tm := NewTransferManager(TransferOptions{CommitInterval: 1})
	dest := &mqutils.MQConnection{}
	src := &mqutils.MQConnection{}
	mqutils.ResetTestState()
	mqutils.CommitCalls = 0
	tm.commitRemaining(2, dest, src)
	if mqutils.CommitCalls != 2 {
		t.Fatalf("expected two commits")
	}

	mqutils.CommitCalls = 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := make(chan workerResult, 1)
	if !tm.handleContextDone(ctx, 1, dest, src, ch) {
		t.Fatalf("context done expected")
	}
	<-ch

	idle := 2
	commitCounter := 1
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	ch2 := make(chan workerResult, 1)
	if !tm.handleIdle(ctx2, &idle, &commitCounter, dest, src, ch2) {
		t.Fatalf("idle exit")
	}
	<-ch2
}

func TestCopyBuffer(t *testing.T) {
	tm := NewTransferManager(TransferOptions{BufferSize: 1})
	b := tm.copyBuffer([]byte("abc"))
	if string(b) != "abc" {
		t.Fatalf("bad copy")
	}
	b[0] = 'x'
	if string(b) == "abc" {
		t.Fatalf("expected copy not alias")
	}
}

func TestHandleIdleBranches(t *testing.T) {
	tm := NewTransferManager(TransferOptions{CommitInterval: 1})
	dest := &mqutils.MQConnection{}
	src := &mqutils.MQConnection{}
	idle := 3
	commitCounter := 1
	ch := make(chan workerResult, 1)
	if !tm.handleIdle(context.Background(), &idle, &commitCounter, dest, src, ch) {
		t.Fatalf("expected true when idle >=3")
	}
	<-ch

	idle = 0
	commitCounter = 1
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !tm.handleIdle(ctx, &idle, &commitCounter, dest, src, ch) {
		t.Fatalf("expected true on cancel")
	}
	<-ch

	idle = 0
	commitCounter = 0
	start := time.Now()
	if tm.handleIdle(context.Background(), &idle, &commitCounter, dest, src, ch) {
		t.Fatalf("expected false")
	}
	if time.Since(start) < time.Second {
		t.Fatalf("sleep not executed")
	}
}

func TestWorkerWithMetrics(t *testing.T) {
	mqutils.ResetTestState()
	tm := NewTransferManager(TransferOptions{CommitInterval: 1, BufferSize: 1})
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ch := make(chan workerResult, 2)
	wg.Add(1)
	m := metricnoop.NewMeterProvider().Meter("m")
	msgC, _ := m.Int64Counter("msg")
	byteC, _ := m.Int64Counter("bytes")
	dur, _ := m.Float64Histogram("dur")
	commitC, _ := m.Int64Counter("commit")
	metrics := &otelutils.MQMetrics{MessagesTransferred: msgC, BytesTransferred: byteC, TransferDuration: dur, CommitCounter: commitC}
	go tm.worker(ctx, cancel, &wg, ch, context.Background(), metrics)
	time.Sleep(2 * time.Millisecond)
	cancel()
	wg.Wait()
	<-ch
}

func TestSetStatsForTest(t *testing.T) {
	tm := NewTransferManager(TransferOptions{})
	tm.SetStatsForTest(Stats{Status: StatusFailed, Error: "e"})
	s := tm.GetStats()
	if s.Status != StatusFailed || s.Error != "e" {
		t.Fatalf("stats not set")
	}
}
