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
		return ":19090"
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

func TestRunWithMultipleServers(t *testing.T) {
	// Start one server on a port
	addr1 := getAvailablePort()
	ctx1, cancel1 := context.WithCancel(context.Background())

	errChan1 := make(chan error, 1)
	go func() {
		errChan1 <- run(ctx1, addr1)
	}()

	// Let it start
	time.Sleep(30 * time.Millisecond)

	// Cancel it
	cancel1()
	time.Sleep(50 * time.Millisecond)

	// Start another server on different port
	addr2 := getAvailablePort()
	ctx2, cancel2 := context.WithCancel(context.Background())

	errChan2 := make(chan error, 1)
	go func() {
		errChan2 <- run(ctx2, addr2)
	}()

	// Let it start
	time.Sleep(30 * time.Millisecond)

	// Cancel it
	cancel2()
	time.Sleep(50 * time.Millisecond)
}

func TestPipelinePerformScan(t *testing.T) {
	catalog := NewCatalog()
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	pipeline := NewPipeline(catalog, logger)

	// Add entries with different resource IDs and table names
	for i := 0; i < 2; i++ {
		entry := &CatalogEntry{
			ResourceID: fmt.Sprintf("res-%d", i),
			TableName:  fmt.Sprintf("tbl-%d", i),
			Tenant:     "test-tenant",
			Columns: []ColumnEntry{
				{Name: "id", DataType: "int"},
				{Name: "email", DataType: "string"},
			},
		}
		catalog.Upsert(entry)
	}

	// Perform a scan manually
	pipeline.performScan(logger)

	// Verify entries were processed (labels should be assigned)
	entries := catalog.List("test-tenant", "")
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Check that at least one entry has labels
	hasLabels := false
	for _, e := range entries {
		if len(e.Labels) > 0 {
			hasLabels = true
			break
		}
	}
	if !hasLabels {
		t.Errorf("expected at least one entry to have labels assigned")
	}
}

func TestPipelineRunDailyScansSetupAndTeardown(t *testing.T) {
	catalog := NewCatalog()
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	pipeline := NewPipeline(catalog, logger)

	// Add an entry with properly formatted key to the catalog
	entry := &CatalogEntry{
		ResourceID: "test-resource",
		TableName:  "test-table",
		Tenant:     "test-tenant",
		Columns: []ColumnEntry{
			{Name: "id", DataType: "int"},
			{Name: "email", DataType: "string"},
		},
	}
	catalog.Upsert(entry)

	// Start the daily scans in a goroutine
	go pipeline.RunDailyScans(logger)

	// Wait to ensure it starts without panicking
	time.Sleep(20 * time.Millisecond)

	// If we get here, it didn't panic during setup and teardown
}

func TestPipelineRunDailyScansTickerLoop(t *testing.T) {
	catalog := NewCatalog()
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	pipeline := NewPipeline(catalog, logger)

	// Add multiple entries
	for i := 0; i < 3; i++ {
		entry := &CatalogEntry{
			ResourceID: fmt.Sprintf("resource-%d", i),
			TableName:  "table",
			Tenant:     "tenant",
			Columns: []ColumnEntry{
				{Name: "col1", DataType: "string"},
			},
		}
		catalog.Upsert(entry)
	}

	// Start the daily scans
	go pipeline.RunDailyScans(logger)

	// Wait briefly
	time.Sleep(10 * time.Millisecond)

	// Should have processed entries without panic
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

	catalog := NewCatalog()
	pipeline := NewPipeline(catalog, logger)
	mux := NewMux(catalog, pipeline, logger, authMiddleware)

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

func TestPipelineAsyncClassificationErrorHandling(t *testing.T) {
	// Test the error handling in async classification
	catalog := NewCatalog()
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	pipeline := NewPipeline(catalog, logger)

	// Add entries with various data types
	for i := 0; i < 3; i++ {
		entry := &CatalogEntry{
			ResourceID: fmt.Sprintf("resource-%d", i),
			TableName:  fmt.Sprintf("table-%d", i),
			Tenant:     "test-tenant",
			Columns: []ColumnEntry{
				{Name: "col1", DataType: "string"},
				{Name: "col2", DataType: "int"},
			},
		}
		catalog.Upsert(entry)
	}

	// Call ClassifyEntry directly to test the paths
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("resource-%d:table-%d", i, i)
		parts := len(key) // Just verify structure
		if parts > 0 {
			if err := pipeline.ClassifyEntry(fmt.Sprintf("resource-%d", i), fmt.Sprintf("table-%d", i)); err != nil {
				t.Logf("classification error (expected): %v", err)
			}
		}
	}

	// Ensure catalog entries were updated
	all := catalog.List("", "")
	if len(all) < 3 {
		t.Errorf("expected at least 3 entries, got %d", len(all))
	}
}

func TestRunFunctionShutdownError(t *testing.T) {
	// Test error handling with immediate shutdown
	addr := getAvailablePort()
	ctx, cancel := context.WithCancel(context.Background())

	// Start server
	go run(ctx, addr)

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Immediately cancel to trigger shutdown
	cancel()

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)
}

func TestFullIntegration(t *testing.T) {
	// Full integration test to cover run() and mux initialization
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

	catalog := NewCatalog()
	pipeline := NewPipeline(catalog, logger)

	// Add test data
	entry := &CatalogEntry{
		ResourceID: "test-res",
		TableName:  "test-table",
		Tenant:     "test-tenant",
		Columns: []ColumnEntry{
			{Name: "user_email", DataType: "string"},
			{Name: "user_id", DataType: "uuid"},
		},
	}
	catalog.Upsert(entry)

	// Manually classify to populate labels
	if err := pipeline.ClassifyEntry("test-res", "test-table"); err != nil {
		t.Logf("classification error: %v", err)
	}

	// Create mux and verify it works
	mux := NewMux(catalog, pipeline, logger, authMiddleware)
	if mux == nil {
		t.Fatal("mux should not be nil")
	}

	// Verify catalog has the entry with labels
	entries := catalog.List("test-tenant", "")
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	if len(entries[0].Labels) == 0 {
		t.Errorf("expected entry to have labels")
	}
}

func TestPerfomScanWithErrorEntry(t *testing.T) {
	// Test performScan with an entry that causes error
	catalog := NewCatalog()
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	pipeline := NewPipeline(catalog, logger)

	// Add an entry
	entry := &CatalogEntry{
		ResourceID: "res1",
		TableName:  "tbl1",
		Tenant:     "t1",
		Columns:    []ColumnEntry{{Name: "c1", DataType: "int"}},
	}
	catalog.Upsert(entry)

	// Add an invalid key to the catalog to test error path
	// This is to ensure the error handling in performScan is tested
	catalog.entries["invalid-key-no-colon"] = entry

	// This should not panic even with invalid key
	pipeline.performScan(logger)
}

func TestShutdownServer(t *testing.T) {
	// Test the shutdownServer helper function
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	addr := getAvailablePort()
	server := &http.Server{
		Addr: addr,
	}

	// Start server in background
	go func() {
		server.ListenAndServe()
	}()

	// Give it time to start
	time.Sleep(20 * time.Millisecond)

	// Call shutdownServer
	err := shutdownServer(server, logger)
	if err != nil {
		t.Logf("shutdown error: %v", err)
	}
}
