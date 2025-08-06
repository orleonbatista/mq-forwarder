package dynamo

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"mq-forwarder-go/transferstore"
)

type mockClient struct {
	items map[string]map[string]types.AttributeValue
}

type errClient struct{}

func (e *errClient) PutItem(ctx context.Context, in *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return nil, context.Canceled
}
func (e *errClient) GetItem(ctx context.Context, in *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return nil, context.Canceled
}
func (e *errClient) UpdateItem(ctx context.Context, in *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return nil, context.Canceled
}
func (e *errClient) Scan(ctx context.Context, in *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	return nil, context.Canceled
}

func newMock() *mockClient {
	return &mockClient{items: make(map[string]map[string]types.AttributeValue)}
}

func (m *mockClient) PutItem(ctx context.Context, in *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	id := in.Item["RequestID"].(*types.AttributeValueMemberS).Value
	cp := make(map[string]types.AttributeValue)
	for k, v := range in.Item {
		cp[k] = v
	}
	m.items[id] = cp
	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockClient) GetItem(ctx context.Context, in *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	id := in.Key["RequestID"].(*types.AttributeValueMemberS).Value
	item, ok := m.items[id]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: item}, nil
}

func (m *mockClient) UpdateItem(ctx context.Context, in *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	id := in.Key["RequestID"].(*types.AttributeValueMemberS).Value
	itm := m.items[id]
	if itm == nil {
		itm = make(map[string]types.AttributeValue)
		m.items[id] = itm
	}
	expr := *in.UpdateExpression
	if expr == "SET MessagesTransferred = :m, BytesTransferred = :b" {
		itm["MessagesTransferred"] = in.ExpressionAttributeValues[":m"]
		itm["BytesTransferred"] = in.ExpressionAttributeValues[":b"]
	} else {
		itm["Status"] = in.ExpressionAttributeValues[":s"]
		if v, ok := in.ExpressionAttributeValues[":e"]; ok {
			itm["EndTime"] = v
		}
		if v, ok := in.ExpressionAttributeValues[":err"]; ok {
			itm["Error"] = v
		}
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func (m *mockClient) Scan(ctx context.Context, in *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	var lst []map[string]types.AttributeValue
	for _, v := range m.items {
		lst = append(lst, v)
	}
	return &dynamodb.ScanOutput{Items: lst}, nil
}

func TestDynamoStore(t *testing.T) {
	mock := newMock()
	store := NewDynamoStore(mock, "tbl")
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
	list, err := store.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", len(list), err)
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
	store := NewDynamoStore(&errClient{}, "tbl")
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
	if _, err := store.List(); err == nil {
		t.Fatal("expected error")
	}

	if err := store.Ping(); err == nil {
		t.Fatal("expected ping error")
	}
}
