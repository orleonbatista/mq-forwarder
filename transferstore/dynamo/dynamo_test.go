package dynamo

import (
	"errors"
	"testing"
	"time"

	"github.com/guregu/dynamo"
	"mq-forwarder-go/transferstore"
)

type mockTable struct {
	items map[string]transferstore.TransferRequest
	fail  bool
}

func newMockTable() *mockTable {
	return &mockTable{items: make(map[string]transferstore.TransferRequest)}
}

func (m *mockTable) Put(item interface{}) PutItem {
	return &mockPut{m: m, item: item.(transferstore.TransferRequest)}
}

func (m *mockTable) Get(key string, val interface{}) GetItem {
	return &mockGet{m: m, id: val.(string)}
}

func (m *mockTable) Update(key string, val interface{}) UpdateItem {
	return &mockUpdate{m: m, id: val.(string)}
}

func (m *mockTable) Scan() ScanItem {
	return &mockScan{m: m}
}

type mockPut struct {
	m    *mockTable
	item transferstore.TransferRequest
}

func (p *mockPut) Run() error {
	if p.m.fail {
		return errors.New("fail")
	}
	p.m.items[p.item.RequestID] = p.item
	return nil
}

type mockGet struct {
	m  *mockTable
	id string
}

func (g *mockGet) One(out interface{}) error {
	if g.m.fail {
		return errors.New("fail")
	}
	req, ok := g.m.items[g.id]
	if !ok {
		return dynamo.ErrNotFound
	}
	*(out.(*transferstore.TransferRequest)) = req
	return nil
}

type mockUpdate struct {
	m    *mockTable
	id   string
	sets map[string]interface{}
}

func (u *mockUpdate) Set(name string, value interface{}) UpdateItem {
	if u.sets == nil {
		u.sets = make(map[string]interface{})
	}
	u.sets[name] = value
	return u
}

func (u *mockUpdate) Run() error {
	if u.m.fail {
		return errors.New("fail")
	}
	req := u.m.items[u.id]
	for k, v := range u.sets {
		switch k {
		case "Status":
			req.Status = v.(string)
		case "EndTime":
			if t, ok := v.(*time.Time); ok {
				req.EndTime = t
			}
		case "Error":
			req.Error = v.(string)
		case "MessagesTransferred":
			req.MessagesTransferred = v.(int)
		case "BytesTransferred":
			req.BytesTransferred = v.(int)
		}
	}
	u.m.items[u.id] = req
	return nil
}

type mockScan struct {
	m     *mockTable
	limit int64
}

func (s *mockScan) Limit(n int64) ScanItem {
	s.limit = n
	return s
}

func (s *mockScan) All(out interface{}) error {
	if s.m.fail {
		return errors.New("fail")
	}
	list := make([]transferstore.TransferRequest, 0, len(s.m.items))
	for _, v := range s.m.items {
		list = append(list, v)
		if s.limit > 0 && int64(len(list)) >= s.limit {
			break
		}
	}
	*(out.(*[]transferstore.TransferRequest)) = list
	return nil
}

func (s *mockScan) Count() (int64, error) {
	if s.m.fail {
		return 0, errors.New("fail")
	}
	var n int64
	for range s.m.items {
		n++
		if s.limit > 0 && n >= s.limit {
			break
		}
	}
	return n, nil
}

func TestDynamoStore(t *testing.T) {
	tbl := newMockTable()
	store := NewDynamoStoreWithTable(tbl)
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
	got, err := store.GetByID("1")
	if err != nil || got.RequestID != "1" {
		t.Fatalf("get: %v %v", got, err)
	}
	if err := store.UpdateProgress("1", 2, 50); err != nil {
		t.Fatalf("upd prog: %v", err)
	}
	got, _ = store.GetByID("1")
	if got.MessagesTransferred != 2 {
		t.Fatalf("prog wrong: %+v", got)
	}
	end := now.Add(time.Minute)
	msg := "ok"
	if err := store.UpdateStatus("1", "completed", &end, &msg); err != nil {
		t.Fatalf("upd status: %v", err)
	}
	got, _ = store.GetByID("1")
	if got.Status != "completed" || got.Error != msg {
		t.Fatalf("status wrong: %+v", got)
	}
	list, err := store.List(transferstore.ListParams{Limit: 10})
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", len(list), err)
	}
	// filtering by status
	filtered, err := store.List(transferstore.ListParams{Status: "completed"})
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filter status failed: %v %v", filtered, err)
	}
	if err := store.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := store.Ping(); err == nil {
		t.Fatalf("expected ping error after close")
	}
}

func TestDynamoStoreErrors(t *testing.T) {
	tbl := newMockTable()
	tbl.fail = true
	store := NewDynamoStoreWithTable(tbl)
	now := time.Now()
	req := transferstore.TransferRequest{RequestID: "1", StartTime: now}
	if err := store.Create(req); err == nil {
		t.Fatal("expected error")
	}
	if _, err := store.GetByID("1"); err == nil {
		t.Fatal("expected error")
	}
	if err := store.UpdateStatus("1", "s", nil, nil); err == nil {
		t.Fatal("expected error")
	}
	if err := store.UpdateProgress("1", 1, 1); err == nil {
		t.Fatal("expected error")
	}
	if _, err := store.List(transferstore.ListParams{Limit: 10}); err == nil {
		t.Fatal("expected error")
	}
	if err := store.Ping(); err == nil {
		t.Fatal("expected ping error")
	}
}
