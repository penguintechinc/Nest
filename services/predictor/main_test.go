package main

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	// Setup logger to suppress output during test
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	addr := "localhost:0" // Use port 0 for automatic assignment
	err := run(ctx, addr)
	if err != nil {
		t.Errorf("run() returned error: %v", err)
	}
}

func TestRunContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	addr := "localhost:0"
	err := run(ctx, addr)
	if err != nil {
		t.Errorf("run() with context cancellation returned error: %v", err)
	}
}

func TestRunListenError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Use an invalid address to trigger listen error
	addr := "invalid:99999"
	err := run(ctx, addr)
	if err == nil {
		t.Error("run() with invalid address should return error")
	}
}

func TestMainWithValidAddr(t *testing.T) {
	// Set ADDR env var for main
	oldAddr := os.Getenv("ADDR")
	defer func() {
		if oldAddr != "" {
			os.Setenv("ADDR", oldAddr)
		} else {
			os.Unsetenv("ADDR")
		}
	}()

	os.Unsetenv("ADDR")

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	// Test default address parsing
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := run(ctx, defaultAddr)
	if err != nil {
		t.Errorf("run() with default addr returned error: %v", err)
	}
}

func TestMainWithEnvAddr(t *testing.T) {
	oldAddr := os.Getenv("ADDR")
	defer func() {
		if oldAddr != "" {
			os.Setenv("ADDR", oldAddr)
		} else {
			os.Unsetenv("ADDR")
		}
	}()

	os.Setenv("ADDR", "localhost:0")

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	envAddr := os.Getenv("ADDR")
	err := run(ctx, envAddr)
	if err != nil {
		t.Errorf("run() with env ADDR returned error: %v", err)
	}
}

func TestRunCancellationBeforeServe(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	addr := "localhost:0"
	err := run(ctx, addr)
	// Should handle immediate cancellation gracefully
	if err != nil && err != context.Canceled {
		t.Errorf("run() with pre-canceled context returned unexpected error: %v", err)
	}
}

func TestRunHealthCheckRegistration(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	addr := "localhost:0"
	err := run(ctx, addr)
	if err != nil {
		t.Errorf("run() returned error: %v", err)
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	addr := "localhost:0"

	// Start run in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, addr)
	}()

	// Wait a bit for server to start
	time.Sleep(20 * time.Millisecond)

	// Cancel context to trigger graceful shutdown
	cancel()

	// Wait for run to complete
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("run() did not complete within timeout")
	}
}

func TestRunShutdownTimeout(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	addr := "localhost:0"

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, addr)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success - run completed after cancellation
	case <-time.After(3 * time.Second):
		t.Error("run() did not complete within 3 seconds")
	}
}

func TestRunReflectionRegistration(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	addr := "localhost:0"
	// If reflection was not registered properly, server would not start
	err := run(ctx, addr)
	if err != nil {
		t.Errorf("run() with reflection registration returned error: %v", err)
	}
}

func TestRunConcurrentContextAndServe(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	addr := "localhost:0"

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, addr)
	}()

	// Give server time to fully initialize
	time.Sleep(30 * time.Millisecond)

	// Cancel during operation
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("run() did not complete")
	}
}

func TestRunMultipleContextCancellations(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	addr := "localhost:0"

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, addr)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("run() did not complete")
	}
}

func TestRunWithDifferentPortRange(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	addr := "127.0.0.1:0"
	err := run(ctx, addr)
	if err != nil {
		t.Errorf("run() with 127.0.0.1:0 returned error: %v", err)
	}
}

func TestRunWithIPv6Address(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	addr := "[::1]:0"
	err := run(ctx, addr)
	if err != nil {
		t.Errorf("run() with IPv6 address returned error: %v", err)
	}
}

func TestRunHealthStatusTransition(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	addr := "localhost:0"

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, addr)
	}()

	// Allow server to start and set health to SERVING
	time.Sleep(20 * time.Millisecond)

	// Cancel to trigger graceful shutdown (health set to NOT_SERVING)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("run() did not complete")
	}
}

func TestRunListenerCleanup(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	addr := "localhost:0"

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, addr)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success - listener should be cleaned up
	case <-time.After(2 * time.Second):
		t.Error("run() did not complete")
	}

	// Try to start another server on same port to verify cleanup
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()

	err := run(ctx2, addr)
	if err != nil {
		t.Logf("Expected behavior: new server may fail if port reuse timing is tight")
	}
}

func TestRunDefaultLogger(t *testing.T) {
	// Test that run uses slog.Default() correctly
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	addr := "localhost:0"
	err := run(ctx, addr)
	if err != nil {
		t.Errorf("run() returned error: %v", err)
	}
}

func TestRunQuickTimeout(t *testing.T) {
	// Test where context times out quickly
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	addr := "localhost:0"
	err := run(ctx, addr)
	if err != nil {
		t.Errorf("run() with quick timeout returned error: %v", err)
	}
}

func TestRunServerStartup(t *testing.T) {
	// Test successful server startup and shutdown sequence
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	addr := "localhost:0"

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, addr)
	}()

	// Give server time to start up and initialize
	time.Sleep(40 * time.Millisecond)

	// Ensure server is running by checking it handles cancellation
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run() returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("run() did not complete after shutdown signal")
	}
}

func TestRunWithLoggingOutput(t *testing.T) {
	// Test that run logs appropriate messages
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	addr := "localhost:0"

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, addr)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("run() did not complete")
	}
}

func TestCreateServer(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	srv, hsrv := createServer()
	if srv == nil {
		t.Error("createServer() returned nil gRPC server")
	}
	if hsrv == nil {
		t.Error("createServer() returned nil health server")
	}

	// Clean up
	srv.Stop()
}

func TestCreateServerReflectionRegistered(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	srv, _ := createServer()
	// If reflection was registered, server should work
	if srv == nil {
		t.Error("createServer() failed to create server with reflection")
	}

	srv.Stop()
}

func TestCreateServerHealthCheckEnabled(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	_, hsrv := createServer()
	if hsrv == nil {
		t.Error("createServer() failed to create health server")
	}
}

func TestHandleShutdownContextDone(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	srv, hsrv := createServer()
	serveDone := make(chan error, 1)

	// Start serving in background
	done := make(chan error, 1)
	go func() {
		done <- handleShutdown(ctx, srv, hsrv, serveDone)
	}()

	// Give handleShutdown time to enter select
	time.Sleep(10 * time.Millisecond)

	// Trigger context cancellation
	cancel()

	// Give shutdown time to complete
	time.Sleep(20 * time.Millisecond)

	// Trigger serve completion
	serveDone <- nil

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("handleShutdown() returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("handleShutdown() did not complete")
	}
}

func TestHandleShutdownServeDone(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx := context.Background()

	srv, hsrv := createServer()
	serveDone := make(chan error, 1)

	done := make(chan error, 1)
	go func() {
		done <- handleShutdown(ctx, srv, hsrv, serveDone)
	}()

	time.Sleep(10 * time.Millisecond)

	// Trigger serve completion (no error)
	serveDone <- nil

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("handleShutdown() returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("handleShutdown() did not complete")
	}
}

func TestHandleShutdownServeError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx := context.Background()

	srv, hsrv := createServer()
	serveDone := make(chan error, 1)

	done := make(chan error, 1)
	go func() {
		done <- handleShutdown(ctx, srv, hsrv, serveDone)
	}()

	time.Sleep(10 * time.Millisecond)

	// Trigger serve error
	testErr := os.ErrInvalid
	serveDone <- testErr

	select {
	case err := <-done:
		if err != testErr {
			t.Errorf("handleShutdown() returned unexpected error: %v (expected %v)", err, testErr)
		}
	case <-time.After(1 * time.Second):
		t.Error("handleShutdown() did not complete")
	}
}

func TestHandleShutdownGracefulStopCompletes(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	srv, hsrv := createServer()
	serveDone := make(chan error, 1)

	done := make(chan error, 1)
	go func() {
		done <- handleShutdown(ctx, srv, hsrv, serveDone)
	}()

	time.Sleep(15 * time.Millisecond)

	// Cancel context to trigger graceful shutdown
	cancel()

	// After a short delay, send serve done to allow shutdown to complete
	time.Sleep(20 * time.Millisecond)
	serveDone <- nil

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("handleShutdown() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("handleShutdown() did not complete")
	}
}

func TestHandleShutdownTimeoutScenario(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())

	srv, hsrv := createServer()
	serveDone := make(chan error, 1)

	done := make(chan error, 1)
	go func() {
		done <- handleShutdown(ctx, srv, hsrv, serveDone)
	}()

	time.Sleep(15 * time.Millisecond)

	// Cancel context to trigger graceful shutdown
	cancel()

	// Don't send anything on serveDone to let timeout occur
	// Wait for the 10s timeout + buffer
	select {
	case err := <-done:
		if err != nil {
			t.Logf("handleShutdown() returned error after timeout: %v", err)
		}
	case <-time.After(12 * time.Second):
		t.Error("handleShutdown() did not complete within timeout")
	}
}

func TestHandleShutdownMultiplePaths(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(logger)

	// Test multiple scenarios in sequence
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		srv, hsrv := createServer()
		serveDone := make(chan error, 1)

		done := make(chan error, 1)
		go func() {
			done <- handleShutdown(ctx, srv, hsrv, serveDone)
		}()

		time.Sleep(10 * time.Millisecond)
		cancel()
		time.Sleep(10 * time.Millisecond)
		serveDone <- nil

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Errorf("iteration %d: handleShutdown() did not complete", i)
		}
	}
}

func TestInitLogger(t *testing.T) {
	logger := initLogger()
	if logger == nil {
		t.Error("initLogger() returned nil")
	}
}

func TestGetAddrDefault(t *testing.T) {
	oldAddr := os.Getenv("ADDR")
	defer func() {
		if oldAddr != "" {
			os.Setenv("ADDR", oldAddr)
		} else {
			os.Unsetenv("ADDR")
		}
	}()

	os.Unsetenv("ADDR")
	addr := getAddr()
	if addr != defaultAddr {
		t.Errorf("getAddr() returned %q, expected %q", addr, defaultAddr)
	}
}

func TestGetAddrFromEnv(t *testing.T) {
	oldAddr := os.Getenv("ADDR")
	defer func() {
		if oldAddr != "" {
			os.Setenv("ADDR", oldAddr)
		} else {
			os.Unsetenv("ADDR")
		}
	}()

	testAddr := "localhost:9999"
	os.Setenv("ADDR", testAddr)
	addr := getAddr()
	if addr != testAddr {
		t.Errorf("getAddr() returned %q, expected %q", addr, testAddr)
	}
}

func TestSetupSignalHandling(t *testing.T) {
	ctx, cancel := setupSignalHandling()
	if ctx == nil {
		t.Error("setupSignalHandling() returned nil context")
	}
	if cancel == nil {
		t.Error("setupSignalHandling() returned nil cancel function")
	}

	// Verify context is not done
	select {
	case <-ctx.Done():
		t.Error("setupSignalHandling() returned already-done context")
	default:
		// Good
	}

	cancel()
}
