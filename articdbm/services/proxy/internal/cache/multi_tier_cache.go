package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"

	"github.com/penguintechinc/articdbm/proxy/internal/numa"
)

// Cache tiers
const (
	TierXDP     = "xdp"     // L1: Kernel-level XDP cache
	TierRedis   = "redis"   // L2: Redis cache
	TierBackend = "backend" // L3: Original backend database
)

type MultiTierCache struct {
	config      *MultiTierConfig
	xdpCache    *XDPCacheManager
	redisClient *redis.Client
	topology    *numa.TopologyInfo
	logger      *zap.Logger
	stats       *MultiTierStats
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
}

type MultiTierConfig struct {
	Enabled       bool                    `json:"enabled"`
	Tiers         []TierConfig            `json:"tiers"`
	DefaultTTL    time.Duration           `json:"default_ttl"`
	Policies      []CachePolicy          `json:"policies"`
	Invalidation  InvalidationConfig      `json:"invalidation"`
	Warming       WarmingConfig           `json:"warming"`
	Compression   CompressionConfig       `json:"compression"`
	Serialization SerializationConfig     `json:"serialization"`
	NUMA          NumaCacheConfig         `json:"numa"`
}

type TierConfig struct {
	Name         string        `json:"name"`
	Enabled      bool          `json:"enabled"`
	MaxSize      int64         `json:"max_size"`      // bytes
	MaxEntries   int           `json:"max_entries"`
	TTL          time.Duration `json:"ttl"`
	WriteThrough bool          `json:"write_through"` // Write to next tier immediately
	WriteBack    bool          `json:"write_back"`    // Write to next tier on eviction
	Priority     int           `json:"priority"`      // Higher = checked first
}

type CachePolicy struct {
	Name        string               `json:"name"`
	Rules       []PolicyRule         `json:"rules"`
	TierConfig  map[string]TierConfig `json:"tier_config"`
	Enabled     bool                 `json:"enabled"`
	Priority    int                  `json:"priority"`
}

type PolicyRule struct {
	Match     MatchCriteria `json:"match"`
	Action    string        `json:"action"`    // "cache", "bypass", "invalidate"
	TTL       time.Duration `json:"ttl"`
	Tier      string        `json:"tier"`      // Which tier to use
	Condition string        `json:"condition"` // Additional conditions
}

type MatchCriteria struct {
	Database    string   `json:"database"`
	Table       string   `json:"table"`
	Operation   string   `json:"operation"`
	UserPattern string   `json:"user_pattern"`
	QuerySize   []int    `json:"query_size"`    // [min, max] bytes
	ResultSize  []int    `json:"result_size"`   // [min, max] bytes
	Frequency   string   `json:"frequency"`     // "high", "medium", "low"
	TimeWindow  []string `json:"time_window"`   // ["09:00", "17:00"]
}

type InvalidationConfig struct {
	Enabled        bool          `json:"enabled"`
	OnWrite        bool          `json:"on_write"`
	OnSchema       bool          `json:"on_schema"`
	MaxPropagation time.Duration `json:"max_propagation"`
	BatchSize      int           `json:"batch_size"`
	Patterns       []string      `json:"patterns"`
}

type WarmingConfig struct {
	Enabled       bool          `json:"enabled"`
	Interval      time.Duration `json:"interval"`
	MaxQueries    int           `json:"max_queries"`
	Predictive    bool          `json:"predictive"`
	NumaAware     bool          `json:"numa_aware"`
	TimeWindows   []string      `json:"time_windows"`
}

type CompressionConfig struct {
	Enabled   bool   `json:"enabled"`
	Algorithm string `json:"algorithm"` // "gzip", "lz4", "zstd"
	Level     int    `json:"level"`
	MinSize   int    `json:"min_size"` // Minimum size to compress
}

type SerializationConfig struct {
	Format      string `json:"format"`       // "json", "msgpack", "protobuf"
	Compression bool   `json:"compression"`
}

type NumaCacheConfig struct {
	Enabled      bool `json:"enabled"`
	LocalityRatio float64 `json:"locality_ratio"` // 0.0-1.0, how much to prefer local NUMA
	Replication   bool `json:"replication"`       // Replicate hot data across NUMA nodes
}

type CacheRequest struct {
	Key       string                 `json:"key"`
	Query     string                 `json:"query"`
	Database  string                 `json:"database"`
	Table     string                 `json:"table"`
	Operation string                 `json:"operation"`
	User      string                 `json:"user"`
	Metadata  map[string]interface{} `json:"metadata"`
	TTL       time.Duration          `json:"ttl"`
	Context   context.Context        `json:"-"`
}

type CacheResponse struct {
	Data      []byte        `json:"data"`
	Found     bool          `json:"found"`
	Tier      string        `json:"tier"`
	TTL       time.Duration `json:"ttl"`
	HitRatio  float64       `json:"hit_ratio"`
	Duration  time.Duration `json:"duration"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type MultiTierStats struct {
	TotalRequests   uint64                  `json:"total_requests"`
	TotalHits       uint64                  `json:"total_hits"`
	TotalMisses     uint64                  `json:"total_misses"`
	TierStats       map[string]*TierStats   `json:"tier_stats"`
	PolicyStats     map[string]*PolicyStats `json:"policy_stats"`
	OverallHitRatio float64                 `json:"overall_hit_ratio"`
	AvgLatency      time.Duration           `json:"avg_latency"`
	LastUpdate      time.Time               `json:"last_update"`
}

type TierStats struct {
	Requests       uint64        `json:"requests"`
	Hits           uint64        `json:"hits"`
	Misses         uint64        `json:"misses"`
	Evictions      uint64        `json:"evictions"`
	Size           int64         `json:"size"`
	Entries        int           `json:"entries"`
	HitRatio       float64       `json:"hit_ratio"`
	AvgLatency     time.Duration `json:"avg_latency"`
	LastAccess     time.Time     `json:"last_access"`
}

type PolicyStats struct {
	Applied       uint64    `json:"applied"`
	CacheHits     uint64    `json:"cache_hits"`
	Bypassed      uint64    `json:"bypassed"`
	Invalidations uint64    `json:"invalidations"`
	LastApplied   time.Time `json:"last_applied"`
}

func NewMultiTierCache(config *MultiTierConfig, redisClient *redis.Client, topology *numa.TopologyInfo, logger *zap.Logger) (*MultiTierCache, error) {
	ctx, cancel := context.WithCancel(context.Background())

	mtc := &MultiTierCache{
		config:      config,
		redisClient: redisClient,
		topology:    topology,
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		stats: &MultiTierStats{
			TierStats:   make(map[string]*TierStats),
			PolicyStats: make(map[string]*PolicyStats),
		},
	}

	// Initialize tier statistics
	for _, tier := range config.Tiers {
		mtc.stats.TierStats[tier.Name] = &TierStats{}
	}

	for _, policy := range config.Policies {
		mtc.stats.PolicyStats[policy.Name] = &PolicyStats{}
	}

	// Initialize XDP cache if enabled
	if mtc.isTierEnabled(TierXDP) {
		xdpConfig := &XDPCacheConfig{
			Enabled:       true,
			DefaultTTL:    config.DefaultTTL,
			MaxResultSize: 4096,
			HashSeed:      0x12345678,
			NumaAware:     config.NUMA.Enabled,
		}

		xdpCache, err := NewXDPCacheManager(xdpConfig, topology, logger)
		if err != nil {
			logger.Warn("Failed to initialize XDP cache, continuing without it", zap.Error(err))
		} else {
			mtc.xdpCache = xdpCache
		}
	}

	// Start background tasks
	go mtc.statsCollector()
	go mtc.cacheWarmer()
	go mtc.invalidationProcessor()

	logger.Info("Multi-tier cache initialized",
		zap.Int("tiers", len(config.Tiers)),
		zap.Int("policies", len(config.Policies)),
		zap.Bool("numa_aware", config.NUMA.Enabled))

	return mtc, nil
}

func (mtc *MultiTierCache) Get(ctx context.Context, request *CacheRequest) (*CacheResponse, error) {
	if !mtc.config.Enabled {
		return &CacheResponse{Found: false}, nil
	}

	atomic.AddUint64(&mtc.stats.TotalRequests, 1)
	start := time.Now()

	// Find matching policy
	policy := mtc.findMatchingPolicy(request)
	if policy == nil || !policy.Enabled {
		return &CacheResponse{Found: false}, nil
	}

	// Apply policy rules
	action, tierName, ttl := mtc.applyPolicyRules(policy, request)
	if action == "bypass" {
		return &CacheResponse{Found: false}, nil
	}

	// Try each tier in priority order
	tiers := mtc.getSortedTiers()
	var response *CacheResponse

	for _, tier := range tiers {
		if !tier.Enabled || (tierName != "" && tier.Name != tierName) {
			continue
		}

		tierStats := mtc.stats.TierStats[tier.Name]
		atomic.AddUint64(&tierStats.Requests, 1)

		result, found, err := mtc.getFromTier(ctx, tier.Name, request)
		if err != nil {
			mtc.logger.Warn("Error getting from tier",
				zap.String("tier", tier.Name),
				zap.Error(err))
			continue
		}

		if found {
			atomic.AddUint64(&tierStats.Hits, 1)
			atomic.AddUint64(&mtc.stats.TotalHits, 1)

			response = &CacheResponse{
				Data:     result,
				Found:    true,
				Tier:     tier.Name,
				TTL:      ttl,
				Duration: time.Since(start),
			}

			// Promote to higher tiers if configured
			mtc.promoteToHigherTiers(ctx, request, result, tier.Name, ttl)
			break
		} else {
			atomic.AddUint64(&tierStats.Misses, 1)
		}
	}

	if response == nil {
		atomic.AddUint64(&mtc.stats.TotalMisses, 1)
		response = &CacheResponse{Found: false, Duration: time.Since(start)}
	}

	// Update policy stats
	if policyStats := mtc.stats.PolicyStats[policy.Name]; policyStats != nil {
		atomic.AddUint64(&policyStats.Applied, 1)
		if response.Found {
			atomic.AddUint64(&policyStats.CacheHits, 1)
		}
	}

	return response, nil
}

func (mtc *MultiTierCache) Set(ctx context.Context, request *CacheRequest, data []byte) error {
	if !mtc.config.Enabled {
		return nil
	}

	// Find matching policy
	policy := mtc.findMatchingPolicy(request)
	if policy == nil || !policy.Enabled {
		return nil
	}

	// Apply policy rules
	action, tierName, ttl := mtc.applyPolicyRules(policy, request)
	if action != "cache" {
		return nil
	}

	// Set in specified tier(s)
	tiers := mtc.getSortedTiers()
	for _, tier := range tiers {
		if !tier.Enabled || (tierName != "" && tier.Name != tierName) {
			continue
		}

		if err := mtc.setInTier(ctx, tier.Name, request, data, ttl); err != nil {
			mtc.logger.Warn("Error setting in tier",
				zap.String("tier", tier.Name),
				zap.Error(err))
		}
	}

	return nil
}

func (mtc *MultiTierCache) Invalidate(ctx context.Context, pattern string) error {
	if !mtc.config.Enabled || !mtc.config.Invalidation.Enabled {
		return nil
	}

	// Invalidate in all tiers
	for tierName := range mtc.stats.TierStats {
		if err := mtc.invalidateInTier(ctx, tierName, pattern); err != nil {
			mtc.logger.Warn("Error invalidating in tier",
				zap.String("tier", tierName),
				zap.String("pattern", pattern),
				zap.Error(err))
		}
	}

	return nil
}

func (mtc *MultiTierCache) getFromTier(ctx context.Context, tierName string, request *CacheRequest) ([]byte, bool, error) {
	switch tierName {
	case TierXDP:
		return mtc.getFromXDP(ctx, request)
	case TierRedis:
		return mtc.getFromRedis(ctx, request)
	default:
		return nil, false, fmt.Errorf("unknown tier: %s", tierName)
	}
}

func (mtc *MultiTierCache) setInTier(ctx context.Context, tierName string, request *CacheRequest, data []byte, ttl time.Duration) error {
	switch tierName {
	case TierXDP:
		return mtc.setInXDP(ctx, request, data, ttl)
	case TierRedis:
		return mtc.setInRedis(ctx, request, data, ttl)
	default:
		return fmt.Errorf("unknown tier: %s", tierName)
	}
}

func (mtc *MultiTierCache) invalidateInTier(ctx context.Context, tierName string, pattern string) error {
	switch tierName {
	case TierXDP:
		if mtc.xdpCache != nil {
			return mtc.xdpCache.InvalidatePattern(ctx, pattern)
		}
	case TierRedis:
		// Use Redis pattern deletion
		keys, err := mtc.redisClient.Keys(ctx, pattern).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			return mtc.redisClient.Del(ctx, keys...).Err()
		}
	}
	return nil
}

func (mtc *MultiTierCache) getFromXDP(ctx context.Context, request *CacheRequest) ([]byte, bool, error) {
	if mtc.xdpCache == nil {
		return nil, false, nil
	}

	key := mtc.generateCacheKey(request)
	return mtc.xdpCache.GetCachedResult(ctx, key)
}

func (mtc *MultiTierCache) setInXDP(ctx context.Context, request *CacheRequest, data []byte, ttl time.Duration) error {
	if mtc.xdpCache == nil {
		return fmt.Errorf("XDP cache not available")
	}

	key := mtc.generateCacheKey(request)
	return mtc.xdpCache.CacheQuery(ctx, key, data, ttl)
}

func (mtc *MultiTierCache) getFromRedis(ctx context.Context, request *CacheRequest) ([]byte, bool, error) {
	key := mtc.generateCacheKey(request)
	result, err := mtc.redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	return []byte(result), true, nil
}

func (mtc *MultiTierCache) setInRedis(ctx context.Context, request *CacheRequest, data []byte, ttl time.Duration) error {
	key := mtc.generateCacheKey(request)
	return mtc.redisClient.Set(ctx, key, data, ttl).Err()
}

func (mtc *MultiTierCache) generateCacheKey(request *CacheRequest) string {
	// Generate consistent cache key
	hasher := sha256.New()
	hasher.Write([]byte(request.Database))
	hasher.Write([]byte(request.Table))
	hasher.Write([]byte(request.Query))
	hasher.Write([]byte(request.User))
	hash := hasher.Sum(nil)

	return fmt.Sprintf("mtc:%x", hash[:16])
}

func (mtc *MultiTierCache) findMatchingPolicy(request *CacheRequest) *CachePolicy {
	// Sort policies by priority
	for _, policy := range mtc.config.Policies {
		if policy.Enabled && mtc.policyMatches(policy, request) {
			return &policy
		}
	}
	return nil
}

func (mtc *MultiTierCache) policyMatches(policy CachePolicy, request *CacheRequest) bool {
	for _, rule := range policy.Rules {
		if mtc.ruleMatches(rule, request) {
			return true
		}
	}
	return false
}

func (mtc *MultiTierCache) ruleMatches(rule PolicyRule, request *CacheRequest) bool {
	match := rule.Match

	if match.Database != "" && match.Database != "*" && match.Database != request.Database {
		return false
	}

	if match.Table != "" && match.Table != "*" && match.Table != request.Table {
		return false
	}

	if match.Operation != "" && match.Operation != "*" && match.Operation != request.Operation {
		return false
	}

	if match.UserPattern != "" && match.UserPattern != "*" && match.UserPattern != request.User {
		return false
	}

	return true
}

func (mtc *MultiTierCache) applyPolicyRules(policy *CachePolicy, request *CacheRequest) (string, string, time.Duration) {
	for _, rule := range policy.Rules {
		if mtc.ruleMatches(rule, request) {
			ttl := rule.TTL
			if ttl == 0 {
				ttl = mtc.config.DefaultTTL
			}
			return rule.Action, rule.Tier, ttl
		}
	}

	return "cache", "", mtc.config.DefaultTTL
}

func (mtc *MultiTierCache) getSortedTiers() []TierConfig {
	tiers := make([]TierConfig, 0, len(mtc.config.Tiers))
	copy(tiers, mtc.config.Tiers)

	// Sort by priority (higher first)
	for i := 0; i < len(tiers)-1; i++ {
		for j := i + 1; j < len(tiers); j++ {
			if tiers[i].Priority < tiers[j].Priority {
				tiers[i], tiers[j] = tiers[j], tiers[i]
			}
		}
	}

	return tiers
}

func (mtc *MultiTierCache) isTierEnabled(tierName string) bool {
	for _, tier := range mtc.config.Tiers {
		if tier.Name == tierName {
			return tier.Enabled
		}
	}
	return false
}

func (mtc *MultiTierCache) promoteToHigherTiers(ctx context.Context, request *CacheRequest, data []byte, currentTier string, ttl time.Duration) {
	// Promote data to higher priority tiers
	tiers := mtc.getSortedTiers()
	promote := true

	for _, tier := range tiers {
		if tier.Name == currentTier {
			promote = false
			continue
		}
		if !promote || !tier.Enabled {
			continue
		}

		if err := mtc.setInTier(ctx, tier.Name, request, data, ttl); err != nil {
			mtc.logger.Debug("Failed to promote to tier",
				zap.String("tier", tier.Name),
				zap.Error(err))
		}
	}
}

func (mtc *MultiTierCache) statsCollector() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-mtc.ctx.Done():
			return
		case <-ticker.C:
			mtc.updateStats()
		}
	}
}

func (mtc *MultiTierCache) updateStats() {
	// Calculate overall hit ratio
	total := mtc.stats.TotalHits + mtc.stats.TotalMisses
	if total > 0 {
		mtc.stats.OverallHitRatio = float64(mtc.stats.TotalHits) / float64(total)
	}

	// Update tier hit ratios
	for _, tierStats := range mtc.stats.TierStats {
		tierTotal := tierStats.Hits + tierStats.Misses
		if tierTotal > 0 {
			tierStats.HitRatio = float64(tierStats.Hits) / float64(tierTotal)
		}
	}

	mtc.stats.LastUpdate = time.Now()
}

func (mtc *MultiTierCache) cacheWarmer() {
	if !mtc.config.Warming.Enabled {
		return
	}

	ticker := time.NewTicker(mtc.config.Warming.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-mtc.ctx.Done():
			return
		case <-ticker.C:
			mtc.performCacheWarming()
		}
	}
}

func (mtc *MultiTierCache) performCacheWarming() {
	mtc.logger.Debug("Performing multi-tier cache warming")

	// This would implement intelligent cache warming:
	// 1. Analyze query patterns
	// 2. Pre-load frequently accessed data
	// 3. NUMA-aware warming if enabled
	// 4. Time-based warming for scheduled workloads

	mtc.logger.Debug("Multi-tier cache warming completed")
}

func (mtc *MultiTierCache) invalidationProcessor() {
	if !mtc.config.Invalidation.Enabled {
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-mtc.ctx.Done():
			return
		case <-ticker.C:
			// Process any pending invalidations
		}
	}
}

func (mtc *MultiTierCache) GetStats() *MultiTierStats {
	return mtc.stats
}

func (mtc *MultiTierCache) Close() error {
	mtc.cancel()

	if mtc.xdpCache != nil {
		mtc.xdpCache.Close()
	}

	mtc.logger.Info("Multi-tier cache stopped")
	return nil
}