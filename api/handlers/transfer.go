package handlers

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"mq-transfer-go/api/models"
	"mq-transfer-go/internal/mqutils"
	"mq-transfer-go/internal/transfer"
)

var (
	transferStatuses = make(map[string]models.TransferStatus)
	transferManagers = make(map[string]*transfer.TransferManager)
	statusMutex      = &sync.RWMutex{}
)

// @Summary Iniciar transferência de mensagens MQ
// @Description Inicia uma transferência de mensagens de uma fila MQ para outra
// @Tags transfer
// @Accept json
// @Produce json
// @Param request body models.TransferRequest true "Detalhes da transferência"
// @Success 202 {object} models.TransferResponse "Transferência iniciada"
// @Failure 400 {object} models.TransferResponse "Erro na requisição"
// @Failure 500 {object} models.TransferResponse "Erro interno"
// @Router /api/v1/transfer [post]
func StartTransfer(c *gin.Context) {
	var request models.TransferRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, models.TransferResponse{
			Status: "failed",
			Error:  "Erro ao processar requisição: " + err.Error(),
		})
		return
	}

	requestID := uuid.New().String()

	if request.BufferSize <= 0 {
		request.BufferSize = 1048576
	}

	options := transfer.TransferOptions{
		SourceConfig: mqutils.MQConnectionConfig{
			QueueManagerName:    request.Source.QueueManagerName,
			ConnectionName:      request.Source.ConnectionName,
			Channel:             request.Source.Channel,
			Username:            request.Source.Username,
			Password:            request.Source.Password,
			NonSharedConnection: request.NonSharedConnection,
		},
		SourceQueue: request.SourceQueue,
		DestConfig: mqutils.MQConnectionConfig{
			QueueManagerName: request.Destination.QueueManagerName,
			ConnectionName:   request.Destination.ConnectionName,
			Channel:          request.Destination.Channel,
			Username:         request.Destination.Username,
			Password:         request.Destination.Password,
		},
		DestQueue:           request.DestinationQueue,
		BufferSize:          request.BufferSize,
		CommitInterval:      request.CommitInterval,
		NonSharedConnection: request.NonSharedConnection,
	}

	transferMgr := transfer.NewTransferManager(options)
	transferMgr.Start()

	status := models.TransferStatus{
		RequestID:           requestID,
		Status:              "in_progress",
		StartTime:           time.Now().UTC().Format(time.RFC3339),
		MessagesTransferred: 0,
	}

	statusMutex.Lock()
	transferStatuses[requestID] = status
	transferManagers[requestID] = transferMgr
	statusMutex.Unlock()

	go monitorTransfer(requestID, transferMgr)

	c.JSON(http.StatusAccepted, models.TransferResponse{
		RequestID: requestID,
		Status:    "in_progress",
	})
}

func monitorTransfer(requestID string, transferMgr *transfer.TransferManager) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		stats := transferMgr.GetStats()

		statusMutex.Lock()
		status := transferStatuses[requestID]
		status.MessagesTransferred = int(stats.MessagesTransferred)
		status.BytesTransferred = stats.BytesTransferred
		status.Status = stats.Status

		if stats.Status == "completed" || stats.Status == "failed" {
			status.EndTime = stats.EndTime.UTC().Format(time.RFC3339)
			status.Error = stats.Error
			transferStatuses[requestID] = status
			delete(transferManagers, requestID)
			statusMutex.Unlock()
			return
		}

		transferStatuses[requestID] = status
		statusMutex.Unlock()
	}
}

// @Summary Obter status da transferência
// @Description Retorna o status atual de uma transferência de mensagens
// @Tags transfer
// @Produce json
// @Param requestId path string true "ID da requisição de transferência"
// @Success 200 {object} models.TransferStatus "Status da transferência"
// @Failure 404 {object} models.TransferResponse "Transferência não encontrada"
// @Router /api/v1/transfer/{requestId} [get]
func GetTransferStatus(c *gin.Context) {
	requestID := c.Param("requestId")

	statusMutex.RLock()
	status, exists := transferStatuses[requestID]
	statusMutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, models.TransferResponse{
			Status: "failed",
			Error:  "Transferência não encontrada",
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// @Summary Listar todas as transferências
// @Description Retorna uma lista com todas as transferências e seus status
// @Tags transfer
// @Produce json
// @Success 200 {array} models.TransferStatus "Lista de transferências"
// @Router /api/v1/transfers [get]
func ListTransfers(c *gin.Context) {
	statusMutex.RLock()
	defer statusMutex.RUnlock()

	transfers := make([]models.TransferStatus, 0, len(transferStatuses))
	for _, status := range transferStatuses {
		transfers = append(transfers, status)
	}

	c.JSON(http.StatusOK, transfers)
}

// @Summary Cancelar uma transferência em andamento
// @Description Cancela uma transferência de mensagens que está em andamento
// @Tags transfer
// @Produce json
// @Param requestId path string true "ID da requisição de transferência"
// @Success 200 {object} models.TransferResponse "Transferência cancelada"
// @Failure 404 {object} models.TransferResponse "Transferência não encontrada"
// @Failure 400 {object} models.TransferResponse "Transferência já concluída"
// @Router /api/v1/transfer/{requestId}/cancel [post]
func CancelTransfer(c *gin.Context) {
	requestID := c.Param("requestId")

	statusMutex.Lock()
	defer statusMutex.Unlock()

	status, exists := transferStatuses[requestID]
	if !exists {
		c.JSON(http.StatusNotFound, models.TransferResponse{
			Status: "failed",
			Error:  "Transferência não encontrada",
		})
		return
	}

	if status.Status != "in_progress" {
		c.JSON(http.StatusBadRequest, models.TransferResponse{
			Status:    "failed",
			Error:     "Transferência já concluída ou falhou",
			RequestID: requestID,
		})
		return
	}

	transferMgr, exists := transferManagers[requestID]
	if exists {
		transferMgr.Stop()
		status.Status = "cancelled"
		status.EndTime = time.Now().UTC().Format(time.RFC3339)
		transferStatuses[requestID] = status
		delete(transferManagers, requestID)
	}

	c.JSON(http.StatusOK, models.TransferResponse{
		Status:    "cancelled",
		RequestID: requestID,
	})
}
