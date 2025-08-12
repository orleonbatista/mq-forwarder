package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mq-forwarder-go/api/models"
	sqlitestore "mq-forwarder-go/transferstore/sqlite"
)

func TestIntegrationSQLite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := sqlitestore.NewSQLiteStore("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	h := NewTransferHandler(store)
	monitorInterval = time.Millisecond

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
	h.StartTransfer(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	var apiResp models.APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &apiResp)
	data, _ := json.Marshal(apiResp.Data)
	var resp models.TransferResponse
	_ = json.Unmarshal(data, &resp)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{gin.Param{Key: "requestId", Value: resp.RequestID}}
	h.GetTransferStatus(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("status not found")
	}

	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	h.ListTransfers(c3)
	if w3.Code != http.StatusOK {
		t.Fatalf("list failed")
	}

	w4 := httptest.NewRecorder()
	c4, _ := gin.CreateTestContext(w4)
	c4.Params = gin.Params{gin.Param{Key: "requestId", Value: resp.RequestID}}
	h.CancelTransfer(c4)
	if w4.Code != http.StatusOK {
		t.Fatalf("cancel failed")
	}

	h.wg.Wait()
}
