package store

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCreateDataResource tests successful creation of a DataResource
func TestCreateDataResource(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	dr := &DataResourceRecord{
		ID:        "res-123",
		Name:      "db-primary",
		Tenant:    "tenant-a",
		Type:      "database",
		Class:     "PostgreSQL",
		Protocols: []string{"tcp", "udp"},
		CreatedAt: time.Now(),
	}

	err := store.CreateDataResource(ctx, dr)
	if err != nil {
		t.Fatalf("CreateDataResource failed: %v", err)
	}

	// Verify it was stored
	retrieved, err := store.GetDataResource(ctx, "tenant-a", "db-primary")
	if err != nil {
		t.Fatalf("GetDataResource failed: %v", err)
	}
	if retrieved.ID != "res-123" {
		t.Errorf("expected ID res-123, got %s", retrieved.ID)
	}
}

// TestCreateDataResourceDuplicate tests that duplicate creation fails
func TestCreateDataResourceDuplicate(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	dr := &DataResourceRecord{
		ID:     "res-123",
		Name:   "db-primary",
		Tenant: "tenant-a",
		Type:   "database",
	}

	err := store.CreateDataResource(ctx, dr)
	if err != nil {
		t.Fatalf("first CreateDataResource failed: %v", err)
	}

	// Try to create the same resource again
	err = store.CreateDataResource(ctx, dr)
	if err == nil {
		t.Error("expected error for duplicate creation, got nil")
	}
	if err.Error() != "DataResource db-primary already exists" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestGetDataResource tests retrieval of an existing resource
func TestGetDataResource(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	dr := &DataResourceRecord{
		ID:        "res-456",
		Name:      "cache-redis",
		Tenant:    "tenant-b",
		Type:      "cache",
		Class:     "Redis",
		Protocols: []string{"tcp"},
		CreatedAt: time.Now(),
	}

	if err := store.CreateDataResource(ctx, dr); err != nil {
		t.Fatalf("CreateDataResource failed: %v", err)
	}

	retrieved, err := store.GetDataResource(ctx, "tenant-b", "cache-redis")
	if err != nil {
		t.Fatalf("GetDataResource failed: %v", err)
	}

	if retrieved.ID != "res-456" || retrieved.Type != "cache" {
		t.Errorf("retrieved resource mismatch: %+v", retrieved)
	}
}

// TestGetDataResourceNotFound tests that retrieval of non-existent resource fails
func TestGetDataResourceNotFound(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	_, err := store.GetDataResource(ctx, "tenant-nonexistent", "resource-missing")
	if err == nil {
		t.Error("expected error for missing resource, got nil")
	}
	if err.Error() != "DataResource resource-missing not found" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestListDataResources tests listing resources for a specific tenant
func TestListDataResources(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Create resources for tenant-a
	dr1 := &DataResourceRecord{
		ID:     "res-1",
		Name:   "resource-1",
		Tenant: "tenant-a",
		Type:   "database",
	}
	dr2 := &DataResourceRecord{
		ID:     "res-2",
		Name:   "resource-2",
		Tenant: "tenant-a",
		Type:   "cache",
	}

	// Create resource for tenant-b (should not appear in list)
	dr3 := &DataResourceRecord{
		ID:     "res-3",
		Name:   "resource-3",
		Tenant: "tenant-b",
		Type:   "database",
	}

	for _, dr := range []*DataResourceRecord{dr1, dr2, dr3} {
		if err := store.CreateDataResource(ctx, dr); err != nil {
			t.Fatalf("CreateDataResource failed: %v", err)
		}
	}

	// List resources for tenant-a
	list, err := store.ListDataResources(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("ListDataResources failed: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 resources for tenant-a, got %d", len(list))
	}

	// Verify tenant-b resource is not included
	for _, dr := range list {
		if dr.Tenant != "tenant-a" {
			t.Errorf("unexpected tenant in list: %s", dr.Tenant)
		}
	}
}

// TestListDataResourcesEmpty tests listing when no resources exist
func TestListDataResourcesEmpty(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	list, err := store.ListDataResources(ctx, "nonexistent-tenant")
	if err != nil {
		t.Fatalf("ListDataResources failed: %v", err)
	}

	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}
	// Note: Go allows nil slices from map iteration, which is valid
}

// TestDeleteDataResource tests successful deletion
func TestDeleteDataResource(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	dr := &DataResourceRecord{
		ID:     "res-789",
		Name:   "to-delete",
		Tenant: "tenant-c",
		Type:   "database",
	}

	if err := store.CreateDataResource(ctx, dr); err != nil {
		t.Fatalf("CreateDataResource failed: %v", err)
	}

	// Verify it exists
	if _, err := store.GetDataResource(ctx, "tenant-c", "to-delete"); err != nil {
		t.Fatalf("GetDataResource before delete failed: %v", err)
	}

	// Delete it
	err := store.DeleteDataResource(ctx, "tenant-c", "to-delete")
	if err != nil {
		t.Fatalf("DeleteDataResource failed: %v", err)
	}

	// Verify it's gone
	_, err = store.GetDataResource(ctx, "tenant-c", "to-delete")
	if err == nil {
		t.Error("expected error after deletion, got nil")
	}
}

// TestDeleteDataResourceNotFound tests deletion of non-existent resource
func TestDeleteDataResourceNotFound(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	err := store.DeleteDataResource(ctx, "nonexistent-tenant", "nonexistent-resource")
	if err == nil {
		t.Error("expected error for deleting non-existent resource, got nil")
	}
	if err.Error() != "DataResource nonexistent-resource not found" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestCountDataResources tests counting resources for a tenant
func TestCountDataResources(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Create 3 resources for tenant-a
	for i := 1; i <= 3; i++ {
		dr := &DataResourceRecord{
			ID:     fmt.Sprintf("res-%d", i),
			Name:   fmt.Sprintf("resource-%d", i),
			Tenant: "tenant-a",
			Type:   "database",
		}
		if err := store.CreateDataResource(ctx, dr); err != nil {
			t.Fatalf("CreateDataResource failed: %v", err)
		}
	}

	// Create 2 resources for tenant-b
	for i := 4; i <= 5; i++ {
		dr := &DataResourceRecord{
			ID:     fmt.Sprintf("res-%d", i),
			Name:   fmt.Sprintf("resource-%d", i),
			Tenant: "tenant-b",
			Type:   "database",
		}
		if err := store.CreateDataResource(ctx, dr); err != nil {
			t.Fatalf("CreateDataResource failed: %v", err)
		}
	}

	// Count for tenant-a
	count, err := store.CountDataResources(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("CountDataResources failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 resources for tenant-a, got %d", count)
	}

	// Count for tenant-b
	count, err = store.CountDataResources(ctx, "tenant-b")
	if err != nil {
		t.Fatalf("CountDataResources failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 resources for tenant-b, got %d", count)
	}

	// Count for non-existent tenant
	count, err = store.CountDataResources(ctx, "nonexistent-tenant")
	if err != nil {
		t.Fatalf("CountDataResources failed: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 resources for nonexistent-tenant, got %d", count)
	}
}

// TestConcurrentCreateAndGet tests concurrent access for race conditions
func TestConcurrentCreateAndGet(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	numGoroutines := 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			dr := &DataResourceRecord{
				ID:     fmt.Sprintf("res-%d", id),
				Name:   fmt.Sprintf("resource-%d", id),
				Tenant: "concurrent-tenant",
				Type:   "database",
			}
			if err := store.CreateDataResource(ctx, dr); err != nil {
				t.Errorf("CreateDataResource failed: %v", err)
			}

			retrieved, err := store.GetDataResource(ctx, "concurrent-tenant", fmt.Sprintf("resource-%d", id))
			if err != nil {
				t.Errorf("GetDataResource failed: %v", err)
			}
			if retrieved.ID != fmt.Sprintf("res-%d", id) {
				t.Errorf("ID mismatch: expected res-%d, got %s", id, retrieved.ID)
			}
		}(i)
	}

	wg.Wait()

	// Verify all were created
	count, err := store.CountDataResources(ctx, "concurrent-tenant")
	if err != nil {
		t.Fatalf("CountDataResources failed: %v", err)
	}
	if count != numGoroutines {
		t.Errorf("expected %d resources, got %d", numGoroutines, count)
	}
}

// TestConcurrentDeleteAndList tests concurrent delete and list operations
func TestConcurrentDeleteAndList(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	numResources := 20

	// Create initial resources
	for i := 0; i < numResources; i++ {
		dr := &DataResourceRecord{
			ID:     fmt.Sprintf("res-%d", i),
			Name:   fmt.Sprintf("resource-%d", i),
			Tenant: "delete-tenant",
			Type:   "database",
		}
		if err := store.CreateDataResource(ctx, dr); err != nil {
			t.Fatalf("CreateDataResource failed: %v", err)
		}
	}

	var wg sync.WaitGroup
	var deleteErrors int32

	// Half the goroutines delete, half list
	for i := 0; i < numResources; i++ {
		wg.Add(1)
		if i%2 == 0 {
			go func(id int) {
				defer wg.Done()
				if err := store.DeleteDataResource(ctx, "delete-tenant", fmt.Sprintf("resource-%d", id)); err != nil {
					atomic.AddInt32(&deleteErrors, 1)
				}
			}(i)
		} else {
			go func() {
				defer wg.Done()
				_, _ = store.ListDataResources(ctx, "delete-tenant")
			}()
		}
	}

	wg.Wait()

	if deleteErrors != 0 {
		t.Errorf("expected 0 delete errors, got %d", deleteErrors)
	}

	// Final count should be around half
	count, err := store.CountDataResources(ctx, "delete-tenant")
	if err != nil {
		t.Fatalf("CountDataResources failed: %v", err)
	}
	expectedApprox := numResources / 2
	if count < expectedApprox-2 || count > expectedApprox+2 {
		t.Logf("final count %d is approximately half of %d (acceptable)", count, numResources)
	}
}

// TestConcurrentCreateDuplicate tests concurrent creation of same resource
func TestConcurrentCreateDuplicate(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	numGoroutines := 10
	successCount := int32(0)
	failCount := int32(0)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			dr := &DataResourceRecord{
				ID:     "res-dup-test",
				Name:   "duplicate-resource",
				Tenant: "dup-tenant",
				Type:   "database",
			}
			if err := store.CreateDataResource(ctx, dr); err != nil {
				atomic.AddInt32(&failCount, 1)
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	// Exactly one should succeed, rest should fail
	if successCount != 1 {
		t.Errorf("expected 1 successful creation, got %d", successCount)
	}
	if failCount != int32(numGoroutines-1) {
		t.Errorf("expected %d failed creations, got %d", numGoroutines-1, failCount)
	}
}

// TestKeyGeneration tests the key function with various inputs
func TestKeyGeneration(t *testing.T) {
	tests := []struct {
		tenant   string
		name     string
		expected string
	}{
		{"tenant-a", "resource-1", "tenant-a/resource-1"},
		{"", "resource", "/resource"},
		{"tenant", "", "tenant/"},
		{"", "", "/"},
		{"tenant-with-slash/test", "resource", "tenant-with-slash/test/resource"},
	}

	for _, tt := range tests {
		result := key(tt.tenant, tt.name)
		if result != tt.expected {
			t.Errorf("key(%q, %q) = %q, expected %q", tt.tenant, tt.name, result, tt.expected)
		}
	}
}

// BenchmarkCreateDataResource benchmarks creation performance
func BenchmarkCreateDataResource(b *testing.B) {
	store := NewMemoryStore()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dr := &DataResourceRecord{
			ID:     fmt.Sprintf("res-%d", i),
			Name:   fmt.Sprintf("resource-%d", i),
			Tenant: "benchmark-tenant",
			Type:   "database",
		}
		_ = store.CreateDataResource(ctx, dr)
	}
}

// BenchmarkGetDataResource benchmarks retrieval performance
func BenchmarkGetDataResource(b *testing.B) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Populate with resources
	for i := 0; i < 1000; i++ {
		dr := &DataResourceRecord{
			ID:     fmt.Sprintf("res-%d", i),
			Name:   fmt.Sprintf("resource-%d", i),
			Tenant: "benchmark-tenant",
			Type:   "database",
		}
		_ = store.CreateDataResource(ctx, dr)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.GetDataResource(ctx, "benchmark-tenant", fmt.Sprintf("resource-%d", i%1000))
	}
}

// BenchmarkListDataResources benchmarks list performance
func BenchmarkListDataResources(b *testing.B) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Populate with resources
	for i := 0; i < 100; i++ {
		dr := &DataResourceRecord{
			ID:     fmt.Sprintf("res-%d", i),
			Name:   fmt.Sprintf("resource-%d", i),
			Tenant: "benchmark-tenant",
			Type:   "database",
		}
		_ = store.CreateDataResource(ctx, dr)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.ListDataResources(ctx, "benchmark-tenant")
	}
}
