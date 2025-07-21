package mqutils

import "testing"

func TestMQConnectionLifecycle(t *testing.T) {
	ResetTestState()
	cfg := MQConnectionConfig{QueueManagerName: "QM", ConnectionName: "conn", Channel: "CH"}
	c := NewMQConnection(cfg)
	if err := c.Connect(); err != nil {
		t.Fatalf("connect error: %v", err)
	}
	if !c.IsConnected {
		t.Fatalf("expected connected")
	}
	if _, err := c.OpenQueue("Q", true, false); err != nil {
		t.Fatalf("open queue error: %v", err)
	}
	if err := c.CloseQueue(struct{}{}); err != nil {
		t.Fatalf("close queue error: %v", err)
	}
	if _, _, err := c.GetMessage(struct{}{}, nil); err != nil {
		t.Fatalf("get message error: %v", err)
	}
	if err := c.PutMessage(struct{}{}, []byte("data"), nil, "none"); err != nil {
		t.Fatalf("put message error: %v", err)
	}
	if err := c.Commit(); err != nil {
		t.Fatalf("commit error: %v", err)
	}
	if err := c.Backout(); err != nil {
		t.Fatalf("backout error: %v", err)
	}
	if err := c.Disconnect(); err != nil {
		t.Fatalf("disconnect error: %v", err)
	}
	if c.IsConnected {
		t.Fatalf("expected disconnected")
	}
}

func TestMQConnectionFailures(t *testing.T) {
	ResetTestState()
	FailConnectCall = 1
	c := NewMQConnection(MQConnectionConfig{})
	if err := c.Connect(); err == nil {
		t.Fatalf("expected connect fail")
	}
	ResetTestState()
	if err := c.Connect(); err != nil {
		t.Fatalf("unexpected connect err: %v", err)
	}
	if _, err := c.OpenQueue("Q", true, false); err != nil {
		t.Fatalf("open queue error: %v", err)
	}
	ResetTestState()
	ReturnNilMessage = true
	if data, _, _ := c.GetMessage(struct{}{}, nil); data != nil {
		t.Fatalf("expected nil data")
	}
	ResetTestState()
	FailPut = true
	if err := c.PutMessage(struct{}{}, nil, nil, "none"); err == nil {
		t.Fatalf("expected put fail")
	}
	ResetTestState()
	FailCommit = true
	if err := c.Commit(); err == nil {
		t.Fatalf("expected commit fail")
	}
	ResetTestState()
	FailGet = true
	if _, _, err := c.GetMessage(struct{}{}, nil); err == nil {
		t.Fatalf("expected get fail")
	}
}

func TestOpenQueueNotConnected(t *testing.T) {
	ResetTestState()
	c := NewMQConnection(MQConnectionConfig{})
	if _, err := c.OpenQueue("q", true, false); err == nil {
		t.Fatalf("expected not connected")
	}
	ResetTestState()
	FailOpenCall = 1
	_ = c.Connect()
	if _, err := c.OpenQueue("q", true, false); err == nil {
		t.Fatalf("expected open fail")
	}
	ResetTestState()
}

func TestGetMessageSequence(t *testing.T) {
	ResetTestState()
	c := &MQConnection{}
	Messages = []bool{false, true}
	data, _, err := c.GetMessage(struct{}{}, nil)
	if err != nil || data != nil {
		t.Fatalf("expected nil msg on first call")
	}
	data, _, err = c.GetMessage(struct{}{}, nil)
	if err != nil || data == nil {
		t.Fatalf("expected message on second call")
	}
	if len(data) != 0 {
		t.Fatalf("expected zero-length data")
	}
}

func TestGetMessageBufferAlloc(t *testing.T) {
	ResetTestState()
	c := &MQConnection{}
	Messages = []bool{true}
	var buf []byte
	data, _, err := c.GetMessage(struct{}{}, buf)
	if err != nil || data == nil {
		t.Fatalf("unexpected error or nil data")
	}
	if len(data) != 0 {
		t.Fatalf("expected empty slice")
	}
}

func TestCommitCalls(t *testing.T) {
	ResetTestState()
	c := &MQConnection{}
	CommitCalls = 0
	if err := c.Commit(); err != nil {
		t.Fatalf("unexpected error")
	}
	if CommitCalls != 1 {
		t.Fatalf("expected 1 commit call")
	}
	FailCommit = true
	if err := c.Commit(); err == nil {
		t.Fatalf("expected commit fail")
	}
	if CommitCalls != 2 {
		t.Fatalf("expected second commit count")
	}
}

func TestCommitFailCall(t *testing.T) {
	ResetTestState()
	c := &MQConnection{}
	CommitCalls = 0
	FailCommitCall = 1
	if err := c.Commit(); err == nil {
		t.Fatalf("expected fail by call")
	}
	if CommitCalls != 1 {
		t.Fatalf("call count not 1")
	}
}

func TestGetMessagePastEnd(t *testing.T) {
	ResetTestState()
	c := &MQConnection{}
	Messages = []bool{true}
	msgIndex = 1
	data, _, err := c.GetMessage(struct{}{}, nil)
	if err != nil || data != nil {
		t.Fatalf("expected nil after end")
	}
}
