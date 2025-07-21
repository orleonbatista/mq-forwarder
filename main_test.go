package main

import (
	"context"
	"errors"
	"log"
	"mq-transfer-go/internal/otelutils"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestMainFunction(t *testing.T) {
	os.Setenv("OTLP_ENDPOINT", "")
	otelInit = func(otelutils.OTelConfig) (*otelutils.MQMetrics, error) {
		return nil, errors.New("init")
	}
	defer func() { otelInit = otelutils.InitOTel }()
	serverAddr = ":18080"
	defer func() { serverAddr = ":8080" }()
	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	// wait until health endpoint responds
	var err error
	url := "http://localhost" + serverAddr + "/api/v1/health"
	for i := 0; i < 10; i++ {
		var resp *http.Response
		resp, err = http.Get(url)
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not start: %v", err)
	}

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := p.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("main did not exit in time")
	}
}

func TestMainListenError(t *testing.T) {
	os.Setenv("OTLP_ENDPOINT", "")
	serverAddr = ":18081"
	ln, err := net.Listen("tcp", serverAddr)
	if err != nil {
		t.Fatalf("failed to listen on port: %v", err)
	}
	defer ln.Close()

	defer func() { serverAddr = ":8080" }()

	called := false
	logFatalf = func(string, ...interface{}) { called = true }
	serverShutdown = func(*http.Server, context.Context) error {
		called = true
		return errors.New("shutdown failure")
	}
	defer func() {
		serverShutdown = func(srv *http.Server, ctx context.Context) error { return srv.Shutdown(ctx) }
	}()
	defer func() { logFatalf = log.Fatalf }()

	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	p, _ := os.FindProcess(os.Getpid())
	p.Signal(syscall.SIGINT)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("main did not exit")
	}
	if !called {
		t.Fatal("expected logFatalf to be called")
	}
}
