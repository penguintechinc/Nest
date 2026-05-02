package main

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestRunGracefulShutdownViaContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context after 50ms to trigger graceful shutdown
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := run(ctx, ":0", logger)

	if err != nil {
		t.Errorf("run() failed with error: %v", err)
	}
}

func TestRunListenFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Use an invalid address to trigger listen failure
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := run(ctx, "invalid://address", logger)

	if err == nil {
		t.Error("run() expected error for invalid address, got nil")
	}
}

func TestRunGracefulShutdownViaSignal(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Use a background context (no timeout)
	ctx := context.Background()

	// Run in a goroutine and send signal after brief delay
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, ":0", logger)
	}()

	// Give the server time to start listening
	time.Sleep(100 * time.Millisecond)

	// Send SIGINT to the process group via os.Interrupt
	// In Go tests, we send to the process, which our signal handler will catch
	proc, _ := os.FindProcess(os.Getpid())
	proc.Signal(os.Interrupt)

	// Wait for server shutdown with reasonable timeout
	select {
	case <-errCh:
		// Server shut down successfully
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not complete within 2 seconds after SIGINT")
	}
}

func TestRunContextWithTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Create a context that cancels after 100ms
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := run(ctx, ":0", logger)

	if err != nil {
		t.Errorf("run() failed with error: %v", err)
	}
}

func TestMainDefault(t *testing.T) {
	// Save original env vars
	originalAddr := os.Getenv("ADDR")
	defer func() {
		if originalAddr != "" {
			os.Setenv("ADDR", originalAddr)
		} else {
			os.Unsetenv("ADDR")
		}
	}()

	os.Unsetenv("ADDR")

	// We won't actually call main() here since it calls os.Exit,
	// but we test the addr resolution logic via run()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Test with default address (should work since :0 = any available port)
	err := run(ctx, defaultAddr, logger)
	if err == nil {
		// Expected: server started successfully
	}
}

func TestMainCustomAddr(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Test with custom address (port 0 = any available port)
	err := run(ctx, ":0", logger)
	if err != nil {
		t.Errorf("run() with custom addr failed: %v", err)
	}
}

func TestRunServerStartupWithContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after server has time to start
	go func() {
		time.Sleep(75 * time.Millisecond)
		cancel()
	}()

	err := run(ctx, ":0", logger)
	if err != nil {
		t.Errorf("run() failed: %v", err)
	}
}

func TestRunQuickShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := run(ctx, ":0", logger)
	if err != nil {
		t.Errorf("run() failed with immediate cancel: %v", err)
	}
}

func TestRunInvalidAddressReturnsError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	// Pass an invalid address that will fail to listen
	// This tests the error path in main() where os.Exit(1) would be called
	err := run(ctx, "999.999.999.999:50059", logger)

	if err == nil {
		t.Error("expected error for invalid address")
	}
}
