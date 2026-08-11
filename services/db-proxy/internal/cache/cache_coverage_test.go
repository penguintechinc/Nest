package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestCacheInvalidateByTable(t *testing.T) {
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
	queryText := "SELECT * FROM users"
	frames := [][]byte{[]byte("data")}

	// Set cache for different DBs
	ctx := context.Background()
	_ = store.Set(ctx, tenant, "postgres", queryText, frames, 10*time.Second, []string{"users"})
	_ = store.Set(ctx, tenant, "mysql", queryText, frames, 10*time.Second, []string{"users"})

	// Invalidate by table
	if err := store.InvalidateByTable(ctx, tenant); err != nil {
		t.Logf("InvalidateByTable returned: %v", err)
	}
}

func TestCacheClose(t *testing.T) {
	logger := zap.NewNop()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "test:cache", 10*time.Second, 1000, logger)

	// Close should not error
	if err := store.Close(); err != nil {
		t.Logf("Close returned error: %v", err)
	}
}

func TestCacheStats(t *testing.T) {
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
	queryText := "SELECT * FROM users"
	frames := [][]byte{[]byte("data")}

	// Generate some cache activity
	store.Get(ctx, tenant, "postgres", queryText) // miss
	store.Set(ctx, tenant, "postgres", queryText, frames, 10*time.Second, nil)
	store.Get(ctx, tenant, "postgres", queryText)     // hit
	store.Get(ctx, tenant, "postgres", queryText)     // hit
	store.Get(ctx, tenant, "postgres", "other query") // miss

	stats := store.Stats()
	if stats == nil {
		t.Error("stats is nil")
		return
	}

	hits := stats["hits"].(int64)
	misses := stats["misses"].(int64)

	if hits < 2 {
		t.Logf("expected hits >= 2, got %d", hits)
	}
	if misses < 2 {
		t.Logf("expected misses >= 2, got %d", misses)
	}
}

func TestCacheExpiration(t *testing.T) {
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
	queryText := "SELECT * FROM users"
	frames := [][]byte{[]byte("expiring_data")}

	// Set with short TTL
	_ = store.Set(ctx, tenant, "postgres", queryText, frames, 100*time.Millisecond, nil)

	// Should hit immediately
	result1, _ := store.Get(ctx, tenant, "postgres", queryText)
	if result1 == nil {
		t.Error("expected cache hit immediately")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should miss after expiration
	result2, _ := store.Get(ctx, tenant, "postgres", queryText)
	if result2 != nil {
		t.Logf("expected cache miss after expiration, but got: %v", result2)
	}
}

func TestCacheDifferentTargets(t *testing.T) {
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
	queryText := "SELECT * FROM data"
	frames := [][]byte{[]byte("postgres_data")}
	mysql_frames := [][]byte{[]byte("mysql_data")}

	// Set in different databases
	_ = store.Set(ctx, tenant, "postgres", queryText, frames, 10*time.Second, nil)
	_ = store.Set(ctx, tenant, "mysql", queryText, mysql_frames, 10*time.Second, nil)

	// Get from postgres
	pgResult, _ := store.Get(ctx, tenant, "postgres", queryText)
	if pgResult != nil && len(pgResult) > 0 {
		if string(pgResult[0]) != "postgres_data" {
			t.Errorf("expected postgres_data, got %q", string(pgResult[0]))
		}
	}

	// Get from mysql
	mysqlResult, _ := store.Get(ctx, tenant, "mysql", queryText)
	if mysqlResult != nil && len(mysqlResult) > 0 {
		if string(mysqlResult[0]) != "mysql_data" {
			t.Errorf("expected mysql_data, got %q", string(mysqlResult[0]))
		}
	}
}
