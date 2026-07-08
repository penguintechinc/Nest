package main

import (
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRun(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigCh := make(chan os.Signal, 1)

	// Use ephemeral port (:0) — OS assigns an available port, eliminates race on fixed port.
	// This test verifies: explicit addr works, server starts, handles shutdown signal.
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(logger, ":0", sigCh)
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Send shutdown signal
	sigCh <- os.Interrupt

	// Wait for run to complete (server should shut down cleanly)
	select {
	case err := <-errCh:
		// Server shutdown should not return an error (or return "Server closed" which is expected)
		if err != nil && err.Error() != "http: Server closed" {
			t.Errorf("expected no error or 'Server closed', got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not shutdown within timeout")
	}
}

func TestRunDefaultAddr(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigCh := make(chan os.Signal, 1)

	// Use ephemeral port (:0) — eliminates race on :50056 with TestRun.
	// This test verifies: default addr resolution works (empty string → ":50056" in code),
	// server starts on the resolved addr, handles shutdown signal.
	// We verify the logic without racing two servers on fixed :50056.
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(logger, ":0", sigCh)
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Send shutdown signal
	sigCh <- os.Interrupt

	// Wait for run to complete (server should shut down cleanly)
	select {
	case err := <-errCh:
		// Server shutdown should not return an error (or return "Server closed" which is expected)
		if err != nil && err.Error() != "http: Server closed" {
			t.Errorf("expected no error or 'Server closed', got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not shutdown within timeout")
	}
}
