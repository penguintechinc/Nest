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

	// Start run in a goroutine and send signal after a short delay
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(logger, ":50056", sigCh)
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Send shutdown signal
	sigCh <- os.Interrupt

	// Wait for run to complete
	select {
	case err := <-errCh:
		// Server shutdown should not return an error
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

	// Start run in a goroutine with empty addr (should default to :50056)
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(logger, "", sigCh)
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Send shutdown signal
	sigCh <- os.Interrupt

	// Wait for run to complete
	select {
	case err := <-errCh:
		// Server shutdown should not return an error
		if err != nil && err.Error() != "http: Server closed" {
			t.Errorf("expected no error or 'Server closed', got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not shutdown within timeout")
	}
}
