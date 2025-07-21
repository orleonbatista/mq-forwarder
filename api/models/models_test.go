package models

import "testing"

func TestNewHealthResponse(t *testing.T) {
	resp := NewHealthResponse("ok", "1.0.0")
	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %s", resp.Status)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %s", resp.Version)
	}
}
