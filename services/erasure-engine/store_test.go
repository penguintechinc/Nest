package main

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCreateRequest(t *testing.T) {
	logger := zap.NewNop()
	store := NewErasureStore(logger)

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
				Tenant:       "tenant-1",
				SubjectID:    "user-789",
				Async:        true,
				Idempotency:  "idempotent-key-1",
				Backends:     []string{"postgres"},
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

			// Check backends match
			if len(req.Backends) != len(tt.checkBackends) {
				t.Errorf("CreateRequest() Backends length = %d, want %d", len(req.Backends), len(tt.checkBackends))
			}
		})
	}
}

func TestCreateRequestIdempotency(t *testing.T) {
	logger := zap.NewNop()
	store := NewErasureStore(logger)

	idempotencyKey := "idempotent-test-key"

	// Create first request with async mode to avoid deadlock
	req1, err := store.CreateRequest(&ErasureRequest{
		Tenant:       "tenant-1",
		SubjectID:    "user-123",
		Async:        true,
		Idempotency:  idempotencyKey,
		Backends:     []string{"postgres"},
	})
	if err != nil {
		t.Fatalf("CreateRequest() first call error = %v", err)
	}

	// Wait a bit for async to potentially start
	time.Sleep(20 * time.Millisecond)

	// Create duplicate request with same idempotency key
	req2, err := store.CreateRequest(&ErasureRequest{
		Tenant:       "tenant-1",
		SubjectID:    "user-456", // Different subject
		Async:        true,
		Idempotency:  idempotencyKey,
		Backends:     []string{"s3"},
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
	logger := zap.NewNop()
	store := NewErasureStore(logger)

	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-123",
		Async:     true,
	})

	// Wait for async to potentially progress
	time.Sleep(20 * time.Millisecond)

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
	logger := zap.NewNop()
	store := NewErasureStore(logger)

	// Create requests for different tenants
	req1, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-1",
		Async:     true,
	})

	time.Sleep(20 * time.Millisecond)

	req2, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-2",
		Async:     true,
	})

	time.Sleep(20 * time.Millisecond)

	req3, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-2",
		SubjectID: "user-3",
		Async:     true,
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

func TestSimulateErasureAsync(t *testing.T) {
	logger := zap.NewNop()
	store := NewErasureStore(logger)

	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-456",
		Async:     true,
		Backends:  []string{"postgres", "kafka"},
	})

	// Initial request just after creation might be in pending or scanning state
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
		t.Errorf("simulateErasure() async final status = %v, want completed", updatedReq.Status)
	}

	if updatedReq.CompletedAt == nil {
		t.Errorf("simulateErasure() async CompletedAt not set")
	}

	if updatedReq.DeletedCount == 0 {
		t.Errorf("simulateErasure() async DeletedCount = 0, want > 0")
	}

	// Verify progress through states
	if len(updatedReq.Progress) != 2 {
		t.Errorf("simulateErasure() async Progress length = %d, want 2", len(updatedReq.Progress))
	}
}

func TestDefaultBackends(t *testing.T) {
	logger := zap.NewNop()
	store := NewErasureStore(logger)

	t.Run("specified backends preserved", func(t *testing.T) {
		customBackends := []string{"postgres", "s3"}
		req, _ := store.CreateRequest(&ErasureRequest{
			Tenant:    "tenant-1",
			SubjectID: "user-custom",
			Async:     true,
			Backends:  customBackends,
		})

		if len(req.Backends) != len(customBackends) {
			t.Errorf("simulateErasure() custom backends count = %d, want %d", len(req.Backends), len(customBackends))
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
				t.Errorf("simulateErasure() custom backends missing %s", backend)
			}
		}
	})
}

func TestRequestStatusProgression(t *testing.T) {
	logger := zap.NewNop()
	store := NewErasureStore(logger)

	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-progress",
		Async:     true,
		Backends:  []string{"postgres"},
	})

	// Wait for async completion
	time.Sleep(100 * time.Millisecond)

	// Verify final status
	finalReq, _ := store.GetRequest(req.ID)

	if finalReq.Status != "completed" {
		t.Errorf("Request final status = %v, want completed", finalReq.Status)
	}

	// Verify CompletedAt is set
	if finalReq.CompletedAt == nil {
		t.Errorf("Request CompletedAt is nil")
	}

	// Verify DeletedCount is set
	if finalReq.DeletedCount == 0 {
		t.Errorf("Request DeletedCount = 0, want > 0")
	}
}

func TestProgressTracking(t *testing.T) {
	logger := zap.NewNop()
	store := NewErasureStore(logger)

	backends := []string{"postgres", "kafka", "s3"}
	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:    "tenant-1",
		SubjectID: "user-progress",
		Async:     true,
		Backends:  backends,
	})

	// Wait for async completion
	time.Sleep(100 * time.Millisecond)

	finalReq, _ := store.GetRequest(req.ID)

	// Verify progress map has all backends
	if len(finalReq.Progress) != len(backends) {
		t.Errorf("Progress map length = %d, want %d", len(finalReq.Progress), len(backends))
	}

	// Verify all backends progressed to "erased"
	for _, backend := range backends {
		status, exists := finalReq.Progress[backend]
		if !exists {
			t.Errorf("Progress missing entry for backend %s", backend)
		}
		if status != "erased" {
			t.Errorf("Progress[%s] = %s, want erased", backend, status)
		}
	}
}

func TestConcurrentCreateRequests(t *testing.T) {
	logger := zap.NewNop()
	store := NewErasureStore(logger)

	done := make(chan bool, 5)
	ids := make(chan string, 5)

	for i := 0; i < 5; i++ {
		go func(idx int) {
			req, _ := store.CreateRequest(&ErasureRequest{
				Tenant:    "tenant-1",
				SubjectID: "user-" + string(rune('0'+idx)),
				Async:     true,
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
	logger := zap.NewNop()
	store := NewErasureStore(logger)

	requests := store.ListRequests("tenant-1")
	if len(requests) != 0 {
		t.Errorf("ListRequests() on empty store returned %d requests, want 0", len(requests))
	}

	_, found := store.GetRequest("nonexistent")
	if found {
		t.Errorf("GetRequest() on empty store returned found, want not found")
	}
}

func TestRequestFields(t *testing.T) {
	logger := zap.NewNop()
	store := NewErasureStore(logger)

	idempotencyKey := "test-idempotency"
	req, _ := store.CreateRequest(&ErasureRequest{
		Tenant:       "tenant-test",
		SubjectID:    "subject-test",
		Async:        true,
		Idempotency:  idempotencyKey,
		Backends:     []string{"postgres"},
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
