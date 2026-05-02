package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRunServerShutdown(t *testing.T) {
	// Create a test logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Sync()

	// Create a context that cancels after 50ms
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run the server with a test address
	err = run(ctx, ":0", logger) // :0 lets OS choose a free port

	// Should return context.Canceled or context.DeadlineExceeded
	if err != context.Canceled && err != context.DeadlineExceeded {
		t.Errorf("expected context.Canceled or context.DeadlineExceeded, got %v", err)
	}
}

func TestRunServerWithCanceledContext(t *testing.T) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Sync()

	// Create a context that is already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Run should exit immediately
	err = run(ctx, ":0", logger)

	// Should return context.Canceled
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunServerWithDeadlineExceeded(t *testing.T) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Sync()

	// Create a context with a short deadline
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run should exit when deadline is exceeded
	err = run(ctx, ":0", logger)

	// Should return context.DeadlineExceeded or context.Canceled
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Errorf("expected context.DeadlineExceeded or context.Canceled, got %v", err)
	}
}

func TestRunServerValidation(t *testing.T) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Sync()

	// Test with a very short timeout to ensure immediate cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Run should handle the already-canceled context gracefully
	err = run(ctx, ":0", logger)

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestNewMuxIntegration(t *testing.T) {
	// Test that the mux is properly created and functional
	logger := zap.NewNop()
	store := NewLineageStore()
	mux := NewMux(store, logger)

	// Create a test server with the mux
	server := httptest.NewServer(mux)
	defer server.Close()

	// Make a simple request to verify the server works
	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestRunWithInvalidAddress(t *testing.T) {
	// Test run function with invalid address to exercise error handling
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Use an invalid address like "invalid_address_that_wont_bind"
	// This will cause ListenAndServe to fail in the background goroutine
	err = run(ctx, "invalid:::port", logger)

	// Should still return a context error (we wait for context to cancel despite the bad address)
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Errorf("expected context error, got %v", err)
	}
}
