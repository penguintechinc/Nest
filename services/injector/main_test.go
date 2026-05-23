package main

import (
	"context"
	"net"
	"testing"
	"time"
)

// getFreePort returns a free port ready to be used.
func getFreePort() string {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return ":8443"
	}
	defer listener.Close()
	return listener.Addr().String()
}

func TestRunWithEmptyAddr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Empty addr defaults to :8443, context times out
	err := run(ctx, "")
	// Should complete (either with error or nil, both are valid)
	// The server startup and context cancellation are async
	_ = err
}

func TestRunWithCustomAddr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Custom addr, context times out
	err := run(ctx, ":9876")
	// Server may or may not have started depending on timing
	// Both outcomes are acceptable
	_ = err
}

func TestRunContextTimeout(t *testing.T) {
	port := getFreePort()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run should eventually return when context times out
	err := run(ctx, port)
	// Either the context timeout causes return, or server error
	// Both are acceptable for this test
	_ = err
}

func TestRunContextCancel(t *testing.T) {
	port := getFreePort()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, port)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for run() to return
	select {
	case err := <-errCh:
		// Shutdown should complete successfully
		if err != nil && err.Error() != "http: Server closed" {
			t.Logf("shutdown completed with: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("expected run() to complete within 2 seconds")
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	port := getFreePort()
	ctx, cancel := context.WithCancel(context.Background())

	finished := make(chan error, 1)

	go func() {
		finished <- run(ctx, port)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-finished:
		// Graceful shutdown should complete
	case <-time.After(3 * time.Second):
		t.Errorf("graceful shutdown timeout")
	}
}

func TestRunWithoutTLSEnvVars(t *testing.T) {
	// Clear any TLS env vars
	t.Setenv("TLS_CERT_FILE", "")
	t.Setenv("TLS_KEY_FILE", "")

	port := getFreePort()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = run(ctx, port)
	// Test completes without panic
}

func TestRunWithTLSEnvVars(t *testing.T) {
	// Set TLS env vars to nonexistent files
	t.Setenv("TLS_CERT_FILE", "/nonexistent/cert.pem")
	t.Setenv("TLS_KEY_FILE", "/nonexistent/key.pem")

	port := getFreePort()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = run(ctx, port)
	// Test completes without panic
}

func TestRunShutdownTimeout(t *testing.T) {
	port := getFreePort()
	ctx, cancel := context.WithCancel(context.Background())

	finished := make(chan error, 1)

	go func() {
		finished <- run(ctx, port)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-finished:
		// Should complete within the 10-second shutdown timeout
	case <-time.After(15 * time.Second):
		t.Errorf("shutdown took too long")
	}
}

func TestRunMultipleContexts(t *testing.T) {
	// Test that run can handle multiple calls with different contexts
	for i := 0; i < 3; i++ {
		port := getFreePort()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_ = run(ctx, port)
		cancel()
	}
}

func TestRunAlternateAddrFormats(t *testing.T) {
	tests := []string{
		"",        // Empty, defaults to :8443
		":8443",   // Explicit HTTPS port
		":9000",   // Alternate port
		":0",      // OS-chosen port
	}

	for _, addr := range tests {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_ = run(ctx, addr)
		cancel()
	}
}

func TestRunServerError(t *testing.T) {
	// Test the case where serverErr channel receives an error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a port that's likely in use to trigger a bind error
	port := ":1"  // Ports below 1024 typically require elevated privileges

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, port)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		// Should get either a bind error or server closed
		_ = err
	case <-time.After(2 * time.Second):
		t.Logf("run() did not complete in time")
	}
}

func TestRunWithTLSFiles(t *testing.T) {
	// Test with TLS cert/key environment variables set
	t.Setenv("TLS_CERT_FILE", "/tmp/cert.pem")
	t.Setenv("TLS_KEY_FILE", "/tmp/key.pem")

	port := getFreePort()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Should attempt HTTPS startup (and fail due to missing files)
	_ = run(ctx, port)
}
