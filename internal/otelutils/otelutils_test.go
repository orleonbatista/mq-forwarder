package otelutils

import (
	"context"
	"testing"
)

func TestInitOTelNoEndpoint(t *testing.T) {
	metrics, err := InitOTel(OTelConfig{})
	if err != nil || metrics != nil {
		t.Fatalf("expected nil metrics and no error")
	}
	if GetMetrics() != nil {
		t.Fatalf("expected GetMetrics nil")
	}
}

func TestInitOTelWithEndpoint(t *testing.T) {
	m, err := InitOTel(OTelConfig{ServiceName: "s", ServiceVersion: "1", Environment: "t", OTLPEndpoint: "localhost:4317"})
	if err != nil || m == nil {
		t.Fatalf("unexpected init failure: %v", err)
	}
	if GetMetrics() == nil {
		t.Fatalf("metrics not set")
	}
	Shutdown(context.Background())
}
