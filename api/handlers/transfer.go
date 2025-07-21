package handlers

import (
	"net/http"
	"os"
	"strconv"
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

// statusTTL defines how long finished transfer statuses are kept in memory.
// It is a variable so tests can shorten the duration.
var statusTTL = 10 * time.Minute

// monitorInterval controls how often the monitor goroutine checks transfer
// status. It is exported as a variable to allow tests to speed up execution.
var monitorInterval = time.Second

func parseTransferRequest(c *gin.Context) (models.TransferRequest, bool) {
	var req models.TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.TransferResponse{
			Status: transfer.StatusFailed,
			Error:  "Erro ao processar requisição: " + err.Error(),
		})
		return models.TransferRequest{}, false
	}
	return req, true
}

func resolveBufferSize(size int) int {
	if size > 0 {
		return size
	}
	if bsEnv := os.Getenv("BUFFER_SIZE"); bsEnv != "" {
		if v, err := strconv.Atoi(bsEnv); err == nil && v > 0 {
			return v
		}
	}
	return 1048576
}

func resolveWorkerCount() int {
	if wcEnv := os.Getenv("WORKER_COUNT"); wcEnv != "" {
		if v, err := strconv.Atoi(wcEnv); err == nil && v > 0 {
			return v
		}
	}
	return 0
}

func resolveCommitInterval(interval int) int {
	if interval > 0 {
		return interval
	}
	if ciEnv := os.Getenv("BATCH_SIZE"); ciEnv != "" {
		if v, err := strconv.Atoi(ciEnv); err == nil && v > 0 {
			return v
		}
	}
	return 10
}

func buildTransferOptions(req models.TransferRequest, commitInterval, workerCount int) transfer.TransferOptions {
	return transfer.TransferOptions{
		SourceConfig: mqutils.MQConnectionConfig{
			QueueManagerName:    req.Source.QueueManagerName,
			ConnectionName:      req.Source.ConnectionName,
			Channel:             req.Source.Channel,
			Username:            req.Source.Username,
			Password:            req.Source.Password,
			NonSharedConnection: req.NonSharedConnection,
		},
		SourceQueue: req.SourceQueue,
		DestConfig: mqutils.MQConnectionConfig{
			QueueManagerName: req.Destination.QueueManagerName,
			ConnectionName:   req.Destination.ConnectionName,
			Channel:          req.Destination.Channel,
			Username:         req.Destination.Username,
			Password:         req.Destination.Password,
		},
		DestQueue:           req.DestinationQueue,
		BufferSize:          req.BufferSize,
		CommitInterval:      commitInterval,
		NonSharedConnection: req.NonSharedConnection,
		WorkerCount:         workerCount,
	}
}

func newTransferStatus(id string) models.TransferStatus {
	return models.TransferStatus{
		RequestID:           id,
		Status:              transfer.StatusInProgress,
		StartTime:           time.Now().UTC().Format(time.RFC3339),
		MessagesTransferred: 0,
	}
}

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
	request, ok := parseTransferRequest(c)
	if !ok {
		return
	}

	requestID := uuid.New().String()

	request.BufferSize = resolveBufferSize(request.BufferSize)
	workerCount := resolveWorkerCount()
	commitInterval := resolveCommitInterval(request.CommitInterval)

	options := buildTransferOptions(request, commitInterval, workerCount)

	transferMgr := transfer.NewTransferManager(options)
	transferMgr.Start()

	status := newTransferStatus(requestID)

	statusMutex.Lock()
	transferStatuses[requestID] = status
	transferManagers[requestID] = transferMgr
	statusMutex.Unlock()

	go monitorTransfer(requestID, transferMgr)

	c.JSON(http.StatusAccepted, models.TransferResponse{
		RequestID: requestID,
		Status:    transfer.StatusInProgress,
	})
}

func monitorTransfer(requestID string, transferMgr *transfer.TransferManager) {
	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()

	for {
		<-ticker.C

		stats := transferMgr.GetStats()

		statusMutex.Lock()
		status := transferStatuses[requestID]
		status.MessagesTransferred = int(stats.MessagesTransferred)
		status.BytesTransferred = stats.BytesTransferred
		status.Status = stats.Status

		if stats.Status == transfer.StatusCompleted || stats.Status == transfer.StatusFailed || stats.Status == transfer.StatusCancelled {
			status.EndTime = stats.EndTime.UTC().Format(time.RFC3339)
			status.Error = stats.Error
			transferStatuses[requestID] = status
			delete(transferManagers, requestID)
			statusMutex.Unlock()
			time.AfterFunc(statusTTL, func() {
				statusMutex.Lock()
				delete(transferStatuses, requestID)
				statusMutex.Unlock()
			})
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
			Status: transfer.StatusFailed,
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
			Status: transfer.StatusFailed,
			Error:  "Transferência não encontrada",
		})
		return
	}

	if status.Status != transfer.StatusInProgress {
		c.JSON(http.StatusBadRequest, models.TransferResponse{
			Status:    transfer.StatusFailed,
			Error:     "Transferência já concluída ou falhou",
			RequestID: requestID,
		})
		return
	}

	transferMgr, exists := transferManagers[requestID]
	if exists {
		transferMgr.Stop()
		status.Status = transfer.StatusCancelled
		status.EndTime = time.Now().UTC().Format(time.RFC3339)
		transferStatuses[requestID] = status
		delete(transferManagers, requestID)
	}

	c.JSON(http.StatusOK, models.TransferResponse{
		Status:    transfer.StatusCancelled,
		RequestID: requestID,
	})
}
