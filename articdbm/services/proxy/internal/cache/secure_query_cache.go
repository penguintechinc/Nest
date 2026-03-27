package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"

	"github.com/penguintechinc/articdbm/proxy/internal/auth"
)

// SecureQueryCache provides authorization-aware query caching with LRU eviction
type SecureQueryCache struct {
	redisClient *redis.Client
	authManager *auth.Manager
	config      *SecureCacheConfig
	logger      *zap.Logger
	stats       *CacheStats
	mu          sync.RWMutex
}

type SecureCacheConfig struct {
	Enabled              bool          `json:"enabled"`
	MaxEntries           int64         `json:"max_entries"`
	MaxMemory            int64         `json:"max_memory"`           // bytes
	DefaultTTL           time.Duration `json:"default_ttl"`
	EvictionPolicy       string        `json:"eviction_policy"`       // "lru", "lfu", "ttl"
	AuthValidation       bool          `json:"auth_validation"`       // Always validate auth
	PermissionCaching    bool          `json:"permission_caching"`    // Cache permission checks
	PermissionCacheTTL   time.Duration `json:"permission_cache_ttl"`
	HitCounterEnabled    bool          `json:"hit_counter_enabled"`
	MinHitsToCache       int           `json:"min_hits_to_cache"`     // Min hits before caching
	CompressionThreshold int           `json:"compression_threshold"` // bytes
}

// CacheEntry represents a cached query result with metadata
type CacheEntry struct {
	Query       string                 `json:"query"`
	Result      []byte                 `json:"result"`
	Database    string                 `json:"database"`
	Table       string                 `json:"table"`
	Permissions QueryPermissions       `json:"permissions"`
	HitCount    int64                  `json:"hit_count"`
	CreatedAt   time.Time              `json:"created_at"`
	LastAccess  time.Time              `json:"last_access"`
	TTL         time.Duration          `json:"ttl"`
	Size        int64                  `json:"size"`
	Compressed  bool                   `json:"compressed"`
	UserGroups  []string               `json:"user_groups"`  // Groups allowed to access
	IPRanges    []string               `json:"ip_ranges"`    // IP ranges allowed
	Metadata    map[string]interface{} `json:"metadata"`
}

// QueryPermissions defines access control for cached queries
type QueryPermissions struct {
	RequiredRoles       []string          `json:"required_roles"`
	RequiredPermissions []string          `json:"required_permissions"`
	AllowedUsers        []string          `json:"allowed_users"`
	DeniedUsers         []string          `json:"denied_users"`
	RequireTLS          bool              `json:"require_tls"`
	RequireAPIKey       bool              `json:"require_api_key"`
	DatabasePermissions map[string]string `json:"database_permissions"`
	TablePermissions    map[string]string `json:"table_permissions"`
	ColumnRestrictions  []string          `json:"column_restrictions"`
	RowLevelSecurity    string            `json:"row_level_security"`
}

// CacheRequest includes user context for authorization
type SecureCacheRequest struct {
	Query       string              `json:"query"`
	Database    string              `json:"database"`
	Table       string              `json:"table"`
	User        string              `json:"user"`
	UserGroups  []string            `json:"user_groups"`
	Roles       []string            `json:"roles"`
	Permissions []string            `json:"permissions"`
	APIKey      string              `json:"api_key"`
	IPAddress   string              `json:"ip_address"`
	UseTLS      bool                `json:"use_tls"`
	Context     context.Context     `json:"-"`
}

// CacheStats tracks cache performance and usage
type CacheStats struct {
	TotalRequests      uint64    `json:"total_requests"`
	CacheHits          uint64    `json:"cache_hits"`
	CacheMisses        uint64    `json:"cache_misses"`
	AuthFailures       uint64    `json:"auth_failures"`
	Evictions          uint64    `json:"evictions"`
	CurrentEntries     int64     `json:"current_entries"`
	CurrentMemory      int64     `json:"current_memory"`
	HitRatio           float64   `json:"hit_ratio"`
	AvgHitCount        float64   `json:"avg_hit_count"`
	TopQueries         []string  `json:"top_queries"`
	LastEviction       time.Time `json:"last_eviction"`
	LastCleanup        time.Time `json:"last_cleanup"`
}

func NewSecureQueryCache(redisClient *redis.Client, authManager *auth.Manager, config *SecureCacheConfig, logger *zap.Logger) *SecureQueryCache {
	cache := &SecureQueryCache{
		redisClient: redisClient,
		authManager: authManager,
		config:      config,
		logger:      logger,
		stats:       &CacheStats{},
	}

	// Start background maintenance tasks
	go cache.maintenanceLoop()
	go cache.statsCollector()

	logger.Info("Secure query cache initialized",
		zap.Int64("max_entries", config.MaxEntries),
		zap.Int64("max_memory", config.MaxMemory),
		zap.String("eviction_policy", config.EvictionPolicy))

	return cache
}

// Get retrieves a cached query result with authorization validation
func (c *SecureQueryCache) Get(ctx context.Context, request *SecureCacheRequest) ([]byte, bool, error) {
	if !c.config.Enabled {
		return nil, false, nil
	}

	atomic.AddUint64(&c.stats.TotalRequests, 1)

	// Generate cache key
	cacheKey := c.generateCacheKey(request)

	// Get cache entry from Redis
	entryData, err := c.redisClient.Get(ctx, cacheKey).Result()
	if err == redis.Nil {
		atomic.AddUint64(&c.stats.CacheMisses, 1)
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	// Deserialize cache entry
	var entry CacheEntry
	if err := json.Unmarshal([]byte(entryData), &entry); err != nil {
		c.logger.Error("Failed to unmarshal cache entry", zap.Error(err))
		return nil, false, err
	}

	// CRITICAL: Validate user authorization before returning cached result
	if !c.validateAuthorization(ctx, request, &entry) {
		atomic.AddUint64(&c.stats.AuthFailures, 1)
		c.logger.Warn("Cache authorization denied",
			zap.String("user", request.User),
			zap.String("query", request.Query[:min(50, len(request.Query))]))
		return nil, false, nil
	}

	// Update hit counter and last access time
	atomic.AddInt64(&entry.HitCount, 1)
	entry.LastAccess = time.Now()

	// Update entry in Redis with new hit count
	c.updateHitCounter(ctx, cacheKey, &entry)

	atomic.AddUint64(&c.stats.CacheHits, 1)

	// Decompress if needed
	result := entry.Result
	if entry.Compressed {
		result = c.decompress(result)
	}

	return result, true, nil
}

// Set stores a query result with permission metadata
func (c *SecureQueryCache) Set(ctx context.Context, request *SecureCacheRequest, result []byte) error {
	if !c.config.Enabled {
		return nil
	}

	// Check if we should cache based on hit counter
	if c.config.HitCounterEnabled {
		hitCount := c.getQueryHitCount(ctx, request)
		if hitCount < int64(c.config.MinHitsToCache) {
			// Track the query but don't cache yet
			c.trackQueryAccess(ctx, request)
			return nil
		}
	}

	// Check cache capacity
	if c.stats.CurrentEntries >= c.config.MaxEntries {
		// Evict least recently used entry
		if err := c.evictLRUEntry(ctx); err != nil {
			c.logger.Warn("Failed to evict LRU entry", zap.Error(err))
		}
	}

	// Create cache entry with permissions
	entry := &CacheEntry{
		Query:      request.Query,
		Result:     result,
		Database:   request.Database,
		Table:      request.Table,
		HitCount:   1,
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		TTL:        c.config.DefaultTTL,
		Size:       int64(len(result)),
		UserGroups: request.UserGroups,
		Permissions: QueryPermissions{
			RequiredRoles:       request.Roles,
			RequiredPermissions: request.Permissions,
			AllowedUsers:        []string{request.User},
			RequireTLS:          request.UseTLS,
			RequireAPIKey:       request.APIKey != "",
			DatabasePermissions: c.extractDatabasePermissions(request),
			TablePermissions:    c.extractTablePermissions(request),
		},
	}

	// Compress if over threshold
	if len(result) > c.config.CompressionThreshold {
		entry.Result = c.compress(result)
		entry.Compressed = true
	}

	// Serialize and store in Redis
	entryData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	cacheKey := c.generateCacheKey(request)
	if err := c.redisClient.Set(ctx, cacheKey, entryData, entry.TTL).Err(); err != nil {
		return err
	}

	// Update cache statistics
	atomic.AddInt64(&c.stats.CurrentEntries, 1)
	atomic.AddInt64(&c.stats.CurrentMemory, entry.Size)

	// Store in sorted set for LRU tracking
	score := float64(time.Now().Unix())
	c.redisClient.ZAdd(ctx, "articdbm:cache:lru", &redis.Z{
		Score:  score,
		Member: cacheKey,
	})

	return nil
}

// validateAuthorization checks if user has permission to access cached result
func (c *SecureQueryCache) validateAuthorization(ctx context.Context, request *SecureCacheRequest, entry *CacheEntry) bool {
	// Always validate authorization if enabled
	if !c.config.AuthValidation {
		return true
	}

	// Check if user is explicitly denied
	for _, deniedUser := range entry.Permissions.DeniedUsers {
		if request.User == deniedUser {
			return false
		}
	}

	// Check if user is explicitly allowed
	for _, allowedUser := range entry.Permissions.AllowedUsers {
		if request.User == allowedUser {
			return true
		}
	}

	// Check TLS requirement
	if entry.Permissions.RequireTLS && !request.UseTLS {
		return false
	}

	// Check API key requirement
	if entry.Permissions.RequireAPIKey && request.APIKey == "" {
		return false
	}

	// Check user groups
	if len(entry.UserGroups) > 0 {
		hasGroup := false
		for _, requiredGroup := range entry.UserGroups {
			for _, userGroup := range request.UserGroups {
				if requiredGroup == userGroup {
					hasGroup = true
					break
				}
			}
		}
		if !hasGroup {
			return false
		}
	}

	// Check roles
	if len(entry.Permissions.RequiredRoles) > 0 {
		hasRole := false
		for _, requiredRole := range entry.Permissions.RequiredRoles {
			for _, userRole := range request.Roles {
				if requiredRole == userRole {
					hasRole = true
					break
				}
			}
		}
		if !hasRole {
			return false
		}
	}

	// Check specific permissions
	if len(entry.Permissions.RequiredPermissions) > 0 {
		hasPermission := false
		for _, requiredPerm := range entry.Permissions.RequiredPermissions {
			for _, userPerm := range request.Permissions {
				if requiredPerm == userPerm {
					hasPermission = true
					break
				}
			}
		}
		if !hasPermission {
			return false
		}
	}

	// Check database-level permissions
	if entry.Database != "" && request.Database != entry.Database {
		return false
	}

	// Check table-level permissions
	if entry.Table != "" && request.Table != entry.Table {
		return false
	}

	// Additional authorization checks via auth manager
	if c.authManager != nil {
		user, err := c.authManager.GetUser(ctx, request.User)
		if err != nil || user == nil {
			return false
		}

		// Check if user has permission for this database
		if !c.authManager.HasDatabaseAccess(ctx, user, entry.Database) {
			return false
		}

		// Check if user has permission for this table
		if !c.authManager.HasTableAccess(ctx, user, entry.Database, entry.Table) {
			return false
		}

		// Check IP restrictions
		if !c.authManager.IsIPAllowed(ctx, user, request.IPAddress) {
			return false
		}
	}

	return true
}

// evictLRUEntry removes the least recently used cache entry
func (c *SecureQueryCache) evictLRUEntry(ctx context.Context) error {
	// Get entry with lowest score (oldest access time)
	members, err := c.redisClient.ZRangeWithScores(ctx, "articdbm:cache:lru", 0, 0).Result()
	if err != nil || len(members) == 0 {
		return err
	}

	// Remove the entry
	cacheKey := members[0].Member.(string)

	// Get entry data for statistics
	entryData, _ := c.redisClient.Get(ctx, cacheKey).Result()
	if entryData != "" {
		var entry CacheEntry
		if json.Unmarshal([]byte(entryData), &entry) == nil {
			atomic.AddInt64(&c.stats.CurrentMemory, -entry.Size)
		}
	}

	// Delete from Redis
	c.redisClient.Del(ctx, cacheKey)
	c.redisClient.ZRem(ctx, "articdbm:cache:lru", cacheKey)

	atomic.AddInt64(&c.stats.CurrentEntries, -1)
	atomic.AddUint64(&c.stats.Evictions, 1)
	c.stats.LastEviction = time.Now()

	c.logger.Debug("Evicted LRU cache entry",
		zap.String("key", cacheKey))

	return nil
}

// updateHitCounter updates the hit count for a cache entry
func (c *SecureQueryCache) updateHitCounter(ctx context.Context, cacheKey string, entry *CacheEntry) {
	// Update entry in Redis
	entryData, err := json.Marshal(entry)
	if err != nil {
		return
	}

	// Update with remaining TTL
	ttl, _ := c.redisClient.TTL(ctx, cacheKey).Result()
	if ttl > 0 {
		c.redisClient.Set(ctx, cacheKey, entryData, ttl)
	}

	// Update LRU score
	score := float64(time.Now().Unix())
	c.redisClient.ZAdd(ctx, "articdbm:cache:lru", &redis.Z{
		Score:  score,
		Member: cacheKey,
	})
}

// getQueryHitCount returns the number of times a query has been accessed
func (c *SecureQueryCache) getQueryHitCount(ctx context.Context, request *SecureCacheRequest) int64 {
	trackingKey := c.generateTrackingKey(request)
	count, _ := c.redisClient.Get(ctx, trackingKey).Int64()
	return count
}

// trackQueryAccess increments the access counter for a query
func (c *SecureQueryCache) trackQueryAccess(ctx context.Context, request *SecureCacheRequest) {
	trackingKey := c.generateTrackingKey(request)
	c.redisClient.Incr(ctx, trackingKey)
	c.redisClient.Expire(ctx, trackingKey, 1*time.Hour)
}

// generateCacheKey creates a unique key for caching
func (c *SecureQueryCache) generateCacheKey(request *SecureCacheRequest) string {
	hasher := sha256.New()
	hasher.Write([]byte(request.Query))
	hasher.Write([]byte(request.Database))
	hasher.Write([]byte(request.Table))
	hash := hasher.Sum(nil)
	return fmt.Sprintf("articdbm:cache:query:%x", hash[:16])
}

// generateTrackingKey creates a key for tracking query access
func (c *SecureQueryCache) generateTrackingKey(request *SecureCacheRequest) string {
	hasher := sha256.New()
	hasher.Write([]byte(request.Query))
	hasher.Write([]byte(request.Database))
	hash := hasher.Sum(nil)
	return fmt.Sprintf("articdbm:cache:track:%x", hash[:8])
}

// extractDatabasePermissions gets database permissions from request
func (c *SecureQueryCache) extractDatabasePermissions(request *SecureCacheRequest) map[string]string {
	perms := make(map[string]string)
	if request.Database != "" {
		perms[request.Database] = "read"
	}
	return perms
}

// extractTablePermissions gets table permissions from request
func (c *SecureQueryCache) extractTablePermissions(request *SecureCacheRequest) map[string]string {
	perms := make(map[string]string)
	if request.Table != "" {
		perms[request.Table] = "read"
	}
	return perms
}

// compress compresses data for storage
func (c *SecureQueryCache) compress(data []byte) []byte {
	// Implementation would use gzip or similar
	return data
}

// decompress decompresses data from storage
func (c *SecureQueryCache) decompress(data []byte) []byte {
	// Implementation would use gzip or similar
	return data
}

// maintenanceLoop performs periodic cache maintenance
func (c *SecureQueryCache) maintenanceLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()

		// Clean up expired entries
		c.cleanupExpiredEntries(ctx)

		// Update statistics
		c.updateStatistics(ctx)

		// Perform memory management if needed
		if c.stats.CurrentMemory > c.config.MaxMemory {
			c.evictByMemoryPressure(ctx)
		}
	}
}

// cleanupExpiredEntries removes expired cache entries
func (c *SecureQueryCache) cleanupExpiredEntries(ctx context.Context) {
	// Remove entries with score older than 24 hours
	maxScore := float64(time.Now().Add(-24 * time.Hour).Unix())
	removed, _ := c.redisClient.ZRemRangeByScore(ctx, "articdbm:cache:lru", "0", fmt.Sprintf("%f", maxScore)).Result()

	if removed > 0 {
		atomic.AddInt64(&c.stats.CurrentEntries, -removed)
		c.logger.Info("Cleaned up expired cache entries", zap.Int64("removed", removed))
	}

	c.stats.LastCleanup = time.Now()
}

// evictByMemoryPressure evicts entries when memory limit is exceeded
func (c *SecureQueryCache) evictByMemoryPressure(ctx context.Context) {
	targetMemory := int64(float64(c.config.MaxMemory) * 0.9) // Target 90% of max

	for c.stats.CurrentMemory > targetMemory {
		if err := c.evictLRUEntry(ctx); err != nil {
			break
		}
	}
}

// statsCollector collects cache statistics
func (c *SecureQueryCache) statsCollector() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		c.updateStatistics(context.Background())
	}
}

// updateStatistics updates cache statistics
func (c *SecureQueryCache) updateStatistics(ctx context.Context) {
	// Calculate hit ratio
	total := c.stats.CacheHits + c.stats.CacheMisses
	if total > 0 {
		c.stats.HitRatio = float64(c.stats.CacheHits) / float64(total)
	}

	// Get top queries
	topQueries, _ := c.redisClient.ZRevRangeWithScores(ctx, "articdbm:cache:lru", 0, 9).Result()
	c.stats.TopQueries = make([]string, 0, len(topQueries))
	for _, q := range topQueries {
		c.stats.TopQueries = append(c.stats.TopQueries, q.Member.(string))
	}

	// Store stats in Redis for monitoring
	statsData, _ := json.Marshal(c.stats)
	c.redisClient.Set(ctx, "articdbm:cache:stats", statsData, 0)
}

// InvalidateUserCache invalidates all cache entries for a specific user
func (c *SecureQueryCache) InvalidateUserCache(ctx context.Context, user string) error {
	// This would scan and invalidate all entries accessible by the user
	pattern := "articdbm:cache:query:*"
	iter := c.redisClient.Scan(ctx, 0, pattern, 100).Iterator()

	invalidated := 0
	for iter.Next(ctx) {
		key := iter.Val()
		entryData, err := c.redisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var entry CacheEntry
		if json.Unmarshal([]byte(entryData), &entry) != nil {
			continue
		}

		// Check if user has access to this entry
		for _, allowedUser := range entry.Permissions.AllowedUsers {
			if allowedUser == user {
				c.redisClient.Del(ctx, key)
				c.redisClient.ZRem(ctx, "articdbm:cache:lru", key)
				invalidated++
				break
			}
		}
	}

	if invalidated > 0 {
		atomic.AddInt64(&c.stats.CurrentEntries, -int64(invalidated))
		c.logger.Info("Invalidated user cache entries",
			zap.String("user", user),
			zap.Int("count", invalidated))
	}

	return nil
}

// InvalidateDatabaseCache invalidates all cache entries for a database
func (c *SecureQueryCache) InvalidateDatabaseCache(ctx context.Context, database string) error {
	pattern := "articdbm:cache:query:*"
	iter := c.redisClient.Scan(ctx, 0, pattern, 100).Iterator()

	invalidated := 0
	for iter.Next(ctx) {
		key := iter.Val()
		entryData, err := c.redisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var entry CacheEntry
		if json.Unmarshal([]byte(entryData), &entry) != nil {
			continue
		}

		if entry.Database == database {
			c.redisClient.Del(ctx, key)
			c.redisClient.ZRem(ctx, "articdbm:cache:lru", key)
			atomic.AddInt64(&c.stats.CurrentMemory, -entry.Size)
			invalidated++
		}
	}

	if invalidated > 0 {
		atomic.AddInt64(&c.stats.CurrentEntries, -int64(invalidated))
		c.logger.Info("Invalidated database cache entries",
			zap.String("database", database),
			zap.Int("count", invalidated))
	}

	return nil
}

// GetStats returns current cache statistics
func (c *SecureQueryCache) GetStats() *CacheStats {
	return c.stats
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}