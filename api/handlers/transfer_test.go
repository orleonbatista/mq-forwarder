package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"mq-forwarder-go/api/models"
	"mq-forwarder-go/internal/transfer"
	"mq-forwarder-go/transferstore"
	mockstore "mq-forwarder-go/transferstore/mock_transferstore"
)

func newMockHandler(t *testing.T) (*TransferHandler, *mockstore.MockTransferStore, *gomock.Controller) {
	ctrl := gomock.NewController(t)
	store := mockstore.NewMockTransferStore(ctrl)
	h := NewTransferHandler(store)
	monitorInterval = time.Millisecond
	return h, store, ctrl
}

func TestStartTransferInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _, ctrl := newMockHandler(t)
	defer ctrl.Finish()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{"))

	h.StartTransfer(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetStatusNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store, ctrl := newMockHandler(t)
	defer ctrl.Finish()
	store.EXPECT().GetByID("na").Return(transferstore.TransferRequest{}, sql.ErrNoRows)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{gin.Param{Key: "requestId", Value: "na"}}
	h.GetTransferStatus(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404")
	}
}

func TestStartAndCancelTransfer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store, ctrl := newMockHandler(t)
	defer ctrl.Finish()

	req := models.TransferRequest{
		Source:           models.ConnectionDetails{QueueManagerName: "qm1", ConnectionName: "c", Channel: "ch"},
		SourceQueue:      "SQ",
		Destination:      models.ConnectionDetails{QueueManagerName: "qm2", ConnectionName: "c", Channel: "ch"},
		DestinationQueue: "DQ",
	}
	body, _ := json.Marshal(req)
	store.EXPECT().Create(gomock.Any()).Return(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/transfer", bytes.NewBuffer(body))
	h.StartTransfer(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	var resp models.TransferResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	h.mu.Lock()
	h.managers[resp.RequestID] = transfer.NewTransferManager(transfer.TransferOptions{})
	h.mu.Unlock()

	store.EXPECT().GetByID(resp.RequestID).Return(transferstore.TransferRequest{RequestID: resp.RequestID, Status: transfer.StatusInProgress}, nil)
	store.EXPECT().UpdateStatus(resp.RequestID, transfer.StatusCancelled, gomock.Any(), gomock.Nil()).Return(nil)
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{gin.Param{Key: "requestId", Value: resp.RequestID}}
	h.CancelTransfer(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
}

func TestCancelCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store, ctrl := newMockHandler(t)
	defer ctrl.Finish()
	store.EXPECT().GetByID("1").Return(transferstore.TransferRequest{RequestID: "1", Status: transfer.StatusCompleted}, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{gin.Param{Key: "requestId", Value: "1"}}
	h.CancelTransfer(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400")
	}
}

func TestListTransfers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store, ctrl := newMockHandler(t)
	defer ctrl.Finish()
	list := []transferstore.TransferRequest{{RequestID: "a"}, {RequestID: "b"}}
	store.EXPECT().List().Return(list, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.ListTransfers(c)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status")
	}
	var out []models.TransferStatus
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries")
	}
}
