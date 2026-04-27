package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	// Create a context that cancels after a short delay
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Use a random available port
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := lis.Addr().String()
	lis.Close()

	// Run the server
	err = run(ctx, addr, logger)
	if err != nil {
		t.Fatalf("run() returned error: %v", err)
	}
}

func TestRunListenError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Try to listen on an invalid address
	err := run(ctx, "invalid.address:99999", logger)
	if err == nil {
		t.Fatalf("run() should return error for invalid address")
	}
}

func TestRunWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Use a random available port
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := lis.Addr().String()
	lis.Close()

	// Run the server with already-cancelled context
	err = run(ctx, addr, logger)
	if err != nil {
		t.Fatalf("run() should not error on cancelled context: %v", err)
	}
}

func TestRunContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Use a random available port
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := lis.Addr().String()
	lis.Close()

	// Run the server with timeout context
	err = run(ctx, addr, logger)
	if err != nil {
		t.Fatalf("run() should complete without error: %v", err)
	}
}

func TestRunMultiplePorts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Test with different ports to ensure robustness
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)

		lis, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("failed to find available port: %v", err)
		}
		addr := lis.Addr().String()
		lis.Close()

		err = run(ctx, addr, logger)
		if err != nil {
			t.Fatalf("run() iteration %d returned error: %v", i, err)
		}
		cancel()
	}
}

func BenchmarkRun(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use a random available port each iteration
		lis, err := net.Listen("tcp", ":0")
		if err != nil {
			b.Fatalf("failed to find available port: %v", err)
		}
		addr := lis.Addr().String()
		lis.Close()

		runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_ = run(runCtx, addr, logger)
		runCancel()
	}
}
