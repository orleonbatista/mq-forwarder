package mqutils

import "testing"

func TestMQConnectionLifecycle(t *testing.T) {
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
	FailConnect = true
	c := NewMQConnection(MQConnectionConfig{})
	if err := c.Connect(); err == nil {
		t.Fatalf("expected connect fail")
	}
	FailConnect = false
	c.Connect()
	ReturnNilMessage = true
	if data, _, _ := c.GetMessage(struct{}{}, nil); data != nil {
		t.Fatalf("expected nil data")
	}
	ReturnNilMessage = false
}
