package handlers

import (
	"database/sql"
	"errors"
	"mq-forwarder-go/transferstore"
	"os"
	"testing"
	"time"
)

func TestResolveHelpers(t *testing.T) {
	os.Setenv("BUFFER_SIZE", "2048")
	if v := resolveBufferSize(0); v != 2048 {
		t.Fatalf("buf env not used: %d", v)
	}
	if v := resolveBufferSize(1024); v != 1024 {
		t.Fatalf("buf direct")
	}
	os.Setenv("BUFFER_SIZE", "bad")
	if v := resolveBufferSize(0); v != 1048576 {
		t.Fatalf("default not used")
	}
	os.Unsetenv("BUFFER_SIZE")

	os.Setenv("WORKER_COUNT", "2")
	if v := resolveWorkerCount(); v != 2 {
		t.Fatalf("worker env")
	}
	os.Setenv("WORKER_COUNT", "bad")
	if v := resolveWorkerCount(); v != 0 {
		t.Fatalf("worker default")
	}
	os.Unsetenv("WORKER_COUNT")

	os.Setenv("BATCH_SIZE", "5")
	if v := resolveCommitInterval(0); v != 5 {
		t.Fatalf("commit env")
	}
	if v := resolveCommitInterval(3); v != 3 {
		t.Fatalf("commit direct")
	}
	os.Setenv("BATCH_SIZE", "bad")
	if v := resolveCommitInterval(0); v != 10 {
		t.Fatalf("commit default")
	}
	os.Unsetenv("BATCH_SIZE")
}

func TestIsNotFound(t *testing.T) {
	if isNotFound(nil) {
		t.Fatalf("nil")
	}
	if !isNotFound(sql.ErrNoRows) {
		t.Fatalf("sql err")
	}
	if !isNotFound(errors.New("not found")) {
		t.Fatalf("text err")
	}
}

func TestToStatus(t *testing.T) {
	now := time.Now().UTC()
	end := now.Add(time.Minute)
	req := transferstore.TransferRequest{RequestID: "1", Status: "s", StartTime: now, EndTime: &end, BytesTransferred: 2, MessagesTransferred: 3}
	st := toStatus(req)
	if st.RequestID != "1" || st.Status != "s" || st.EndTime == "" {
		t.Fatalf("bad status: %+v", st)
	}
}
