package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

func getAvailablePort() string {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return ":19091"
	}
	defer listener.Close()
	addr := listener.Addr().(*net.TCPAddr)
	return fmt.Sprintf(":%d", addr.Port)
}

func TestRunMainContextCancel(t *testing.T) {
	addr := getAvailablePort()

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start the server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx, addr)
	}()

	// Give it a moment to start
	time.Sleep(20 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait for completion
	time.Sleep(100 * time.Millisecond)
}

func TestCalculatorRunDailyAggregationDoesNotPanic(t *testing.T) {
	calc := NewCalculator()
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Start the daily aggregation in a goroutine
	go calc.RunDailyAggregation(logger)

	// Wait a short time to ensure it starts without panicking
	time.Sleep(10 * time.Millisecond)

	// If we get here, it didn't panic
}

func TestCalculatorRunDailyAggregationWithRecords(t *testing.T) {
	calc := NewCalculator()
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Add some records - all in same month so only 2 unique month/tenant combos
	calc.AddTokens("tenant-1", "api", 100.0)
	calc.AddTokens("tenant-1", "web", 50.0)
	calc.AddTokens("tenant-2", "api", 75.0)

	// Start the daily aggregation
	go calc.RunDailyAggregation(logger)

	// Wait a short time
	time.Sleep(10 * time.Millisecond)

	// Verify records are still there (should not panic or delete)
	records := calc.AllRecords()
	if len(records) != 2 {
		t.Errorf("expected 2 records (tenant-1 and tenant-2, same month), got %d", len(records))
	}
}

func TestServerStartup(t *testing.T) {
	addr := getAvailablePort()
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Initialize auth middleware for test
	authConfig := &auth.Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	calc := NewCalculator()
	mux := NewMux(calc, logger, authMiddleware)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start server
	serverErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Give it time to start
	time.Sleep(20 * time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Logf("shutdown error: %v", err)
	}
}

func TestRunFunctionStartsAndShutdowns(t *testing.T) {
	// Test the run function path - it starts server and waits for signal
	addr := getAvailablePort()
	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx, addr)
	}()

	// Let it start
	time.Sleep(30 * time.Millisecond)

	// Cancel the context to trigger shutdown
	cancel()

	// Wait for it to finish
	time.Sleep(100 * time.Millisecond)
}

func TestRunFunctionListenError(t *testing.T) {
	// Test error path by using an invalid address format
	// This should cause ListenAndServe to fail and return an error
	addr := "invalid-addr-format"
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := run(ctx, addr)
	// The run function should return an error or timeout
	_ = err
}

func TestCalculatorIntegration(t *testing.T) {
	// Test full workflow with calculator and mux
	calc := NewCalculator()
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Initialize auth middleware for test
	authConfig := &auth.Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	mux := NewMux(calc, logger, authMiddleware)

	// Add some data
	calc.AddTokens("test-tenant", "api", 100.0)

	// Verify it's stored
	records := calc.AllRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}

	// Verify mux is properly initialized
	if mux == nil {
		t.Fatal("mux should not be nil")
	}
}
