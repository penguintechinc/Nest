package cache

import (
	"context"
	"crypto/md5" // #nosec - used for cache key generation only, not cryptographic
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Store is the interface for a query result cache
type Store interface {
	// Get retrieves a cached response by key
	Get(ctx context.Context, tenant string, targetDB string, queryText string) ([][]byte, error)

	// Set stores a response in the cache with TTL and optional table tags
	Set(ctx context.Context, tenant string, targetDB string, queryText string, frames [][]byte, ttl time.Duration, tables []string) error

	// InvalidateByTable invalidates all cache entries tagged with the given table
	InvalidateByTable(ctx context.Context, table string) error

	// Close closes the cache store
	Close() error

	// Stats returns cache statistics
	Stats() map[string]interface{}
}

// Config holds cache configuration
type Config struct {
	Enabled     bool
	TTL         time.Duration
	MaxSizeKB   int
	RedisAddr   string
	RedisPort   int
	RedisDB     int
	RedisPrefix string
}

// RedisStore implements Store using Redis
type RedisStore struct {
	client    *redis.Client
	prefix    string
	ttl       time.Duration
	maxSizeKB int
	logger    *zap.Logger

	// metrics (protected by mu)
	mu     sync.RWMutex
	hits   int64
	misses int64
	errors int64
}

// NewRedisStore creates a new Redis-backed cache store
func NewRedisStore(client *redis.Client, prefix string, ttl time.Duration, maxSizeKB int, logger *zap.Logger) *RedisStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RedisStore{
		client:    client,
		prefix:    prefix,
		ttl:       ttl,
		maxSizeKB: maxSizeKB,
		logger:    logger,
	}
}

// Get retrieves a cached response
func (rs *RedisStore) Get(ctx context.Context, tenant string, targetDB string, queryText string) ([][]byte, error) {
	cacheKey := rs.makeKey(tenant, targetDB, queryText)

	data, err := rs.client.Get(ctx, cacheKey).Bytes()
	if err == redis.Nil {
		// Cache miss
		rs.recordMiss()
		return nil, nil
	}
	if err != nil {
		rs.recordError()
		rs.logger.Debug("cache get error",
			zap.String("key", cacheKey),
			zap.Error(err),
		)
		return nil, err
	}

	// Parse frames from stored data
	frames, err := decodeFrames(data)
	if err != nil {
		rs.recordError()
		rs.logger.Debug("failed to decode cached frames",
			zap.String("key", cacheKey),
			zap.Error(err),
		)
		return nil, err
	}

	rs.recordHit()
	rs.logger.Debug("cache hit",
		zap.String("tenant", tenant),
		zap.String("target_db", targetDB),
		zap.Int("frame_count", len(frames)),
	)
	return frames, nil
}

// Set stores a response in the cache
func (rs *RedisStore) Set(ctx context.Context, tenant string, targetDB string, queryText string, frames [][]byte, ttl time.Duration, tables []string) error {
	// Check size constraint
	totalSize := 0
	for _, frame := range frames {
		totalSize += len(frame)
	}

	if rs.maxSizeKB > 0 && totalSize > rs.maxSizeKB*1024 {
		rs.logger.Debug("query result exceeds max cache size",
			zap.String("tenant", tenant),
			zap.String("target_db", targetDB),
			zap.Int("size_kb", totalSize/1024),
			zap.Int("max_size_kb", rs.maxSizeKB),
		)
		return nil // silently skip caching
	}

	cacheKey := rs.makeKey(tenant, targetDB, queryText)

	// Encode frames
	data, err := encodeFrames(frames)
	if err != nil {
		rs.recordError()
		rs.logger.Debug("failed to encode frames for cache",
			zap.String("key", cacheKey),
			zap.Error(err),
		)
		return err
	}

	// Use configured TTL if provided
	if ttl == 0 {
		ttl = rs.ttl
	}

	// Set cache entry
	if err := rs.client.Set(ctx, cacheKey, data, ttl).Err(); err != nil {
		rs.recordError()
		rs.logger.Debug("cache set error",
			zap.String("key", cacheKey),
			zap.Error(err),
		)
		return err
	}

	// Tag with tables for invalidation (best-effort)
	if len(tables) > 0 {
		for _, table := range tables {
			tableTagKey := rs.makeTableTagKey(table)
			// Add cache key to table tag set (with TTL)
			if err := rs.client.SAdd(ctx, tableTagKey, cacheKey).Err(); err != nil {
				rs.logger.Debug("failed to add table tag",
					zap.String("table", table),
					zap.String("cache_key", cacheKey),
					zap.Error(err),
				)
				// Continue despite error - tag is optional
			}
			// Expire table tag after TTL
			_ = rs.client.Expire(ctx, tableTagKey, ttl).Err()
		}
	}

	rs.logger.Debug("cache set",
		zap.String("tenant", tenant),
		zap.String("target_db", targetDB),
		zap.Int("frame_count", len(frames)),
		zap.Int("size_kb", totalSize/1024),
		zap.Duration("ttl", ttl),
		zap.Strings("tables", tables),
	)
	return nil
}

// InvalidateByTable invalidates cache entries tagged with a table
func (rs *RedisStore) InvalidateByTable(ctx context.Context, table string) error {
	tableTagKey := rs.makeTableTagKey(table)

	// Get all cache keys for this table
	members, err := rs.client.SMembers(ctx, tableTagKey).Result()
	if err != nil {
		rs.logger.Debug("failed to get table tag members",
			zap.String("table", table),
			zap.Error(err),
		)
		return err
	}

	if len(members) == 0 {
		return nil
	}

	// Delete all tagged cache entries
	if err := rs.client.Del(ctx, members...).Err(); err != nil {
		rs.logger.Debug("failed to delete cache entries by table",
			zap.String("table", table),
			zap.Error(err),
		)
		return err
	}

	// Delete the table tag set itself
	_ = rs.client.Del(ctx, tableTagKey).Err()

	rs.logger.Debug("invalidated cache by table",
		zap.String("table", table),
		zap.Int("entries_deleted", len(members)),
	)
	return nil
}

// Close closes the Redis connection
func (rs *RedisStore) Close() error {
	return rs.client.Close()
}

// Stats returns cache statistics
func (rs *RedisStore) Stats() map[string]interface{} {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	total := rs.hits + rs.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(rs.hits) / float64(total) * 100
	}

	return map[string]interface{}{
		"hits":     rs.hits,
		"misses":   rs.misses,
		"errors":   rs.errors,
		"hit_rate": hitRate,
		"total":    total,
	}
}

// makeKey generates a cache key from tenant, targetDB, and query text
// Uses MD5 for cache key generation (not cryptographic, for performance only)
func (rs *RedisStore) makeKey(tenant string, targetDB string, queryText string) string {
	normalized := normalizeQuery(queryText)
	// #nosec - MD5 is used for cache key generation only, not cryptographic purposes
	hash := md5.Sum([]byte(normalized))
	hashStr := hex.EncodeToString(hash[:])
	return fmt.Sprintf("%s:query:%s:%s:%s", rs.prefix, tenant, targetDB, hashStr)
}

// makeTableTagKey generates a tag key for table-based invalidation
func (rs *RedisStore) makeTableTagKey(table string) string {
	return fmt.Sprintf("%s:table_tag:%s", rs.prefix, table)
}

// recordHit increments the hit counter
func (rs *RedisStore) recordHit() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.hits++
}

// recordMiss increments the miss counter
func (rs *RedisStore) recordMiss() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.misses++
}

// recordError increments the error counter
func (rs *RedisStore) recordError() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.errors++
}

// NoOpStore is a cache store that does nothing (when caching is disabled)
type NoOpStore struct{}

func (n *NoOpStore) Get(ctx context.Context, tenant string, targetDB string, queryText string) ([][]byte, error) {
	return nil, nil
}

func (n *NoOpStore) Set(ctx context.Context, tenant string, targetDB string, queryText string, frames [][]byte, ttl time.Duration, tables []string) error {
	return nil
}

func (n *NoOpStore) InvalidateByTable(ctx context.Context, table string) error {
	return nil
}

func (n *NoOpStore) Close() error {
	return nil
}

func (n *NoOpStore) Stats() map[string]interface{} {
	return map[string]interface{}{
		"hits":     0,
		"misses":   0,
		"errors":   0,
		"hit_rate": 0.0,
		"total":    0,
	}
}
