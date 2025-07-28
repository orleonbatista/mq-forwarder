package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"mq-transfer-go/transferstore"
)

// SQLiteStore implements TransferStore backed by SQLite

type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (and creates if not exists) the database file and ensures schema
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	schema := `CREATE TABLE IF NOT EXISTS transfer_requests (
        request_id TEXT PRIMARY KEY,
        status TEXT,
        start_time TEXT,
        end_time TEXT,
        messages_total INTEGER,
        messages_transferred INTEGER,
        bytes_transferred INTEGER,
        error TEXT,
        buffer_size INTEGER,
        commit_interval INTEGER,
        non_shared_connection BOOLEAN,
        source_queue TEXT,
        destination_queue TEXT,
        source_connection TEXT,
        destination_connection TEXT
    );`
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Create(req transferstore.TransferRequest) error {
	src, _ := json.Marshal(req.SourceConnection)
	dest, _ := json.Marshal(req.DestinationConnection)
	_, err := s.db.Exec(`INSERT INTO transfer_requests (
        request_id, status, start_time, end_time, messages_total,
        messages_transferred, bytes_transferred, error, buffer_size,
        commit_interval, non_shared_connection, source_queue, destination_queue,
        source_connection, destination_connection
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.RequestID, req.Status, req.StartTime.Format(time.RFC3339), nilIfTime(req.EndTime), req.MessagesTotal,
		req.MessagesTransferred, req.BytesTransferred, req.Error, req.BufferSize,
		req.CommitInterval, req.NonSharedConnection, req.SourceQueue, req.DestinationQueue,
		string(src), string(dest))
	return err
}

func (s *SQLiteStore) GetByID(id string) (transferstore.TransferRequest, error) {
	row := s.db.QueryRow(`SELECT request_id, status, start_time, end_time, messages_total,
        messages_transferred, bytes_transferred, error, buffer_size, commit_interval,
        non_shared_connection, source_queue, destination_queue, source_connection,
        destination_connection FROM transfer_requests WHERE request_id = ?`, id)
	var req transferstore.TransferRequest
	var start, end, errMsg sql.NullString
	var srcJSON, destJSON string
	if err := row.Scan(&req.RequestID, &req.Status, &start, &end, &req.MessagesTotal,
		&req.MessagesTransferred, &req.BytesTransferred, &errMsg, &req.BufferSize,
		&req.CommitInterval, &req.NonSharedConnection, &req.SourceQueue, &req.DestinationQueue,
		&srcJSON, &destJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return transferstore.TransferRequest{}, err
		}
		return transferstore.TransferRequest{}, err
	}
	req.StartTime, _ = time.Parse(time.RFC3339, start.String)
	if end.Valid {
		t, _ := time.Parse(time.RFC3339, end.String)
		req.EndTime = &t
	}
	req.Error = errMsg.String
	_ = json.Unmarshal([]byte(srcJSON), &req.SourceConnection)
	_ = json.Unmarshal([]byte(destJSON), &req.DestinationConnection)
	return req, nil
}

func (s *SQLiteStore) UpdateStatus(id, status string, endTime *time.Time, errorMsg *string) error {
	var end interface{}
	if endTime != nil {
		end = endTime.Format(time.RFC3339)
	}
	_, err := s.db.Exec(`UPDATE transfer_requests SET status=?, end_time=?, error=? WHERE request_id=?`,
		status, end, errorMsg, id)
	return err
}

func (s *SQLiteStore) UpdateProgress(id string, messagesTransferred int, bytesTransferred int) error {
	_, err := s.db.Exec(`UPDATE transfer_requests SET messages_transferred=?, bytes_transferred=? WHERE request_id=?`,
		messagesTransferred, bytesTransferred, id)
	return err
}

func (s *SQLiteStore) List() ([]transferstore.TransferRequest, error) {
	rows, err := s.db.Query(`SELECT request_id, status, start_time, end_time, messages_total,
        messages_transferred, bytes_transferred, error, buffer_size, commit_interval,
        non_shared_connection, source_queue, destination_queue, source_connection,
        destination_connection FROM transfer_requests`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []transferstore.TransferRequest
	for rows.Next() {
		var req transferstore.TransferRequest
		var start, end, errMsg sql.NullString
		var srcJSON, destJSON string
		if err := rows.Scan(&req.RequestID, &req.Status, &start, &end, &req.MessagesTotal,
			&req.MessagesTransferred, &req.BytesTransferred, &errMsg, &req.BufferSize,
			&req.CommitInterval, &req.NonSharedConnection, &req.SourceQueue, &req.DestinationQueue,
			&srcJSON, &destJSON); err != nil {
			return nil, err
		}
		req.StartTime, _ = time.Parse(time.RFC3339, start.String)
		if end.Valid {
			t, _ := time.Parse(time.RFC3339, end.String)
			req.EndTime = &t
		}
		req.Error = errMsg.String
		_ = json.Unmarshal([]byte(srcJSON), &req.SourceConnection)
		_ = json.Unmarshal([]byte(destJSON), &req.DestinationConnection)
		results = append(results, req)
	}
	return results, rows.Err()
}

func nilIfTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}
