package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type pingStore struct{ err error }

func (p pingStore) Ping() error { return p.err }

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	HealthStore = pingStore{}
	defer func() { HealthStore = nil }()

	HealthCheck(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "\"database\":\"ok\"") {
		t.Fatalf("database not ok: %s", w.Body.String())
	}
}

func TestHealthCheckFail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	HealthStore = pingStore{err: http.ErrServerClosed}
	defer func() { HealthStore = nil }()

	HealthCheck(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "\"database\":\"unhealthy\"") {
		t.Fatalf("database status not unhealthy: %s", w.Body.String())
	}
}
