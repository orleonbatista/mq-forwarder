package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
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

func TestStartTransferStoreError(t *testing.T) {
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
	store.EXPECT().Create(gomock.Any()).Return(errors.New("fail"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/transfer", bytes.NewBuffer(body))
	h.StartTransfer(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
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
	c.Request = httptest.NewRequest(http.MethodGet, "/transfer/na", nil)
	h.GetTransferStatus(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404")
	}
}

func TestGetStatusError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store, ctrl := newMockHandler(t)
	defer ctrl.Finish()
	store.EXPECT().GetByID("na").Return(transferstore.TransferRequest{}, errors.New("boom"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{gin.Param{Key: "requestId", Value: "na"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/transfer/na", nil)
	h.GetTransferStatus(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500")
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
	store.EXPECT().UpdateProgress(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/transfer", bytes.NewBuffer(body))
	h.StartTransfer(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	var apiResp models.APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &apiResp)
	dataBytes, _ := json.Marshal(apiResp.Data)
	var resp models.TransferResponse
	_ = json.Unmarshal(dataBytes, &resp)

	store.EXPECT().GetByID(resp.RequestID).Return(transferstore.TransferRequest{RequestID: resp.RequestID, Status: transfer.StatusInProgress}, nil)
	store.EXPECT().UpdateStatus(resp.RequestID, transfer.StatusCancelled, gomock.Any(), gomock.Nil()).AnyTimes()
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{gin.Param{Key: "requestId", Value: resp.RequestID}}
	c2.Request = httptest.NewRequest(http.MethodPost, "/transfer/"+resp.RequestID+"/cancel", nil)
	h.CancelTransfer(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	h.wg.Wait()
}

func TestCancelCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store, ctrl := newMockHandler(t)
	defer ctrl.Finish()
	store.EXPECT().GetByID("1").Return(transferstore.TransferRequest{RequestID: "1", Status: transfer.StatusCompleted}, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{gin.Param{Key: "requestId", Value: "1"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/transfer/1/cancel", nil)
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
	store.EXPECT().List(transferstore.ListParams{Limit: 100}).Return(list, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	h.ListTransfers(c)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status")
	}
	var out models.APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	data, _ := json.Marshal(out.Data)
	var statuses []models.TransferStatus
	_ = json.Unmarshal(data, &statuses)
	if len(statuses) != 2 {
		t.Fatalf("expected 2 entries")
	}
}

func TestListTransfersWithParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store, ctrl := newMockHandler(t)
	defer ctrl.Finish()
	list := []transferstore.TransferRequest{{RequestID: "c"}}
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	params := transferstore.ListParams{Offset: 5, Limit: 1, Status: "completed", StartTime: &start, EndTime: &end, Order: "asc"}
	store.EXPECT().List(params).Return(list, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?limit=1&offset=5&status=completed&start="+start.Format(time.RFC3339)+"&end="+end.Format(time.RFC3339)+"&order=asc", nil)
	h.ListTransfers(c)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status")
	}
	var out2 models.APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &out2)
	data2, _ := json.Marshal(out2.Data)
	var statuses2 []models.TransferStatus
	_ = json.Unmarshal(data2, &statuses2)
	if len(statuses2) != 1 || statuses2[0].RequestID != "c" {
		t.Fatalf("expected filtered result")
	}
}
