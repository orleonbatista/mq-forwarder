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

func TestInitOTelFailures(t *testing.T) {
	FailResourceMerge = true
	if _, err := InitOTel(OTelConfig{OTLPEndpoint: "x"}); err == nil {
		t.Fatalf("expected resource error")
	}
	FailResourceMerge = false

	FailExporter = true
	if _, err := InitOTel(OTelConfig{OTLPEndpoint: "x"}); err == nil {
		t.Fatalf("expected exporter error")
	}
	FailExporter = false

	FailMsgCounter = true
	if _, err := InitOTel(OTelConfig{ServiceName: "s", ServiceVersion: "v", Environment: "e", OTLPEndpoint: "x"}); err == nil {
		t.Fatalf("expected msg counter error")
	}
	FailMsgCounter = false

	FailBytesCounter = true
	if _, err := InitOTel(OTelConfig{ServiceName: "s", ServiceVersion: "v", Environment: "e", OTLPEndpoint: "x"}); err == nil {
		t.Fatalf("expected bytes counter error")
	}
	FailBytesCounter = false

	FailDurationHistogram = true
	if _, err := InitOTel(OTelConfig{ServiceName: "s", ServiceVersion: "v", Environment: "e", OTLPEndpoint: "x"}); err == nil {
		t.Fatalf("expected duration histogram error")
	}
	FailDurationHistogram = false

	FailCommitCounter = true
	if _, err := InitOTel(OTelConfig{ServiceName: "s", ServiceVersion: "v", Environment: "e", OTLPEndpoint: "x"}); err == nil {
		t.Fatalf("expected commit counter error")
	}
	FailCommitCounter = false

	FailErrorCounter = true
	if _, err := InitOTel(OTelConfig{ServiceName: "s", ServiceVersion: "v", Environment: "e", OTLPEndpoint: "x"}); err == nil {
		t.Fatalf("expected error counter error")
	}
	FailErrorCounter = false
}
