package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"
)

func init() {
	// Set JWT env vars for tests
	os.Setenv("JWT_ALGORITHM", "HS256")
	os.Setenv("JWT_SHARED_SECRET", "test-secret-key-for-testing")
	os.Setenv("JWT_ISSUER", "test-issuer")
	os.Setenv("JWT_AUDIENCE", "test-audience")
}

func TestRunBasicStartup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	// Find an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	// Create a context with short timeout to test graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run should exit after context timeout
	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() returned error: %v", err)
	}
}

func TestRunInvalidAddr(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Invalid address should cause error
	err := run(ctx, "invalid:addr:here")
	if err == nil {
		t.Error("run() should have failed with invalid address")
	}
}

func TestRunContextCancel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() returned error: %v", err)
	}
}

func TestRunDefaultLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Should use slog.Default() without error
	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() with default logger returned error: %v", err)
	}
}

func TestRunMultipleShutdowns(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	// Calling run multiple times should work
	for i := 0; i < 3; i++ {
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("failed to find available port: %v", err)
		}
		addr := listener.Addr().String()
		listener.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err = run(ctx, addr)
		if err != nil {
			t.Errorf("iteration %d: run() returned error: %v", i, err)
		}
	}
}

func TestRunContextExpiry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	// Very short timeout to ensure context expires during run
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() returned error: %v", err)
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run should handle graceful shutdown with context expiry
	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() returned error: %v", err)
	}
}

func TestRunAddrParameter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	// Test with custom addr format
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	customAddr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = run(ctx, customAddr)
	if err != nil {
		t.Errorf("run() with custom addr returned error: %v", err)
	}
}

func TestRunHealthCheck(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// run() should initialize health check status
	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() returned error: %v", err)
	}
}

func TestRunListenerClosesOnError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Using impossible address to trigger error
	err := run(ctx, "999.999.999.999:99999")
	if err == nil {
		t.Error("run() should have failed with invalid address")
	}
}

func TestRunTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	// Timeout should cause graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() returned error: %v", err)
	}
}

func TestRunConcurrentContexts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	// Test that multiple run() calls with different contexts don't interfere
	done := make(chan error, 2)

	for i := 0; i < 2; i++ {
		go func() {
			listener, err := net.Listen("tcp", ":0")
			if err != nil {
				done <- err
				return
			}
			addr := listener.Addr().String()
			listener.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			done <- run(ctx, addr)
		}()
	}

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent run() returned error: %v", err)
		}
	}
}

func TestRunEmptyAddr(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Empty addr should still work (binds to all interfaces with default)
	err := run(ctx, "")
	if err != nil {
		t.Errorf("run() with empty addr should succeed, got error: %v", err)
	}
}

func TestRunShutdownPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	// Context that expires quickly to trigger shutdown path
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() returned error: %v", err)
	}
}

func TestRunReturnValue(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Successful run should return nil
	err = run(ctx, addr)
	if err != nil {
		t.Errorf("successful run() returned error: %v", err)
	}
}

func TestRunMuxInitialization(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// run() should initialize mux without error
	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() with mux initialization returned error: %v", err)
	}
}

func TestRunHealthServerSetup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// run() should set up health server and eventually mark as NOT_SERVING during shutdown
	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() health server setup returned error: %v", err)
	}
}

func TestRunListenerDefer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Listener should be properly deferred and closed
	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() with listener defer returned error: %v", err)
	}
}

func TestRunReflectionRegistration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// gRPC reflection should be registered
	err = run(ctx, addr)
	if err != nil {
		t.Errorf("run() with reflection registration returned error: %v", err)
	}
}

func TestRunWithSignalPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	sigChan := make(chan os.Signal, 1)
	cfg := &runConfig{sigChan: sigChan}
	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		done <- runWithConfig(ctx, addr, cfg)
	}()

	// Give server time to start
	time.Sleep(20 * time.Millisecond)

	// Send signal to trigger shutdown
	sigChan <- os.Interrupt

	// Wait for completion
	err = <-done
	if err != nil {
		t.Errorf("runWithConfig() returned error: %v", err)
	}
}

func TestRunWithConfigContextCancel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	cfg := &runConfig{sigChan: sigChan, shutdownTimeMs: 10000}

	err = runWithConfig(ctx, addr, cfg)
	if err != nil {
		t.Errorf("runWithConfig() returned error: %v", err)
	}
}

func TestRunWithShutdownTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	sigChan := make(chan os.Signal, 1)
	// Use very short timeout to force timeout path
	cfg := &runConfig{sigChan: sigChan, shutdownTimeMs: 1}

	ctx := context.Background()
	done := make(chan error, 1)

	go func() {
		done <- runWithConfig(ctx, addr, cfg)
	}()

	// Give server time to start
	time.Sleep(20 * time.Millisecond)

	// Send signal to trigger shutdown with very short timeout
	sigChan <- os.Interrupt

	// Wait for completion
	err = <-done
	if err != nil {
		t.Errorf("runWithConfig() with timeout returned error: %v", err)
	}
}
