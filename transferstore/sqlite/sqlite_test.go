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
	list, err := store.List(transferstore.ListParams{Limit: 10})
	if err != nil || len(list) != 1 {
		t.Fatalf("list failed: %v %v", len(list), err)
	}

	// create another record to test filtering
	req2 := transferstore.TransferRequest{
		RequestID:             "2",
		Status:                "failed",
		StartTime:             now.Add(time.Hour),
		SourceQueue:           "SRC",
		DestinationQueue:      "DST",
		SourceConnection:      transferstore.ConnectionDetails{QueueManagerName: "QM1"},
		DestinationConnection: transferstore.ConnectionDetails{QueueManagerName: "QM2"},
	}
	if err := store.Create(req2); err != nil {
		t.Fatalf("create2: %v", err)
	}
	// filter by status
	filtered, err := store.List(transferstore.ListParams{Status: "failed"})
	if err != nil || len(filtered) != 1 || filtered[0].RequestID != "2" {
		t.Fatalf("status filter failed: %v %v", filtered, err)
	}
	// filter by time range
	start := now.Add(time.Minute)
	end2 := now.Add(2 * time.Hour)
	rangeFiltered, err := store.List(transferstore.ListParams{StartTime: &start, EndTime: &end2})
	if err != nil || len(rangeFiltered) != 1 || rangeFiltered[0].RequestID != "2" {
		t.Fatalf("time range filter failed: %v %v", rangeFiltered, err)
	}
	// order descending
	ordered, err := store.List(transferstore.ListParams{Order: "asc"})
	if err != nil || len(ordered) != 2 || ordered[0].RequestID != "1" {
		t.Fatalf("order failed: %v %v", ordered, err)
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
