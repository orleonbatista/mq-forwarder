package dynamo

import (
	"errors"
	"time"

	dynamolib "github.com/guregu/dynamo"
	"mq-forwarder-go/transferstore"
)

type (
	TableAPI interface {
		Put(interface{}) PutItem
		Get(string, interface{}) GetItem
		Update(string, interface{}) UpdateItem
		Scan() ScanItem
	}

	PutItem interface {
		Run() error
	}

	GetItem interface {
		One(out interface{}) error
	}

	UpdateItem interface {
		Set(name string, value interface{}) UpdateItem
		Run() error
	}

	ScanItem interface {
		Limit(n int64) ScanItem
		All(out interface{}) error
		Count() (int64, error)
	}
)

type DynamoStore struct {
	table  TableAPI
	closed bool
}

func NewDynamoStore(db *dynamolib.DB, tableName string) *DynamoStore {
	return &DynamoStore{table: dynamoTable{tbl: db.Table(tableName)}}
}

func NewDynamoStoreWithTable(tbl TableAPI) *DynamoStore {
	return &DynamoStore{table: tbl}
}

func (d *DynamoStore) Create(req transferstore.TransferRequest) error {
	if d.closed {
		return errors.New("store closed")
	}
	return d.table.Put(req).Run()
}

func (d *DynamoStore) GetByID(id string) (transferstore.TransferRequest, error) {
	if d.closed {
		return transferstore.TransferRequest{}, errors.New("store closed")
	}
	var req transferstore.TransferRequest
	err := d.table.Get("RequestID", id).One(&req)
	if err != nil {
		if errors.Is(err, dynamolib.ErrNotFound) {
			return transferstore.TransferRequest{}, errors.New("not found")
		}
		return transferstore.TransferRequest{}, err
	}
	return req, nil
}

func (d *DynamoStore) UpdateStatus(id, status string, endTime *time.Time, errorMsg *string) error {
	if d.closed {
		return errors.New("store closed")
	}
	upd := d.table.Update("RequestID", id).Set("Status", status)
	if endTime != nil {
		upd = upd.Set("EndTime", endTime)
	}
	if errorMsg != nil {
		upd = upd.Set("Error", *errorMsg)
	}
	return upd.Run()
}

func (d *DynamoStore) UpdateProgress(id string, messagesTransferred int, bytesTransferred int) error {
	if d.closed {
		return errors.New("store closed")
	}
	return d.table.Update("RequestID", id).
		Set("MessagesTransferred", messagesTransferred).
		Set("BytesTransferred", bytesTransferred).
		Run()
}

func (d *DynamoStore) List(offset, limit int) ([]transferstore.TransferRequest, error) {
	if d.closed {
		return nil, errors.New("store closed")
	}
	var reqs []transferstore.TransferRequest
	scan := d.table.Scan()
	if limit > 0 {
		// fetch enough items to satisfy offset + limit
		scan = scan.Limit(int64(offset + limit))
	}
	if err := scan.All(&reqs); err != nil {
		return nil, err
	}
	if offset >= len(reqs) {
		return []transferstore.TransferRequest{}, nil
	}
	end := len(reqs)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return reqs[offset:end], nil
}

func (d *DynamoStore) Ping() error {
	if d.closed {
		return errors.New("store closed")
	}
	_, err := d.table.Scan().Limit(1).Count()
	return err
}

func (d *DynamoStore) Close() error {
	d.closed = true
	return nil
}

// Adapter implementations

type dynamoTable struct{ tbl dynamolib.Table }

func (t dynamoTable) Put(item interface{}) PutItem { return &dynamoPut{t.tbl.Put(item)} }
func (t dynamoTable) Get(partitionKey string, value interface{}) GetItem {
	return &dynamoGet{t.tbl.Get(partitionKey, value)}
}
func (t dynamoTable) Update(partitionKey string, value interface{}) UpdateItem {
	return &dynamoUpdate{t.tbl.Update(partitionKey, value)}
}
func (t dynamoTable) Scan() ScanItem { return &dynamoScan{t.tbl.Scan()} }

type dynamoPut struct{ put *dynamolib.Put }

func (p *dynamoPut) Run() error { return p.put.Run() }

type dynamoGet struct{ q *dynamolib.Query }

func (g *dynamoGet) One(out interface{}) error { return g.q.One(out) }

type dynamoUpdate struct{ upd *dynamolib.Update }

func (u *dynamoUpdate) Set(name string, value interface{}) UpdateItem {
	u.upd = u.upd.Set(name, value)
	return u
}
func (u *dynamoUpdate) Run() error { return u.upd.Run() }

type dynamoScan struct{ sc *dynamolib.Scan }

func (s *dynamoScan) Limit(n int64) ScanItem {
	s.sc = s.sc.Limit(n)
	return s
}
func (s *dynamoScan) All(out interface{}) error { return s.sc.All(out) }
func (s *dynamoScan) Count() (int64, error)     { return s.sc.Count() }
