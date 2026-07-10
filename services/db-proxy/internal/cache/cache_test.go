package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TestCacheHitAndMiss tests basic cache hit/miss behavior
func TestCacheHitAndMiss(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	// Setup miniredis
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "test:cache", 10*time.Second, 1000, logger)

	tenant := "tenant-1"
	targetDB := "postgres"
	queryText := "SELECT * FROM users"
	frames := [][]byte{
		[]byte("frame1"),
		[]byte("frame2"),
	}

	// First call should be a miss
	result, err := store.Get(ctx, tenant, targetDB, queryText)
	if err != nil {
		t.Fatalf("unexpected error on cache miss: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil on cache miss, got %v", result)
	}

	stats := store.Stats()
	if stats["misses"] != int64(1) {
		t.Errorf("expected 1 miss, got %v", stats["misses"])
	}

	// Set in cache
	err = store.Set(ctx, tenant, targetDB, queryText, frames, 10*time.Second, nil)
	if err != nil {
		t.Fatalf("failed to set cache: %v", err)
	}

	// Second call should be a hit
	result, err = store.Get(ctx, tenant, targetDB, queryText)
	if err != nil {
		t.Fatalf("unexpected error on cache hit: %v", err)
	}
	if result == nil {
		t.Fatalf("expected non-nil on cache hit")
	}
	if len(result) != len(frames) {
		t.Fatalf("expected %d frames, got %d", len(frames), len(result))
	}

	for i, frame := range result {
		if string(frame) != string(frames[i]) {
			t.Errorf("frame %d mismatch: expected %q, got %q", i, frames[i], frame[i])
		}
	}

	stats = store.Stats()
	if stats["hits"] != int64(1) {
		t.Errorf("expected 1 hit, got %v", stats["hits"])
	}
}

// TestTenantIsolation ensures different tenants have separate cache entries
func TestTenantIsolation(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "test:cache", 10*time.Second, 1000, logger)

	tenant1 := "tenant-1"
	tenant2 := "tenant-2"
	targetDB := "postgres"
	queryText := "SELECT * FROM users"
	frames1 := [][]byte{[]byte("tenant1_data")}
	frames2 := [][]byte{[]byte("tenant2_data")}

	// Set for tenant 1
	err := store.Set(ctx, tenant1, targetDB, queryText, frames1, 10*time.Second, nil)
	if err != nil {
		t.Fatalf("failed to set cache for tenant1: %v", err)
	}

	// Set for tenant 2
	err = store.Set(ctx, tenant2, targetDB, queryText, frames2, 10*time.Second, nil)
	if err != nil {
		t.Fatalf("failed to set cache for tenant2: %v", err)
	}

	// Verify tenant 1 gets its own data
	result1, _ := store.Get(ctx, tenant1, targetDB, queryText)
	if len(result1) != 1 || string(result1[0]) != "tenant1_data" {
		t.Errorf("tenant1 got wrong data: %v", result1)
	}

	// Verify tenant 2 gets its own data
	result2, _ := store.Get(ctx, tenant2, targetDB, queryText)
	if len(result2) != 1 || string(result2[0]) != "tenant2_data" {
		t.Errorf("tenant2 got wrong data: %v", result2)
	}
}

// TestMaxSizeConstraint verifies that oversized entries are not cached
func TestMaxSizeConstraint(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Store with max size of 1 KB
	store := NewRedisStore(client, "test:cache", 10*time.Second, 1, logger)

	tenant := "tenant-1"
	targetDB := "postgres"
	queryText := "SELECT * FROM large_table"

	// Create a frame larger than 1 KB
	largeFrame := make([]byte, 2*1024) // 2 KB
	frames := [][]byte{largeFrame}

	// This should not error, but silently skip caching
	err := store.Set(ctx, tenant, targetDB, queryText, frames, 10*time.Second, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's not in cache
	result, _ := store.Get(ctx, tenant, targetDB, queryText)
	if result != nil {
		t.Fatalf("expected nil for oversized entry, got %v", result)
	}
}

// TestTableInvalidation verifies cache invalidation by table
func TestTableInvalidation(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "test:cache", 10*time.Second, 1000, logger)

	tenant := "tenant-1"
	targetDB := "postgres"

	// Set two queries with table tags
	query1 := "SELECT * FROM users"
	frames1 := [][]byte{[]byte("data1")}
	err := store.Set(ctx, tenant, targetDB, query1, frames1, 10*time.Second, []string{"users"})
	if err != nil {
		t.Fatalf("failed to set cache: %v", err)
	}

	query2 := "SELECT * FROM orders"
	frames2 := [][]byte{[]byte("data2")}
	err = store.Set(ctx, tenant, targetDB, query2, frames2, 10*time.Second, []string{"orders"})
	if err != nil {
		t.Fatalf("failed to set cache: %v", err)
	}

	// Verify both are cached
	result1, _ := store.Get(ctx, tenant, targetDB, query1)
	result2, _ := store.Get(ctx, tenant, targetDB, query2)
	if result1 == nil || result2 == nil {
		t.Fatalf("expected both queries in cache")
	}

	// Invalidate the users table
	err = store.InvalidateByTable(ctx, "users")
	if err != nil {
		t.Fatalf("failed to invalidate table: %v", err)
	}

	// Verify users query is gone
	result1, _ = store.Get(ctx, tenant, targetDB, query1)
	if result1 != nil {
		t.Fatalf("expected users query to be invalidated, but it's still there")
	}

	// Verify orders query is still there
	result2, _ = store.Get(ctx, tenant, targetDB, query2)
	if result2 == nil {
		t.Fatalf("expected orders query to still be cached")
	}
}

// TestQueryNormalization verifies that different query formats produce the same cache key
func TestQueryNormalization(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "test:cache", 10*time.Second, 1000, logger)

	tenant := "tenant-1"
	targetDB := "postgres"

	// Various forms of the same query
	query1 := "SELECT * FROM users"
	query2 := "SELECT  *  FROM  users"                   // Extra spaces
	query3 := "select * from users"                      // Lowercase
	query4 := "-- comment\nSELECT * FROM users"          // With comment
	query5 := "SELECT * /* inline comment */ FROM users" // Inline comment

	frames := [][]byte{[]byte("cached_data")}

	// Cache using first form
	err := store.Set(ctx, tenant, targetDB, query1, frames, 10*time.Second, nil)
	if err != nil {
		t.Fatalf("failed to set cache: %v", err)
	}

	// All other forms should hit the same cache entry
	for i, query := range []string{query2, query3, query4, query5} {
		result, _ := store.Get(ctx, tenant, targetDB, query)
		if result == nil {
			t.Errorf("query%d should hit cache after normalization", i+2)
		}
	}
}

// TestCacheability verifies IsCacheable logic
func TestCacheability(t *testing.T) {
	tests := []struct {
		name          string
		queryType     int // 1=SELECT, 2=INSERT, 3=UPDATE, 4=DELETE, 5=DDL
		queryText     string
		inTransaction bool
		stateDirty    bool
		expected      bool
		description   string
	}{
		{
			name:          "simple_select",
			queryType:     1, // SELECT
			queryText:     "SELECT * FROM users",
			inTransaction: false,
			stateDirty:    false,
			expected:      true,
			description:   "Simple SELECT should be cacheable",
		},
		{
			name:          "select_for_update",
			queryType:     1, // SELECT
			queryText:     "SELECT * FROM users FOR UPDATE",
			inTransaction: false,
			stateDirty:    false,
			expected:      false,
			description:   "SELECT FOR UPDATE should not be cacheable",
		},
		{
			name:          "select_with_now",
			queryType:     1, // SELECT
			queryText:     "SELECT NOW() FROM users",
			inTransaction: false,
			stateDirty:    false,
			expected:      false,
			description:   "SELECT with NOW() should not be cacheable",
		},
		{
			name:          "select_with_rand",
			queryType:     1, // SELECT
			queryText:     "SELECT RAND() FROM users",
			inTransaction: false,
			stateDirty:    false,
			expected:      false,
			description:   "SELECT with RAND() should not be cacheable",
		},
		{
			name:          "select_in_transaction",
			queryType:     1, // SELECT
			queryText:     "SELECT * FROM users",
			inTransaction: true,
			stateDirty:    false,
			expected:      false,
			description:   "SELECT in transaction should not be cacheable",
		},
		{
			name:          "select_dirty_session",
			queryType:     1, // SELECT
			queryText:     "SELECT * FROM users",
			inTransaction: false,
			stateDirty:    true,
			expected:      false,
			description:   "SELECT with dirty session should not be cacheable",
		},
		{
			name:          "insert_query",
			queryType:     2, // INSERT
			queryText:     "INSERT INTO users VALUES (1, 'John')",
			inTransaction: false,
			stateDirty:    false,
			expected:      false,
			description:   "INSERT should not be cacheable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := IsCacheable(test.queryType, test.queryText, test.inTransaction, test.stateDirty)
			if result != test.expected {
				t.Errorf("%s: expected %v, got %v", test.description, test.expected, result)
			}
		})
	}
}

// TestExtractTablesFromQuery verifies table extraction from different query types
func TestExtractTablesFromQuery(t *testing.T) {
	tests := []struct {
		name      string
		queryType int
		queryText string
		expected  []string
	}{
		{
			name:      "insert_simple",
			queryType: 2, // INSERT
			queryText: "INSERT INTO users VALUES (1, 'John')",
			expected:  []string{"USERS"},
		},
		{
			name:      "update_simple",
			queryType: 3, // UPDATE
			queryText: "UPDATE users SET name='Jane' WHERE id=1",
			expected:  []string{"USERS"},
		},
		{
			name:      "delete_simple",
			queryType: 4, // DELETE
			queryText: "DELETE FROM users WHERE id=1",
			expected:  []string{"USERS"},
		},
		{
			name:      "insert_with_schema",
			queryType: 2, // INSERT
			queryText: "INSERT INTO public.users VALUES (1, 'John')",
			expected:  []string{"USERS"},
		},
		{
			name:      "update_with_backticks",
			queryType: 3, // UPDATE
			queryText: "UPDATE `users` SET name='Jane'",
			expected:  []string{"USERS"},
		},
		{
			name:      "select_extracts_tables",
			queryType: 1, // SELECT
			queryText: "SELECT * FROM users",
			expected:  []string{"USERS"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ExtractTablesFromQuery(test.queryType, test.queryText)
			if len(result) != len(test.expected) {
				t.Errorf("expected %v tables, got %v", len(test.expected), len(result))
				return
			}
			for i, table := range result {
				if table != test.expected[i] {
					t.Errorf("table %d: expected %q, got %q", i, test.expected[i], table)
				}
			}
		})
	}
}

// BenchmarkCacheGet benchmarks cache retrieval
func BenchmarkCacheGet(b *testing.B) {
	ctx := context.Background()
	logger := zap.NewNop()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "test:cache", 10*time.Second, 1000, logger)

	tenant := "tenant-1"
	targetDB := "postgres"
	queryText := "SELECT * FROM users"
	frames := [][]byte{[]byte("frame1"), []byte("frame2")}

	// Pre-populate cache
	store.Set(ctx, tenant, targetDB, queryText, frames, 10*time.Second, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Get(ctx, tenant, targetDB, queryText)
	}
}

// BenchmarkCacheSet benchmarks cache storage
func BenchmarkCacheSet(b *testing.B) {
	ctx := context.Background()
	logger := zap.NewNop()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "test:cache", 10*time.Second, 1000, logger)

	tenant := "tenant-1"
	targetDB := "postgres"
	frames := [][]byte{[]byte("frame1"), []byte("frame2")}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queryText := fmt.Sprintf("SELECT * FROM table_%d", i)
		store.Set(ctx, tenant, targetDB, queryText, frames, 10*time.Second, nil)
	}
}
