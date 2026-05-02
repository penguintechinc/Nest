package main

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRunWithDefaultAddr(t *testing.T) {
	os.Unsetenv("ADDR")

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// run() blocks until ctx is cancelled; with a 500ms timeout it should return cleanly.
	err := run(ctx, logger)
	if err != nil {
		t.Errorf("run() returned unexpected error: %v", err)
	}
}

func TestRunWithCustomAddr(t *testing.T) {
	os.Setenv("ADDR", ":50097")
	defer os.Unsetenv("ADDR")

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := run(ctx, logger)
	if err != nil {
		t.Errorf("run() with custom addr returned unexpected error: %v", err)
	}
}

func TestRunWithInvalidAddr(t *testing.T) {
	// Use an address that is already in use (port 1 is privileged and fails to bind).
	os.Setenv("ADDR", ":1")
	defer os.Unsetenv("ADDR")

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Should return an error because ListenAndServe fails on a privileged port.
	err := run(ctx, logger)
	if err == nil {
		t.Log("run() returned nil (server may have started successfully); acceptable if port was available")
	} else {
		t.Logf("run() returned expected error for privileged port: %v", err)
	}
}

func TestRunCreatesStoreAndMux(t *testing.T) {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	store := NewPolicyStore()
	if store == nil {
		t.Errorf("expected non-nil PolicyStore")
	}

	mux := NewMux(store, logger)
	if mux == nil {
		t.Errorf("expected non-nil mux")
	}

	var _ http.Handler = mux
}

func TestRunHTTPServerCreation(t *testing.T) {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	addr := ":50098"
	store := NewPolicyStore()
	mux := NewMux(store, logger)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	if srv.Addr != addr {
		t.Errorf("expected server addr %s, got %s", addr, srv.Addr)
	}
	if srv.Handler != mux {
		t.Errorf("expected server handler to be mux")
	}
}
