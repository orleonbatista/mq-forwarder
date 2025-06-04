package models

// ConnectionDetails contém os detalhes de conexão para um Queue Manager MQ
type ConnectionDetails struct {
	QueueManagerName string `json:"queueManagerName" binding:"required" example:"QM1" swaggertype:"string"`
	ConnectionName   string `json:"connectionName" binding:"required" example:"localhost(1414)" swaggertype:"string"`
	Channel          string `json:"channel" binding:"required" example:"SYSTEM.DEF.SVRCONN" swaggertype:"string"`
	Username         string `json:"username,omitempty" example:"mquser" swaggertype:"string"`
	Password         string `json:"password,omitempty" example:"mqpassword" swaggertype:"string"`
}

// TransferRequest representa o payload para uma solicitação de transferência de mensagens
type TransferRequest struct {
	Source              ConnectionDetails `json:"source" binding:"required"`
	SourceQueue         string            `json:"sourceQueue" binding:"required" example:"SOURCE.QUEUE" swaggertype:"string"`
	Destination         ConnectionDetails `json:"destination" binding:"required"`
	DestinationQueue    string            `json:"destinationQueue" binding:"required" example:"DEST.QUEUE" swaggertype:"string"`
	BufferSize          int               `json:"bufferSize,omitempty" example:"1048576" swaggertype:"integer"`
	NonSharedConnection bool              `json:"nonSharedConnection,omitempty" example:"false" swaggertype:"boolean"`
	CommitInterval      int               `json:"commitInterval,omitempty" example:"10" swaggertype:"integer"`
}

// TransferResponse representa a resposta de uma operação de transferência
type TransferResponse struct {
	RequestID           string `json:"requestId" example:"550e8400-e29b-41d4-a716-446655440000" swaggertype:"string"`
	Status              string `json:"status" example:"in_progress" swaggertype:"string"`
	MessagesTotal       int    `json:"messagesTotal,omitempty" example:"1000" swaggertype:"integer"`
	MessagesTransferred int    `json:"messagesTransferred,omitempty" example:"500" swaggertype:"integer"`
	Error               string `json:"error,omitempty" example:"Failed to connect to source queue manager" swaggertype:"string"`
}

// TransferStatus representa o status atual de uma transferência em andamento
type TransferStatus struct {
	RequestID           string `json:"requestId" example:"550e8400-e29b-41d4-a716-446655440000" swaggertype:"string"`
	Status              string `json:"status" example:"in_progress" swaggertype:"string"`
	StartTime           string `json:"startTime" example:"2025-06-04T00:15:30Z" swaggertype:"string"`
	EndTime             string `json:"endTime,omitempty" example:"2025-06-04T00:16:45Z" swaggertype:"string"`
	MessagesTotal       int    `json:"messagesTotal,omitempty" example:"1000" swaggertype:"integer"`
	MessagesTransferred int    `json:"messagesTransferred" example:"500" swaggertype:"integer"`
	BytesTransferred    int64  `json:"bytesTransferred" example:"1048576" swaggertype:"integer"`
	Error               string `json:"error,omitempty" example:"Failed to connect to source queue manager" swaggertype:"string"`
}

// HealthResponse representa a resposta do endpoint de health check
type HealthResponse struct {
	Status  string `json:"status" example:"ok" swaggertype:"string"`
	Version string `json:"version" example:"1.0.0" swaggertype:"string"`
}
