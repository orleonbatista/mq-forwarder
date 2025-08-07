package sqlite

import (
	"testing"
	"time"

	"mq-forwarder-go/transferstore"
)

func TestSQLiteStore(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := NewSQLiteStore("/invalid/path/db.sqlite"); err == nil {
		t.Fatalf("expected error for invalid path")
	}
	now := time.Now().UTC()
	req := transferstore.TransferRequest{
		RequestID:             "1",
		Status:                "in_progress",
		StartTime:             now,
		SourceQueue:           "SRC",
		DestinationQueue:      "DST",
		SourceConnection:      transferstore.ConnectionDetails{QueueManagerName: "QM1"},
		DestinationConnection: transferstore.ConnectionDetails{QueueManagerName: "QM2"},
	}
	if err := store.Create(req); err != nil {
		t.Fatalf("create: %v", err)
	}
	fetched, err := store.GetByID("1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetched.RequestID != req.RequestID || fetched.Status != req.Status {
		t.Fatalf("unexpected record: %+v", fetched)
	}
	if err := store.UpdateProgress("1", 5, 100); err != nil {
		t.Fatalf("update progress: %v", err)
	}
	fetched, _ = store.GetByID("1")
	if fetched.MessagesTransferred != 5 || fetched.BytesTransferred != 100 {
		t.Fatalf("progress not updated: %+v", fetched)
	}
	end := now.Add(time.Minute)
	errMsg := "done"
	if err := store.UpdateStatus("1", "completed", &end, &errMsg); err != nil {
		t.Fatalf("update status: %v", err)
	}
	fetched, _ = store.GetByID("1")
	if fetched.Status != "completed" || fetched.Error != errMsg {
		t.Fatalf("status not updated: %+v", fetched)
	}
	list, err := store.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list failed: %v %v", len(list), err)
	}
	if _, err := store.GetByID("missing"); err == nil {
		t.Fatalf("expected not found")
	}

	if err := store.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	store.Close()
	if err := store.Ping(); err == nil {
		t.Fatalf("expected ping error after close")
	}
}
