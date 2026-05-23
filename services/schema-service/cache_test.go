package main

import (
	"testing"
	"time"
)

func TestNewSchemaCache(t *testing.T) {
	ttl := 5 * time.Minute
	cache := NewSchemaCache(ttl)

	if cache == nil {
		t.Fatal("NewSchemaCache returned nil")
	}

	if cache.entries == nil {
		t.Fatal("entries map is nil")
	}

	if cache.ttl != ttl {
		t.Errorf("expected ttl %v, got %v", ttl, cache.ttl)
	}

	if len(cache.entries) != 0 {
		t.Errorf("expected empty cache, got %d entries", len(cache.entries))
	}
}

func TestSet_NewEntry(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
		DiscoveredAt: time.Now(),
	}

	cache.Set("resource-1", schema)

	if len(cache.entries) != 1 {
		t.Errorf("expected 1 entry in cache, got %d", len(cache.entries))
	}
}

func TestGet_Found(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
		DiscoveredAt: time.Now(),
	}

	cache.Set("resource-1", schema)

	retrieved, found := cache.Get("resource-1")
	if !found {
		t.Fatal("expected to find schema in cache")
	}

	if retrieved == nil {
		t.Fatal("retrieved schema is nil")
	}

	if retrieved.ResourceID != "resource-1" {
		t.Errorf("expected resourceID 'resource-1', got '%s'", retrieved.ResourceID)
	}

	if retrieved.ResourceType != "postgres" {
		t.Errorf("expected resourceType 'postgres', got '%s'", retrieved.ResourceType)
	}
}

func TestGet_NotFound(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	retrieved, found := cache.Get("nonexistent")
	if found {
		t.Fatal("expected not to find schema in cache")
	}

	if retrieved != nil {
		t.Fatal("expected nil schema")
	}
}

func TestGet_ExpiredEntry(t *testing.T) {
	// Create cache with very short TTL
	cache := NewSchemaCache(1 * time.Millisecond)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
		DiscoveredAt: time.Now(),
	}

	cache.Set("resource-1", schema)

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	retrieved, found := cache.Get("resource-1")
	if found {
		t.Fatal("expected expired entry to not be found")
	}

	if retrieved != nil {
		t.Fatal("expected nil schema for expired entry")
	}
}

func TestSet_Overwrite(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	schema1 := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
		TableCount:   10,
	}

	schema2 := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "mysql",
		TableCount:   20,
	}

	cache.Set("resource-1", schema1)
	cache.Set("resource-1", schema2)

	if len(cache.entries) != 1 {
		t.Errorf("expected 1 entry in cache, got %d", len(cache.entries))
	}

	retrieved, _ := cache.Get("resource-1")
	if retrieved.ResourceType != "mysql" {
		t.Errorf("expected overwritten resourceType 'mysql', got '%s'", retrieved.ResourceType)
	}

	if retrieved.TableCount != 20 {
		t.Errorf("expected overwritten tableCount 20, got %d", retrieved.TableCount)
	}
}

func TestInvalidate_Removes(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
	}

	cache.Set("resource-1", schema)

	cache.Invalidate("resource-1")

	retrieved, found := cache.Get("resource-1")
	if found {
		t.Fatal("expected invalidated entry to not be found")
	}

	if retrieved != nil {
		t.Fatal("expected nil schema for invalidated entry")
	}
}

func TestInvalidate_NonExistent(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	// Should not panic
	cache.Invalidate("nonexistent")

	if len(cache.entries) != 0 {
		t.Errorf("expected empty cache, got %d entries", len(cache.entries))
	}
}

func TestMultipleEntries(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	schema1 := &Schema{ResourceID: "resource-1", ResourceType: "postgres"}
	schema2 := &Schema{ResourceID: "resource-2", ResourceType: "mysql"}
	schema3 := &Schema{ResourceID: "resource-3", ResourceType: "clickhouse"}

	cache.Set("resource-1", schema1)
	cache.Set("resource-2", schema2)
	cache.Set("resource-3", schema3)

	if len(cache.entries) != 3 {
		t.Errorf("expected 3 entries in cache, got %d", len(cache.entries))
	}

	// Verify all can be retrieved
	r1, found1 := cache.Get("resource-1")
	r2, found2 := cache.Get("resource-2")
	r3, found3 := cache.Get("resource-3")

	if !found1 || !found2 || !found3 {
		t.Fatal("expected to find all entries")
	}

	if r1.ResourceType != "postgres" || r2.ResourceType != "mysql" || r3.ResourceType != "clickhouse" {
		t.Fatal("retrieved wrong schemas")
	}
}

func TestTTLBoundary(t *testing.T) {
	// Test at TTL boundary
	ttl := 100 * time.Millisecond
	cache := NewSchemaCache(ttl)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
	}

	cache.Set("resource-1", schema)

	// Check just before expiration
	time.Sleep(50 * time.Millisecond)
	retrieved, found := cache.Get("resource-1")
	if !found {
		t.Fatal("expected to find schema before expiration")
	}

	if retrieved == nil {
		t.Fatal("expected non-nil schema")
	}

	// Check after expiration
	time.Sleep(60 * time.Millisecond)
	retrieved, found = cache.Get("resource-1")
	if found {
		t.Fatal("expected expired entry to not be found")
	}

	if retrieved != nil {
		t.Fatal("expected nil schema after expiration")
	}
}

func TestConcurrent_Read(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
	}

	cache.Set("resource-1", schema)

	done := make(chan bool)

	// Simulate concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			retrieved, found := cache.Get("resource-1")
			if !found || retrieved == nil {
				t.Error("concurrent read failed")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestConcurrent_Write(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	done := make(chan bool)

	// Simulate concurrent writes
	for i := 0; i < 10; i++ {
		go func(id int) {
			schema := &Schema{
				ResourceID:   "resource-1",
				ResourceType: "postgres",
				TableCount:   id,
			}
			cache.Set("resource-1", schema)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have one entry (final write)
	if len(cache.entries) != 1 {
		t.Errorf("expected 1 entry in cache, got %d", len(cache.entries))
	}

	retrieved, _ := cache.Get("resource-1")
	if retrieved == nil {
		t.Fatal("expected non-nil schema")
	}
}

func TestConcurrent_ReadWrite(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
	}

	cache.Set("resource-1", schema)

	done := make(chan bool)

	// Mix reads and writes
	for i := 0; i < 20; i++ {
		if i%2 == 0 {
			// Read
			go func() {
				cache.Get("resource-1")
				done <- true
			}()
		} else {
			// Write
			go func(id int) {
				schema := &Schema{
					ResourceID:   "resource-1",
					ResourceType: "postgres",
					TableCount:   id,
				}
				cache.Set("resource-1", schema)
				done <- true
			}(i)
		}
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	// Should have at least one entry
	retrieved, found := cache.Get("resource-1")
	if !found {
		t.Fatal("expected to find schema")
	}

	if retrieved == nil {
		t.Fatal("expected non-nil schema")
	}
}

func TestSet_UpdatesExpiration(t *testing.T) {
	ttl := 100 * time.Millisecond
	cache := NewSchemaCache(ttl)

	schema1 := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
	}

	cache.Set("resource-1", schema1)

	// Wait half TTL
	time.Sleep(50 * time.Millisecond)

	// Update the entry
	schema2 := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "mysql",
	}
	cache.Set("resource-1", schema2)

	// Wait another 75ms (total would be 125ms from original, but only 75ms from update)
	time.Sleep(75 * time.Millisecond)

	// Should still be valid due to second Set
	retrieved, found := cache.Get("resource-1")
	if !found {
		t.Fatal("expected entry to still be valid after update")
	}

	if retrieved.ResourceType != "mysql" {
		t.Errorf("expected updated resourceType 'mysql', got '%s'", retrieved.ResourceType)
	}
}

func TestZeroTTL(t *testing.T) {
	// Cache with zero TTL should always expire immediately
	cache := NewSchemaCache(0 * time.Second)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
	}

	cache.Set("resource-1", schema)

	// Even immediate retrieval might expire
	time.Sleep(1 * time.Millisecond)

	_, found := cache.Get("resource-1")
	if found {
		t.Fatal("expected zero-TTL entry to expire immediately")
	}
}

func TestLongTTL(t *testing.T) {
	// Cache with very long TTL
	cache := NewSchemaCache(1 * time.Hour)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
	}

	cache.Set("resource-1", schema)

	// Should still be valid after a short wait
	time.Sleep(100 * time.Millisecond)

	retrieved, found := cache.Get("resource-1")
	if !found {
		t.Fatal("expected long-TTL entry to still be valid")
	}

	if retrieved == nil {
		t.Fatal("expected non-nil schema")
	}
}

func TestInvalidate_AfterExpiration(t *testing.T) {
	cache := NewSchemaCache(10 * time.Millisecond)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
	}

	cache.Set("resource-1", schema)

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Invalidate already-expired entry
	cache.Invalidate("resource-1")

	// Should still not be found
	_, found := cache.Get("resource-1")
	if found {
		t.Fatal("expected entry to not be found")
	}
}

func TestSet_WithNilSchema(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	// Set with nil schema (edge case)
	cache.Set("resource-1", nil)

	retrieved, found := cache.Get("resource-1")
	if !found {
		t.Fatal("expected to find entry even with nil schema")
	}

	if retrieved != nil {
		t.Fatal("expected retrieved schema to be nil")
	}
}

func TestGet_EmptyKey(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
	}

	// Set with empty key
	cache.Set("", schema)

	retrieved, found := cache.Get("")
	if !found {
		t.Fatal("expected to find entry with empty key")
	}

	if retrieved == nil {
		t.Fatal("expected non-nil schema")
	}
}

func TestSchemaWithFields(t *testing.T) {
	cache := NewSchemaCache(5 * time.Minute)

	schema := &Schema{
		ResourceID:   "resource-1",
		ResourceType: "postgres",
		Fields: []SchemaField{
			{Name: "id", Type: "bigint", Nullable: false, Primary: true},
			{Name: "name", Type: "varchar", Nullable: true, Primary: false},
		},
		Indexes: []SchemaIndex{
			{Name: "pkey", Fields: []string{"id"}, Unique: true},
		},
		TableCount: 10,
	}

	cache.Set("resource-1", schema)

	retrieved, _ := cache.Get("resource-1")
	if len(retrieved.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(retrieved.Fields))
	}

	if len(retrieved.Indexes) != 1 {
		t.Errorf("expected 1 index, got %d", len(retrieved.Indexes))
	}

	if retrieved.TableCount != 10 {
		t.Errorf("expected tableCount 10, got %d", retrieved.TableCount)
	}
}
