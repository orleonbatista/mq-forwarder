package sqlite

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mq-forwarder-go/transferstore"
)

// SQLiteStore implements TransferStore backed by GORM and SQLite
type SQLiteStore struct {
	db *gorm.DB
}

// transferRequestModel represents the database schema for a transfer request
type transferRequestModel struct {
	RequestID             string `gorm:"primaryKey"`
	Status                string
	StartTime             time.Time
	EndTime               *time.Time
	MessagesTotal         int
	MessagesTransferred   int
	BytesTransferred      int
	Error                 string
	BufferSize            int
	CommitInterval        int
	NonSharedConnection   bool
	SourceQueue           string
	DestinationQueue      string
	SourceConnection      datatypes.JSON
	DestinationConnection datatypes.JSON
}

// NewSQLiteStore opens (and creates if not exists) the database file and ensures schema
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&transferRequestModel{}); err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Create(req transferstore.TransferRequest) error {
	src, _ := json.Marshal(req.SourceConnection)
	dest, _ := json.Marshal(req.DestinationConnection)
	model := transferRequestModel{
		RequestID:             req.RequestID,
		Status:                req.Status,
		StartTime:             req.StartTime,
		EndTime:               req.EndTime,
		MessagesTotal:         req.MessagesTotal,
		MessagesTransferred:   req.MessagesTransferred,
		BytesTransferred:      req.BytesTransferred,
		Error:                 req.Error,
		BufferSize:            req.BufferSize,
		CommitInterval:        req.CommitInterval,
		NonSharedConnection:   req.NonSharedConnection,
		SourceQueue:           req.SourceQueue,
		DestinationQueue:      req.DestinationQueue,
		SourceConnection:      datatypes.JSON(src),
		DestinationConnection: datatypes.JSON(dest),
	}
	return s.db.Create(&model).Error
}

func (s *SQLiteStore) GetByID(id string) (transferstore.TransferRequest, error) {
	var model transferRequestModel
	if err := s.db.First(&model, "request_id = ?", id).Error; err != nil {
		return transferstore.TransferRequest{}, err
	}
	return toTransferRequest(model), nil
}

func (s *SQLiteStore) UpdateStatus(id, status string, endTime *time.Time, errorMsg *string) error {
	return s.db.Model(&transferRequestModel{}).
		Where("request_id = ?", id).
		Updates(map[string]interface{}{
			"status":   status,
			"end_time": endTime,
			"error":    errorMsg,
		}).Error
}

func (s *SQLiteStore) UpdateProgress(id string, messagesTransferred int, bytesTransferred int) error {
	return s.db.Model(&transferRequestModel{}).
		Where("request_id = ?", id).
		Updates(map[string]interface{}{
			"messages_transferred": messagesTransferred,
			"bytes_transferred":    bytesTransferred,
		}).Error
}

func (s *SQLiteStore) List(params transferstore.ListParams) ([]transferstore.TransferRequest, error) {
	var models []transferRequestModel
	q := s.db
	if params.Status != "" {
		q = q.Where("status = ?", params.Status)
	}
	if params.StartTime != nil {
		q = q.Where("start_time >= ?", *params.StartTime)
	}
	if params.EndTime != nil {
		q = q.Where("start_time <= ?", *params.EndTime)
	}
	order := "start_time desc"
	if params.Order == "asc" {
		order = "start_time asc"
	}
	q = q.Order(order)
	if params.Offset > 0 {
		q = q.Offset(params.Offset)
	}
	if params.Limit > 0 {
		q = q.Limit(params.Limit)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	results := make([]transferstore.TransferRequest, 0, len(models))
	for _, m := range models {
		results = append(results, toTransferRequest(m))
	}
	return results, nil
}

func toTransferRequest(m transferRequestModel) transferstore.TransferRequest {
	var src, dest transferstore.ConnectionDetails
	_ = json.Unmarshal([]byte(m.SourceConnection), &src)
	_ = json.Unmarshal([]byte(m.DestinationConnection), &dest)
	return transferstore.TransferRequest{
		RequestID:             m.RequestID,
		Status:                m.Status,
		StartTime:             m.StartTime,
		EndTime:               m.EndTime,
		MessagesTotal:         m.MessagesTotal,
		MessagesTransferred:   m.MessagesTransferred,
		BytesTransferred:      m.BytesTransferred,
		Error:                 m.Error,
		BufferSize:            m.BufferSize,
		CommitInterval:        m.CommitInterval,
		NonSharedConnection:   m.NonSharedConnection,
		SourceQueue:           m.SourceQueue,
		DestinationQueue:      m.DestinationQueue,
		SourceConnection:      src,
		DestinationConnection: dest,
	}
}

// Ping verifies the database connection is alive.
func (s *SQLiteStore) Ping() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
