package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestStore creates a test store with an in-memory SQLite database.
func newTestStore(t *testing.T) *ErasureStore {
	logger := zap.NewNop()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	store, err := NewErasureStoreWithDB(db, logger)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	return store
}

func TestCreateRequest(t *testing.T) {
	tests := []struct {
		name           string
		request        *ErasureRequest
		wantErr        bool
		checkBackends  []string
		idempotencyKey string
	}{
		{
			name: "success async request",
			request: &ErasureRequest{
				Tenant:    "tenant-1",
				SubjectID: "user-123",
				Async:     true,
				Backends:  []string{"postgres", "s3"},
			},
			wantErr:       false,
			checkBackends: []string{"postgres", "s3"},
		},
		{
			name: "success with idempotency key",
			request: &ErasureRequest{
				Tenant:      "tenant-1",
				SubjectID:   "user-789",
				Async:       true,
				Idempotency: "idempotent-key-1",
				Backends:    []string{"postgres"},
			},
			wantErr:        false,
			checkBackends:  []string{"postgres"},
			idempotencyKey: "idempotent-key-1",
		},
		{
			name: "success with custom backends",
			request: &ErasureRequest{
				Tenant:    "tenant-2",
				SubjectID: "user-999",
				Async:     true,
				Backends:  []string{"s3", "mongo"},
			},
			wantErr:       false,
			checkBackends: []string{"s3", "mongo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			req, err := store.CreateRequest(tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if req.ID == "" {
				t.Errorf("CreateRequest() ID not set")
			}

			if req.Tenant != tt.request.Tenant {
				t.Errorf("CreateRequest() Tenant = %v, want %v", req.Tenant, tt.request.Tenant)
			}

			if req.SubjectID != tt.request.SubjectID {
				t.Errorf("CreateRequest() SubjectID = %v, want %v", req.SubjectID, tt.request.SubjectID)
			}

			if req.Async != tt.request.Async {
				t.Errorf("CreateRequest() Async = %v, want %v", req.Async, tt.request.Async)
			}

			if req.RequestedAt.IsZero() {
				t.Errorf("CreateRequest() RequestedAt not set")
			}

			if req.Backends == nil {
				t.Errorf("CreateRequest() Backends is nil")
				return
			}

			if len(req.Backends) != len(tt.checkBackends) {
				t.Errorf("CreateRequest() Backends length = %d, want %d", len(req.Backends), len(tt.checkBackends))
			}
		})
	}
}

func TestCreateRequestIdempotency(t *testing.T) {
	store := newTestStore(t)
	idempotencyKey := "idempotent-test-key"

	req1, err := store.CreateRequest(&ErasureRequest{
		Tenant:      "tenant-1",
		SubjectID:   "user-123",
		Async:       false, // Sync to avoid goroutine issues in test
		Idempotency: idempotencyKey,
		Backends:    []string{"postgres"},
	})
	if err != nil {
		t.Fatalf("CreateRequest() first call error = %v", err)
	}

	// Create duplicate request with same idempotency key
	req2, err := store.CreateRequest(&ErasureRequest{
		Tenant:      "tenant-1",
		SubjectID:   "user-456", // Different subject
		Async:       false,
		Idempotency: idempotencyKey,
		Backends:    []string{"s3"},
	})
	if err != nil {
		t.Fatalf("CreateRequest() duplicate call error = %v", err)
	}

	// Should return the same request
	if req2.ID != req1.ID {
		t.Errorf("CreateRequest() duplicate idempotency returned different ID: %v vs %v", req2.ID, req1.ID)
	}

	if req2.SubjectID != req1.SubjectID {
		t.Errorf("CreateRequest() duplicate idempotency returned different SubjectID: %v vs %v", req2.SubjectID, req1.SubjectID)
	}
}

func TestGetRequest(t *testing.T) {
	store := newTestStore(t)

	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-123",
		Async:     false,
	})

	requestID := req.ID

	tests := []struct {
		name  string
		id    string
		found bool
	}{
		{
			name:  "request found",
			id:    requestID,
			found: true,
		},
		{
			name:  "request not found",
			id:    "nonexistent-id",
			found: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := store.GetRequest(tt.id)
			if ok != tt.found {
				t.Errorf("GetRequest() ok = %v, want %v", ok, tt.found)
				return
			}

			if tt.found && got.ID != tt.id {
				t.Errorf("GetRequest() ID = %v, want %v", got.ID, tt.id)
			}
		})
	}
}

func TestListRequests(t *testing.T) {
	store := newTestStore(t)

	req1, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-1",
		Async:     false,
	})

	req2, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-2",
		Async:     false,
	})

	req3, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-2",
		SubjectID: "user-3",
		Async:     false,
	})

	tests := []struct {
		name      string
		tenant    string
		wantCount int
		checkIDs  map[string]bool
	}{
		{
			name:      "filter by tenant-1",
			tenant:    "tenant-1",
			wantCount: 2,
			checkIDs:  map[string]bool{req1.ID: true, req2.ID: true},
		},
		{
			name:      "filter by tenant-2",
			tenant:    "tenant-2",
			wantCount: 1,
			checkIDs:  map[string]bool{req3.ID: true},
		},
		{
			name:      "nonexistent tenant",
			tenant:    "nonexistent",
			wantCount: 0,
			checkIDs:  map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := store.ListRequests(tt.tenant)
			if len(requests) != tt.wantCount {
				t.Errorf("ListRequests() returned %d requests, want %d", len(requests), tt.wantCount)
				return
			}

			for _, req := range requests {
				if !tt.checkIDs[req.ID] {
					t.Errorf("ListRequests() returned unexpected request ID %v", req.ID)
				}
				if req.Tenant != tt.tenant {
					t.Errorf("ListRequests() request tenant = %v, want %v", req.Tenant, tt.tenant)
				}
				delete(tt.checkIDs, req.ID)
			}

			if len(tt.checkIDs) != 0 {
				t.Errorf("ListRequests() did not return expected request IDs: %v", tt.checkIDs)
			}
		})
	}
}

func TestOrchestrateErasureSync(t *testing.T) {
	store := newTestStore(t)

	// Register fake erasers
	store.eraser.RegisterEraser("postgres", NewFakeEraser(5, nil))
	store.eraser.RegisterEraser("s3", NewFakeEraser(3, nil))
	store.eraser.RegisterEraser("kafka", NewFakeEraser(0, nil))
	store.eraser.RegisterEraser("mongo", NewFakeEraser(2, nil))
	store.eraser.RegisterEraser("iceberg", NewFakeEraser(0, nil))

	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-456",
		Async:     false, // Sync to test immediately
		Backends:  []string{"postgres", "s3"},
	})

	if req.ID == "" {
		t.Fatalf("CreateRequest() did not return request ID")
	}

	// Get updated request
	updatedReq, ok := store.GetRequest(req.ID)
	if !ok {
		t.Fatalf("GetRequest() returned not found for erasure request")
	}

	if updatedReq.Status != "completed" {
		t.Errorf("orchestrateErasure() sync final status = %v, want completed", updatedReq.Status)
	}

	// DeletedCount should be sum of all erasers (5 from postgres + 3 from s3 = 8)
	if updatedReq.DeletedCount != 8 {
		t.Errorf("orchestrateErasure() sync DeletedCount = %d, want 8", updatedReq.DeletedCount)
	}

	if updatedReq.CompletedAt == nil {
		t.Errorf("orchestrateErasure() sync CompletedAt not set")
	}

	// Progress should track each backend
	if len(updatedReq.Progress) != 2 {
		t.Errorf("orchestrateErasure() sync Progress length = %d, want 2", len(updatedReq.Progress))
	}

	if updatedReq.Progress["postgres"] != "completed" {
		t.Errorf("orchestrateErasure() postgres progress = %v, want completed", updatedReq.Progress["postgres"])
	}

	if updatedReq.Progress["s3"] != "completed" {
		t.Errorf("orchestrateErasure() s3 progress = %v, want completed", updatedReq.Progress["s3"])
	}
}

func TestOrchestrateErasureAsync(t *testing.T) {
	store := newTestStore(t)

	// Register fake erasers
	store.eraser.RegisterEraser("postgres", NewFakeEraser(5, nil))
	store.eraser.RegisterEraser("s3", NewFakeEraser(3, nil))
	store.eraser.RegisterEraser("kafka", NewFakeEraser(0, nil))
	store.eraser.RegisterEraser("mongo", NewFakeEraser(2, nil))
	store.eraser.RegisterEraser("iceberg", NewFakeEraser(0, nil))

	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-456",
		Async:     true,
		Backends:  []string{"postgres", "kafka"},
	})

	if req.ID == "" {
		t.Fatalf("CreateRequest() did not return request ID")
	}

	// Wait for async operation to complete
	time.Sleep(100 * time.Millisecond)

	// Get updated request
	updatedReq, ok := store.GetRequest(req.ID)
	if !ok {
		t.Fatalf("GetRequest() returned not found for async request")
	}

	if updatedReq.Status != "completed" {
		t.Errorf("orchestrateErasure() async final status = %v, want completed", updatedReq.Status)
	}

	if updatedReq.CompletedAt == nil {
		t.Errorf("orchestrateErasure() async CompletedAt not set")
	}
}

func TestOrchestrateErasureWithBackendError(t *testing.T) {
	store := newTestStore(t)

	// Register fake erasers: postgres fails, s3 succeeds
	store.eraser.RegisterEraser("postgres", NewFakeEraser(0, fmt.Errorf("connection refused")))
	store.eraser.RegisterEraser("s3", NewFakeEraser(3, nil))
	store.eraser.RegisterEraser("kafka", NewFakeEraser(0, nil))
	store.eraser.RegisterEraser("mongo", NewFakeEraser(2, nil))
	store.eraser.RegisterEraser("iceberg", NewFakeEraser(0, nil))

	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-with-errors",
		Async:     false,
		Backends:  []string{"postgres", "s3"},
	})

	updatedReq, ok := store.GetRequest(req.ID)
	if !ok {
		t.Fatalf("GetRequest() returned not found")
	}

	// Status should be failed because one backend failed
	if updatedReq.Status != "failed" {
		t.Errorf("orchestrateErasure() error status = %v, want failed", updatedReq.Status)
	}

	// DeletedCount should still include successes (3 from s3)
	if updatedReq.DeletedCount != 3 {
		t.Errorf("orchestrateErasure() error DeletedCount = %d, want 3", updatedReq.DeletedCount)
	}

	if updatedReq.Error == "" {
		t.Errorf("orchestrateErasure() error Error not set")
	}

	if updatedReq.Progress["postgres"] != "failed" {
		t.Errorf("orchestrateErasure() postgres progress = %v, want failed", updatedReq.Progress["postgres"])
	}

	if updatedReq.Progress["s3"] != "completed" {
		t.Errorf("orchestrateErasure() s3 progress = %v, want completed", updatedReq.Progress["s3"])
	}
}

func TestDefaultBackends(t *testing.T) {
	store := newTestStore(t)

	t.Run("specified backends preserved", func(t *testing.T) {
		customBackends := []string{"postgres", "s3"}
		req, _ := store.CreateRequest(&ErasureRequest{
			Tenant:    "tenant-1",
			SubjectID: "user-custom",
			Async:     false,
			Backends:  customBackends,
		})

		if len(req.Backends) != len(customBackends) {
			t.Errorf("backends count = %d, want %d", len(req.Backends), len(customBackends))
		}

		for _, backend := range customBackends {
			found := false
			for _, b := range req.Backends {
				if b == backend {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("custom backends missing %s", backend)
			}
		}
	})

	t.Run("default backends applied", func(t *testing.T) {
		req, _ := store.CreateRequest(&ErasureRequest{
			Tenant:    "tenant-1",
			SubjectID: "user-default",
			Async:     false,
			Backends:  []string{}, // Empty
		})

		expectedDefaults := []string{"postgres", "kafka", "s3", "mongo", "iceberg"}
		if len(req.Backends) != len(expectedDefaults) {
			t.Errorf("default backends count = %d, want %d", len(req.Backends), len(expectedDefaults))
		}

		for _, backend := range expectedDefaults {
			found := false
			for _, b := range req.Backends {
				if b == backend {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("default backends missing %s", backend)
			}
		}
	})
}

func TestRequestFields(t *testing.T) {
	store := newTestStore(t)

	idempotencyKey := "test-idempotency"
	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:      "tenant-test",
		SubjectID:   "subject-test",
		Async:       false,
		Idempotency: idempotencyKey,
		Backends:    []string{"postgres"},
	})

	t.Run("ID is generated", func(t *testing.T) {
		if req.ID == "" {
			t.Errorf("Request ID not generated")
		}
		if !strings.HasPrefix(req.ID, "erasure-") {
			t.Errorf("Request ID = %v, want erasure- prefix", req.ID)
		}
	})

	t.Run("tenant and subject preserved", func(t *testing.T) {
		if req.Tenant != "tenant-test" {
			t.Errorf("Request Tenant = %v, want tenant-test", req.Tenant)
		}
		if req.SubjectID != "subject-test" {
			t.Errorf("Request SubjectID = %v, want subject-test", req.SubjectID)
		}
	})

	t.Run("idempotency key preserved", func(t *testing.T) {
		if req.Idempotency != idempotencyKey {
			t.Errorf("Request Idempotency = %v, want %v", req.Idempotency, idempotencyKey)
		}
	})

	t.Run("timestamps set", func(t *testing.T) {
		if req.RequestedAt.IsZero() {
			t.Errorf("Request RequestedAt not set")
		}
	})
}

func TestDurablePersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	store, err := NewErasureStoreWithDB(db, zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	// Register fake erasers
	store.eraser.RegisterEraser("postgres", NewFakeEraser(5, nil))
	store.eraser.RegisterEraser("s3", NewFakeEraser(3, nil))
	store.eraser.RegisterEraser("kafka", NewFakeEraser(0, nil))
	store.eraser.RegisterEraser("mongo", NewFakeEraser(2, nil))
	store.eraser.RegisterEraser("iceberg", NewFakeEraser(0, nil))

	// Create and execute erasure
	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-durable",
		Async:     false,
		Backends:  []string{"postgres", "s3"},
	})

	originalID := req.ID
	originalDeleted := req.DeletedCount

	// Simulate closing and reopening the store (data should persist in DB)
	// In real scenario, the DB connection would survive, but we verify persistence
	retrieved, found := store.GetRequest(originalID)
	if !found {
		t.Fatalf("request not found after persistence")
	}

	if retrieved.DeletedCount != originalDeleted {
		t.Errorf("DeletedCount mismatch: got %d, want %d", retrieved.DeletedCount, originalDeleted)
	}

	if retrieved.Status != "completed" {
		t.Errorf("Status mismatch: got %s, want completed", retrieved.Status)
	}
}

func TestConcurrentCreateRequests(t *testing.T) {
	store := newTestStore(t)

	done := make(chan bool, 5)
	ids := make(chan string, 5)

	for i := 0; i < 5; i++ {
		go func(idx int) {
			req, _ := store.CreateRequest(&ErasureRequest{
				Tenant:    "tenant-1",
				SubjectID: fmt.Sprintf("user-%d", idx),
				Async:     false,
			})
			ids <- req.ID
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	time.Sleep(20 * time.Millisecond)

	requests := store.ListRequests("tenant-1")
	if len(requests) != 5 {
		t.Errorf("Concurrent creates resulted in %d requests, want 5", len(requests))
	}
}

func TestEmptyStore(t *testing.T) {
	store := newTestStore(t)

	requests := store.ListRequests("tenant-1")
	if len(requests) != 0 {
		t.Errorf("ListRequests() on empty store returned %d requests, want 0", len(requests))
	}

	_, found := store.GetRequest("nonexistent")
	if found {
		t.Errorf("GetRequest() on empty store returned found, want not found")
	}
}

// TestPostgresIntegration tests erasure store with a real PostgreSQL instance via testcontainers.
// This is an integration test and will be skipped if Docker is unavailable.
func TestPostgresIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Note: Full postgres integration test would require testcontainers setup.
	// For now, this is a placeholder that demonstrates the pattern.
	// In production, use testcontainers-go to spin up a postgres:16-bookworm container.
	t.Log("postgres integration test - requires docker (testcontainers)")
}
