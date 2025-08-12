package transferstore

import "time"

// ConnectionDetails holds MQ connection info
type ConnectionDetails struct {
	QueueManagerName string
	ConnectionName   string
	Channel          string
	Username         string
}

// TransferRequest represents a transfer record persisted in storage
// Error is optional. EndTime can be nil while in progress.
type TransferRequest struct {
	RequestID             string
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
	SourceConnection      ConnectionDetails
	DestinationConnection ConnectionDetails
}

// ListParams defines optional filters for listing transfer requests.
type ListParams struct {
	Offset    int
	Limit     int
	Status    string
	StartTime *time.Time
	EndTime   *time.Time
	Order     string // "asc" or "desc" by StartTime
}

// TransferStore abstracts persistence operations for TransferRequest
// UpdateProgress should be called periodically to persist transfer progress.
type TransferStore interface {
	Create(req TransferRequest) error
	GetByID(id string) (TransferRequest, error)
	UpdateStatus(id, status string, endTime *time.Time, errorMsg *string) error
	UpdateProgress(id string, messagesTransferred int, bytesTransferred int) error
	// List returns transfer records matching the provided filters.
	List(params ListParams) ([]TransferRequest, error)
}
