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
	t.Setenv("DB_TYPE", "sqlite")
	t.Setenv("DB_NAME", ":memory:")
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	os.Setenv("ADDR", ":9999")
	defer os.Unsetenv("ENTERPRISE_LICENSE")
	defer os.Unsetenv("ADDR")

	server := newServer(logger)

	if server == nil {
		t.Errorf("expected server to be created")
	}

	if server.Addr != ":9999" {
		t.Errorf("expected addr :9999, got %s", server.Addr)
	}

	if server.Handler == nil {
		t.Errorf("expected handler to be set")
	}
}

func TestNewServerDefaultAddr(t *testing.T) {
	t.Setenv("DB_TYPE", "sqlite")
	t.Setenv("DB_NAME", ":memory:")
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	os.Unsetenv("ADDR")

	server := newServer(logger)

	if server.Addr != ":8086" {
		t.Errorf("expected default addr :8086, got %s", server.Addr)
	}
}

func TestNewServerTimeouts(t *testing.T) {
	t.Setenv("DB_TYPE", "sqlite")
	t.Setenv("DB_NAME", ":memory:")
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	server := newServer(logger)

	if server.ReadTimeout.Seconds() != 15 {
		t.Errorf("expected read timeout 15s, got %v", server.ReadTimeout)
	}

	if server.WriteTimeout.Seconds() != 15 {
		t.Errorf("expected write timeout 15s, got %v", server.WriteTimeout)
	}

	if server.IdleTimeout.Seconds() != 60 {
		t.Errorf("expected idle timeout 60s, got %v", server.IdleTimeout)
	}
}

func TestNewServerHandlerNotNil(t *testing.T) {
	t.Setenv("DB_TYPE", "sqlite")
	t.Setenv("DB_NAME", ":memory:")
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	server := newServer(logger)

	if server.Handler == nil {
		t.Errorf("expected handler to be initialized")
	}

	// Verify handler responds
	req, _ := http.NewRequest("GET", "http://localhost:8087/healthz", nil)
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
	t.Setenv("DB_TYPE", "sqlite")
	t.Setenv("DB_NAME", ":memory:")
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Create server on a random port
	os.Setenv("ADDR", ":0")
	defer os.Unsetenv("ADDR")

	server := &http.Server{
		Addr:         ":0",
		Handler:      NewMux(NewSCIMStore(getTestDAL(), logger), logger),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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
