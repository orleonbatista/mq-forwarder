package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"mq-forwarder-go/api/models"
	"mq-forwarder-go/internal/mqutils"
	"mq-forwarder-go/internal/transfer"
	"mq-forwarder-go/transferstore"
)

// TransferHandler holds dependencies for transfer related endpoints.
type TransferHandler struct {
	store    transferstore.TransferStore
	managers sync.Map // key string -> *transfer.TransferManager
	wg       sync.WaitGroup
}

// NewTransferHandler creates a new handler with the given store.
func NewTransferHandler(s transferstore.TransferStore) *TransferHandler {
	return &TransferHandler{store: s}
}

// monitorInterval controls how often the monitor goroutine checks transfer status.
// It is exported as a variable to allow tests to speed up execution.
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

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, sql.ErrNoRows) || err.Error() == "not found"
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

func toStatus(req transferstore.TransferRequest) models.TransferStatus {
	s := models.TransferStatus{
		RequestID:           req.RequestID,
		Status:              req.Status,
		StartTime:           req.StartTime.UTC().Format(time.RFC3339),
		MessagesTotal:       req.MessagesTotal,
		MessagesTransferred: req.MessagesTransferred,
		BytesTransferred:    int64(req.BytesTransferred),
		Error:               req.Error,
	}
	if req.EndTime != nil {
		s.EndTime = req.EndTime.UTC().Format(time.RFC3339)
	}
	return s
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
func (h *TransferHandler) StartTransfer(c *gin.Context) {
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

	tr := transferstore.TransferRequest{
		RequestID:           requestID,
		Status:              transfer.StatusInProgress,
		StartTime:           time.Now().UTC(),
		MessagesTransferred: 0,
		BufferSize:          options.BufferSize,
		CommitInterval:      commitInterval,
		NonSharedConnection: options.NonSharedConnection,
		SourceQueue:         options.SourceQueue,
		DestinationQueue:    options.DestQueue,
		SourceConnection: transferstore.ConnectionDetails{
			QueueManagerName: request.Source.QueueManagerName,
			ConnectionName:   request.Source.ConnectionName,
			Channel:          request.Source.Channel,
			Username:         request.Source.Username,
		},
		DestinationConnection: transferstore.ConnectionDetails{
			QueueManagerName: request.Destination.QueueManagerName,
			ConnectionName:   request.Destination.ConnectionName,
			Channel:          request.Destination.Channel,
			Username:         request.Destination.Username,
		},
	}
	if err := h.store.Create(tr); err != nil {
		c.JSON(http.StatusInternalServerError, models.TransferResponse{Status: transfer.StatusFailed, Error: "erro ao persistir"})
		return
	}

	h.managers.Store(requestID, transferMgr)

	h.wg.Add(1)
	go h.monitorTransfer(requestID, transferMgr)

	c.JSON(http.StatusAccepted, models.TransferResponse{
		RequestID: requestID,
		Status:    transfer.StatusInProgress,
	})
}

func (h *TransferHandler) monitorTransfer(requestID string, transferMgr *transfer.TransferManager) {
	defer h.wg.Done()
	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()

	for {
		<-ticker.C

		stats := transferMgr.GetStats()
		_ = h.store.UpdateProgress(requestID, int(stats.MessagesTransferred), int(stats.BytesTransferred))

		if stats.Status == transfer.StatusCompleted || stats.Status == transfer.StatusFailed || stats.Status == transfer.StatusCancelled {
			end := stats.EndTime
			var errMsg *string
			if stats.Error != "" {
				errMsg = &stats.Error
			}
			_ = h.store.UpdateStatus(requestID, stats.Status, &end, errMsg)
			h.managers.Delete(requestID)
			return
		}
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
func (h *TransferHandler) GetTransferStatus(c *gin.Context) {
	requestID := c.Param("requestId")

	req, err := h.store.GetByID(requestID)
	if err != nil {
		if isNotFound(err) {
			c.JSON(http.StatusNotFound, models.TransferResponse{Status: transfer.StatusFailed, Error: "Transferência não encontrada"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.TransferResponse{Status: transfer.StatusFailed, Error: "erro ao consultar"})
		return
	}

	c.JSON(http.StatusOK, toStatus(req))
}

// @Summary Listar todas as transferências
// @Description Retorna uma lista com todas as transferências e seus status
// @Tags transfer
// @Produce json
// @Success 200 {array} models.TransferStatus "Lista de transferências"
// @Router /api/v1/transfers [get]
func (h *TransferHandler) ListTransfers(c *gin.Context) {
	limit := 100
	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if oStr := c.Query("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}
	reqs, err := h.store.List(offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.TransferResponse{Status: transfer.StatusFailed, Error: "erro ao listar"})
		return
	}
	statuses := make([]models.TransferStatus, 0, len(reqs))
	for _, r := range reqs {
		statuses = append(statuses, toStatus(r))
	}
	c.JSON(http.StatusOK, statuses)
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
func (h *TransferHandler) CancelTransfer(c *gin.Context) {
	requestID := c.Param("requestId")

	req, err := h.store.GetByID(requestID)
	if err != nil {
		if isNotFound(err) {
			c.JSON(http.StatusNotFound, models.TransferResponse{Status: transfer.StatusFailed, Error: "Transferência não encontrada"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.TransferResponse{Status: transfer.StatusFailed, Error: "erro ao consultar"})
		return
	}

	if req.Status != transfer.StatusInProgress {
		c.JSON(http.StatusBadRequest, models.TransferResponse{Status: transfer.StatusFailed, Error: "Transferência já concluída ou falhou", RequestID: requestID})
		return
	}

	if v, ok := h.managers.Load(requestID); ok {
		mgr := v.(*transfer.TransferManager)
		mgr.Stop()
	}
	now := time.Now().UTC()
	if err := h.store.UpdateStatus(requestID, transfer.StatusCancelled, &now, nil); err != nil {
		c.JSON(http.StatusInternalServerError, models.TransferResponse{Status: transfer.StatusFailed, Error: "erro ao cancelar"})
		return
	}

	h.managers.Delete(requestID)

	c.JSON(http.StatusOK, models.TransferResponse{Status: transfer.StatusCancelled, RequestID: requestID})
}
