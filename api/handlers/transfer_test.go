package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mq-transfer-go/api/models"
	"mq-transfer-go/internal/transfer"
)

// helper to reset globals
func resetGlobals() {
	statusMutex.Lock()
	transferStatuses = make(map[string]models.TransferStatus)
	transferManagers = make(map[string]*transfer.TransferManager)
	statusMutex.Unlock()
	monitorInterval = time.Millisecond
	statusTTL = time.Millisecond
}

func TestStartTransferInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGlobals()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{"))

	StartTransfer(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTransferHandlersFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGlobals()
	req := models.TransferRequest{
		Source:           models.ConnectionDetails{QueueManagerName: "qm1", ConnectionName: "c", Channel: "ch"},
		SourceQueue:      "SQ",
		Destination:      models.ConnectionDetails{QueueManagerName: "qm2", ConnectionName: "c", Channel: "ch"},
		DestinationQueue: "DQ",
		CommitInterval:   1,
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/transfer", bytes.NewBuffer(body))
	StartTransfer(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	var resp models.TransferResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode err: %v", err)
	}
	id := resp.RequestID

	// check status exists
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{gin.Param{Key: "requestId", Value: id}}
	GetTransferStatus(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("status not found")
	}

	// cancel transfer
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Params = gin.Params{gin.Param{Key: "requestId", Value: id}}
	CancelTransfer(c3)
	if w3.Code != http.StatusOK {
		t.Fatalf("cancel failed: %d", w3.Code)
	}

	// list transfers (should have one or zero)
	time.Sleep(2 * time.Millisecond)
	w4 := httptest.NewRecorder()
	c4, _ := gin.CreateTestContext(w4)
	ListTransfers(c4)
	if w4.Code != http.StatusOK {
		t.Fatalf("list failed")
	}
}

func TestCancelNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGlobals()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{gin.Param{Key: "requestId", Value: "na"}}
	CancelTransfer(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCancelCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGlobals()
	id := "id1"
	statusMutex.Lock()
	transferStatuses[id] = models.TransferStatus{RequestID: id, Status: transfer.StatusCompleted}
	statusMutex.Unlock()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{gin.Param{Key: "requestId", Value: id}}
	CancelTransfer(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetStatusNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGlobals()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{gin.Param{Key: "requestId", Value: "none"}}
	GetTransferStatus(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404")
	}
}

func TestStartTransferWithEnv(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGlobals()
	t.Setenv("BUFFER_SIZE", "2")
	t.Setenv("WORKER_COUNT", "1")
	t.Setenv("BATCH_SIZE", "1")
	req := models.TransferRequest{
		Source:           models.ConnectionDetails{QueueManagerName: "qm1", ConnectionName: "c", Channel: "ch"},
		SourceQueue:      "SQ",
		Destination:      models.ConnectionDetails{QueueManagerName: "qm2", ConnectionName: "c", Channel: "ch"},
		DestinationQueue: "DQ",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/transfer", bytes.NewBuffer(body))
	StartTransfer(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	var resp models.TransferResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// cleanup
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{gin.Param{Key: "requestId", Value: resp.RequestID}}
	CancelTransfer(c2)
}

func TestMonitorTransferCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGlobals()
	id := "mid"
	tm := transfer.NewTransferManager(transfer.TransferOptions{})
	tm.SetStatsForTest(transfer.Stats{Status: transfer.StatusCompleted, EndTime: time.Now()})
	statusMutex.Lock()
	transferStatuses[id] = models.TransferStatus{RequestID: id}
	transferManagers[id] = tm
	statusMutex.Unlock()
	go monitorTransfer(id, tm)
	for i := 0; i < 10; i++ {
		time.Sleep(5 * time.Millisecond)
		statusMutex.RLock()
		_, okMgr := transferManagers[id]
		_, okStat := transferStatuses[id]
		statusMutex.RUnlock()
		if !okMgr && !okStat {
			return
		}
	}
	t.Fatalf("expected cleanup")
}

func TestStartTransferDefaultInterval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGlobals()
	req := models.TransferRequest{
		Source:           models.ConnectionDetails{QueueManagerName: "qm1", ConnectionName: "c", Channel: "ch"},
		SourceQueue:      "SQ",
		Destination:      models.ConnectionDetails{QueueManagerName: "qm2", ConnectionName: "c", Channel: "ch"},
		DestinationQueue: "DQ",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/transfer", bytes.NewBuffer(body))
	StartTransfer(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202")
	}
	var resp models.TransferResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// cleanup
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{gin.Param{Key: "requestId", Value: resp.RequestID}}
	CancelTransfer(c2)
}

func TestResolveBufferSize(t *testing.T) {
	t.Setenv("BUFFER_SIZE", "2")
	if v := resolveBufferSize(0); v != 2 {
		t.Fatalf("env not used")
	}
	if v := resolveBufferSize(3); v != 3 {
		t.Fatalf("param not used")
	}
}

func TestListTransfersWithEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGlobals()
	statusTTL = time.Hour
	statusMutex.Lock()
	transferStatuses["a"] = models.TransferStatus{RequestID: "a"}
	transferStatuses["b"] = models.TransferStatus{RequestID: "b"}
	statusMutex.Unlock()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ListTransfers(c)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status")
	}
	var out []models.TransferStatus
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries")
	}
}

func TestMonitorTransferProgress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGlobals()
	id := "pid"
	tm := transfer.NewTransferManager(transfer.TransferOptions{})
	tm.SetStatsForTest(transfer.Stats{Status: transfer.StatusInProgress})
	statusMutex.Lock()
	transferStatuses[id] = models.TransferStatus{RequestID: id}
	transferManagers[id] = tm
	statusMutex.Unlock()
	go monitorTransfer(id, tm)
	time.Sleep(2 * time.Millisecond)
	tm.SetStatsForTest(transfer.Stats{Status: transfer.StatusCompleted, EndTime: time.Now()})
	time.Sleep(5 * time.Millisecond)
	statusMutex.RLock()
	_, okMgr := transferManagers[id]
	_, okStat := transferStatuses[id]
	statusMutex.RUnlock()
	if okMgr || okStat {
		t.Fatalf("expected cleanup")
	}
}
