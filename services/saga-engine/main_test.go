package main

import (
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewServer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	os.Setenv("ADDR", ":8888")
	defer os.Unsetenv("ADDR")

	server := newServer(logger)

	if server == nil {
		t.Errorf("expected server to be created")
	}

	if server.Addr != ":8888" {
		t.Errorf("expected addr :8888, got %s", server.Addr)
	}

	if server.Handler == nil {
		t.Errorf("expected handler to be set")
	}
}

func TestNewServerDefaultAddr(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	os.Unsetenv("ADDR")

	server := newServer(logger)

	if server.Addr != ":50055" {
		t.Errorf("expected default addr :50055, got %s", server.Addr)
	}
}

func TestNewServerTimeouts(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	server := newServer(logger)

	if server.ReadTimeout.Seconds() != 30 {
		t.Errorf("expected read timeout 30s, got %v", server.ReadTimeout)
	}

	if server.WriteTimeout.Seconds() != 60 {
		t.Errorf("expected write timeout 60s, got %v", server.WriteTimeout)
	}
}

func TestNewServerHandlerNotNil(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	server := newServer(logger)

	if server.Handler == nil {
		t.Errorf("expected handler to be initialized")
	}

	// Verify handler responds
	req, _ := http.NewRequest("GET", "http://localhost:50055/healthz", nil)
	resp := &mockResponseWriter{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}

	server.Handler.ServeHTTP(resp, req)

	if resp.statusCode != http.StatusOK {
		t.Errorf("expected healthz to return 200, got %d", resp.statusCode)
	}
}

type mockResponseWriter struct {
	header     http.Header
	statusCode int
	body       []byte
}

func (m *mockResponseWriter) Header() http.Header {
	return m.header
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	m.body = append(m.body, b...)
	return len(b), nil
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

func TestServeAndWait(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	server := &http.Server{
		Addr:         ":9876",
		Handler:      NewMux(NewSagaStore(logger), logger),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	// Track if serveAndWait completes
	done := make(chan bool, 1)
	go func() {
		serveAndWait(logger, server)
		done <- true
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Send SIGINT to trigger shutdown
	proc, _ := os.FindProcess(os.Getpid())
	proc.Signal(syscall.SIGINT)

	// Wait for serveAndWait to return (with timeout to prevent hanging)
	select {
	case <-done:
		// Success - serveAndWait completed
	case <-time.After(5 * time.Second):
		t.Errorf("serveAndWait did not complete within timeout")
	}
}
