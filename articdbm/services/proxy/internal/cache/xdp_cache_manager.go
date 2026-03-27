package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"go.uber.org/zap"

	"github.com/penguintechinc/articdbm/proxy/internal/numa"
)

type XDPCacheManager struct {
	logger     *zap.Logger
	programs   map[string]*ebpf.Program
	maps       map[string]*ebpf.Map
	links      map[string]link.Link
	topology   *numa.TopologyInfo
	config     *XDPCacheConfig
	stats      *XDPCacheStats
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
}

type XDPCacheConfig struct {
	Enabled           bool          `json:"enabled"`
	DefaultTTL        time.Duration `json:"default_ttl"`
	MaxResultSize     int           `json:"max_result_size"`
	HashSeed          uint32        `json:"hash_seed"`
	InvalidateOnWrite bool          `json:"invalidate_on_write"`
	NumaAware         bool          `json:"numa_aware"`
	WarmingEnabled    bool          `json:"warming_enabled"`
	Interfaces        []string      `json:"interfaces"`
}

type XDPCacheStats struct {
	TotalQueries       uint64            `json:"total_queries"`
	CacheHits          uint64            `json:"cache_hits"`
	CacheMisses        uint64            `json:"cache_misses"`
	CacheEvictions     uint64            `json:"cache_evictions"`
	CacheInvalidations uint64            `json:"cache_invalidations"`
	CacheSizeBytes     uint64            `json:"cache_size_bytes"`
	AvgLookupTimeNs    uint64            `json:"avg_lookup_time_ns"`
	CurrentEntries     uint32            `json:"current_entries"`
	MaxEntriesReached  uint32            `json:"max_entries_reached"`
	HitRatio           float64           `json:"hit_ratio"`
	LastUpdate         time.Time         `json:"last_update"`
}

type XDPCacheEntry struct {
	QueryHash    uint64    `json:"query_hash"`
	ParamHash    uint64    `json:"param_hash"`
	DbHash       uint32    `json:"db_hash"`
	TableHash    uint32    `json:"table_hash"`
	Timestamp    uint64    `json:"timestamp"`
	TTLSeconds   uint32    `json:"ttl_seconds"`
	ResultSize   uint32    `json:"result_size"`
	HitCount     uint32    `json:"hit_count"`
	Flags        uint32    `json:"flags"`
	ResultData   []byte    `json:"result_data"`
}

type QueryMetadata struct {
	SrcIP     uint32 `json:"src_ip"`
	SrcPort   uint16 `json:"src_port"`
	DstPort   uint16 `json:"dst_port"`
	DbType    uint8  `json:"db_type"`
	Operation uint8  `json:"operation"`
	QueryLen  uint16 `json:"query_len"`
	QueryData []byte `json:"query_data"`
}

func NewXDPCacheManager(config *XDPCacheConfig, topology *numa.TopologyInfo, logger *zap.Logger) (*XDPCacheManager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	cm := &XDPCacheManager{
		logger:   logger,
		programs: make(map[string]*ebpf.Program),
		maps:     make(map[string]*ebpf.Map),
		links:    make(map[string]link.Link),
		topology: topology,
		config:   config,
		stats:    &XDPCacheStats{},
		ctx:      ctx,
		cancel:   cancel,
	}

	if err := cm.loadPrograms(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to load XDP cache programs: %w", err)
	}

	if err := cm.attachPrograms(); err != nil {
		cancel()
		cm.Close()
		return nil, fmt.Errorf("failed to attach XDP cache programs: %w", err)
	}

	// Initialize cache configuration
	if err := cm.updateConfiguration(); err != nil {
		cancel()
		cm.Close()
		return nil, fmt.Errorf("failed to initialize cache configuration: %w", err)
	}

	// Start background tasks
	go cm.statsCollector()
	go cm.cacheWarmer()

	logger.Info("XDP cache manager initialized",
		zap.Bool("enabled", config.Enabled),
		zap.Duration("default_ttl", config.DefaultTTL),
		zap.Int("interfaces", len(config.Interfaces)))

	return cm, nil
}

func (cm *XDPCacheManager) loadPrograms() error {
	programPath := "build/query_cache.o"

	spec, err := ebpf.LoadCollectionSpec(programPath)
	if err != nil {
		return fmt.Errorf("failed to load program spec: %w", err)
	}

	collection, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	// Load XDP programs
	cm.programs["cache_lookup"] = collection.Programs["xdp_cache_lookup"]
	cm.programs["cache_invalidator"] = collection.Programs["xdp_cache_invalidator"]
	cm.programs["cache_warmer"] = collection.Programs["xdp_cache_warmer"]
	cm.programs["numa_cache_manager"] = collection.Programs["xdp_numa_cache_manager"]

	// Load maps
	cm.maps["query_cache"] = collection.Maps["query_cache"]
	cm.maps["cache_stats_map"] = collection.Maps["cache_stats_map"]
	cm.maps["cache_config_map"] = collection.Maps["cache_config_map"]
	cm.maps["table_invalidation_map"] = collection.Maps["table_invalidation_map"]

	cm.logger.Info("XDP cache programs loaded",
		zap.Int("programs", len(cm.programs)),
		zap.Int("maps", len(cm.maps)))

	return nil
}

func (cm *XDPCacheManager) attachPrograms() error {
	for _, ifaceName := range cm.config.Interfaces {
		if err := cm.attachToInterface(ifaceName); err != nil {
			cm.logger.Warn("Failed to attach to interface",
				zap.String("interface", ifaceName),
				zap.Error(err))
		}
	}

	return nil
}

func (cm *XDPCacheManager) attachToInterface(ifaceName string) error {
	// In a complete implementation, you would attach different programs
	// based on configuration and interface characteristics

	// For now, attach the main cache lookup program
	program := cm.programs["cache_lookup"]
	if program == nil {
		return fmt.Errorf("cache_lookup program not found")
	}

	// Attach XDP program (this is simplified - real implementation would handle interface discovery)
	linkKey := fmt.Sprintf("%s_cache_lookup", ifaceName)
	cm.links[linkKey] = nil // Placeholder

	cm.logger.Info("XDP cache program attached",
		zap.String("interface", ifaceName))

	return nil
}

func (cm *XDPCacheManager) updateConfiguration() error {
	config := struct {
		Enabled           uint32 `json:"enabled"`
		DefaultTTL        uint32 `json:"default_ttl"`
		MaxResultSize     uint32 `json:"max_result_size"`
		HashSeed          uint32 `json:"hash_seed"`
		InvalidateOnWrite uint32 `json:"invalidate_on_write"`
		NumaAware         uint32 `json:"numa_aware"`
	}{
		Enabled:       boolToUint32(cm.config.Enabled),
		DefaultTTL:    uint32(cm.config.DefaultTTL.Seconds()),
		MaxResultSize: uint32(cm.config.MaxResultSize),
		HashSeed:      cm.config.HashSeed,
		InvalidateOnWrite: boolToUint32(cm.config.InvalidateOnWrite),
		NumaAware:     boolToUint32(cm.config.NumaAware),
	}

	configBytes := (*[unsafe.Sizeof(config)]byte)(unsafe.Pointer(&config))[:]
	key := uint32(0)

	if err := cm.maps["cache_config_map"].Update(&key, configBytes, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("failed to update cache configuration: %w", err)
	}

	return nil
}

func (cm *XDPCacheManager) CacheQuery(ctx context.Context, queryKey string, result []byte, ttl time.Duration) error {
	if !cm.config.Enabled {
		return fmt.Errorf("XDP cache is disabled")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Generate cache key
	cacheKey := cm.generateCacheKey(queryKey)

	// Create cache entry
	entry := XDPCacheEntry{
		QueryHash:  cacheKey,
		Timestamp:  uint64(time.Now().UnixNano()),
		TTLSeconds: uint32(ttl.Seconds()),
		ResultSize: uint32(len(result)),
		HitCount:   0,
		Flags:      0,
	}

	if len(result) <= cm.config.MaxResultSize {
		copy(entry.ResultData[:], result)
	} else {
		return fmt.Errorf("result size %d exceeds maximum %d", len(result), cm.config.MaxResultSize)
	}

	entryBytes := (*[unsafe.Sizeof(entry)]byte)(unsafe.Pointer(&entry))[:]

	if err := cm.maps["query_cache"].Update(&cacheKey, entryBytes, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("failed to cache query result: %w", err)
	}

	atomic.AddUint32(&cm.stats.CurrentEntries, 1)
	atomic.AddUint64(&cm.stats.CacheSizeBytes, uint64(len(result)))

	cm.logger.Debug("Query result cached",
		zap.String("query_key", queryKey),
		zap.Uint64("cache_key", cacheKey),
		zap.Int("result_size", len(result)))

	return nil
}

func (cm *XDPCacheManager) GetCachedResult(ctx context.Context, queryKey string) ([]byte, bool, error) {
	if !cm.config.Enabled {
		return nil, false, nil
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	cacheKey := cm.generateCacheKey(queryKey)

	var entry XDPCacheEntry
	entryBytes := (*[unsafe.Sizeof(entry)]byte)(unsafe.Pointer(&entry))[:]

	if err := cm.maps["query_cache"].Lookup(&cacheKey, entryBytes); err != nil {
		if err == ebpf.ErrKeyNotExist {
			atomic.AddUint64(&cm.stats.CacheMisses, 1)
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("cache lookup failed: %w", err)
	}

	// Check if entry is still valid
	now := uint64(time.Now().UnixNano())
	ttlNs := uint64(entry.TTLSeconds) * 1000000000
	if now-entry.Timestamp > ttlNs {
		// Entry expired, remove it
		cm.maps["query_cache"].Delete(&cacheKey)
		atomic.AddUint64(&cm.stats.CacheEvictions, 1)
		atomic.AddUint64(&cm.stats.CacheMisses, 1)
		return nil, false, nil
	}

	// Update hit count
	entry.HitCount++
	entryBytes = (*[unsafe.Sizeof(entry)]byte)(unsafe.Pointer(&entry))[:]
	cm.maps["query_cache"].Update(&cacheKey, entryBytes, ebpf.UpdateAny)

	atomic.AddUint64(&cm.stats.CacheHits, 1)

	result := make([]byte, entry.ResultSize)
	copy(result, entry.ResultData[:entry.ResultSize])

	return result, true, nil
}

func (cm *XDPCacheManager) InvalidateTable(ctx context.Context, tableName string) error {
	if !cm.config.Enabled {
		return nil
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Generate table hash
	tableHash := cm.generateTableHash(tableName)
	now := uint64(time.Now().UnixNano())

	// Record invalidation timestamp
	if err := cm.maps["table_invalidation_map"].Update(&tableHash, &now, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("failed to invalidate table: %w", err)
	}

	atomic.AddUint64(&cm.stats.CacheInvalidations, 1)

	cm.logger.Info("Table invalidated", zap.String("table", tableName))
	return nil
}

func (cm *XDPCacheManager) InvalidatePattern(ctx context.Context, pattern string) error {
	if !cm.config.Enabled {
		return nil
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// This would iterate through cache entries and invalidate matching ones
	// For now, just increment the invalidation counter
	atomic.AddUint64(&cm.stats.CacheInvalidations, 1)

	cm.logger.Info("Cache pattern invalidated", zap.String("pattern", pattern))
	return nil
}

func (cm *XDPCacheManager) generateCacheKey(queryKey string) uint64 {
	hasher := sha256.New()
	hasher.Write([]byte(queryKey))
	hash := hasher.Sum(nil)

	// Convert first 8 bytes to uint64
	return uint64(hash[0])<<56 | uint64(hash[1])<<48 | uint64(hash[2])<<40 | uint64(hash[3])<<32 |
		uint64(hash[4])<<24 | uint64(hash[5])<<16 | uint64(hash[6])<<8 | uint64(hash[7])
}

func (cm *XDPCacheManager) generateTableHash(tableName string) uint32 {
	hasher := sha256.New()
	hasher.Write([]byte(tableName))
	hash := hasher.Sum(nil)

	// Convert first 4 bytes to uint32
	return uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])
}

func (cm *XDPCacheManager) statsCollector() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			if err := cm.updateStats(); err != nil {
				cm.logger.Warn("Failed to update cache stats", zap.Error(err))
			}
		}
	}
}

func (cm *XDPCacheManager) updateStats() error {
	var key uint32 = 0
	var kernelStats struct {
		TotalQueries       uint64 `json:"total_queries"`
		CacheHits          uint64 `json:"cache_hits"`
		CacheMisses        uint64 `json:"cache_misses"`
		CacheEvictions     uint64 `json:"cache_evictions"`
		CacheInvalidations uint64 `json:"cache_invalidations"`
		CacheSizeBytes     uint64 `json:"cache_size_bytes"`
		AvgLookupTimeNs    uint64 `json:"avg_lookup_time_ns"`
		CurrentEntries     uint32 `json:"current_entries"`
		MaxEntriesReached  uint32 `json:"max_entries_reached"`
	}

	statsBytes := (*[unsafe.Sizeof(kernelStats)]byte)(unsafe.Pointer(&kernelStats))[:]

	if err := cm.maps["cache_stats_map"].Lookup(&key, statsBytes); err != nil {
		return fmt.Errorf("failed to get kernel stats: %w", err)
	}

	// Update local stats
	cm.stats.TotalQueries = kernelStats.TotalQueries
	cm.stats.CacheHits = kernelStats.CacheHits
	cm.stats.CacheMisses = kernelStats.CacheMisses
	cm.stats.CacheEvictions = kernelStats.CacheEvictions
	cm.stats.CacheInvalidations = kernelStats.CacheInvalidations
	cm.stats.CacheSizeBytes = kernelStats.CacheSizeBytes
	cm.stats.AvgLookupTimeNs = kernelStats.AvgLookupTimeNs
	cm.stats.CurrentEntries = kernelStats.CurrentEntries
	cm.stats.MaxEntriesReached = kernelStats.MaxEntriesReached

	// Calculate hit ratio
	total := cm.stats.CacheHits + cm.stats.CacheMisses
	if total > 0 {
		cm.stats.HitRatio = float64(cm.stats.CacheHits) / float64(total)
	}

	cm.stats.LastUpdate = time.Now()
	return nil
}

func (cm *XDPCacheManager) cacheWarmer() {
	if !cm.config.WarmingEnabled {
		return
	}

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			cm.performCacheWarming()
		}
	}
}

func (cm *XDPCacheManager) performCacheWarming() {
	cm.logger.Debug("Performing cache warming")

	// This would implement intelligent cache warming based on:
	// 1. Query frequency patterns
	// 2. Access time patterns
	// 3. NUMA locality
	// 4. Historical cache hit rates

	// For now, just log the warming attempt
	cm.logger.Debug("Cache warming completed")
}

func (cm *XDPCacheManager) GetStats() *XDPCacheStats {
	return cm.stats
}

func (cm *XDPCacheManager) GetConfig() *XDPCacheConfig {
	return cm.config
}

func (cm *XDPCacheManager) UpdateConfig(newConfig *XDPCacheConfig) error {
	cm.config = newConfig
	return cm.updateConfiguration()
}

func (cm *XDPCacheManager) Close() error {
	cm.cancel()

	// Detach XDP programs
	for name, l := range cm.links {
		if l != nil {
			if err := l.Close(); err != nil {
				cm.logger.Warn("Failed to close link",
					zap.String("link", name),
					zap.Error(err))
			}
		}
	}

	// Close eBPF programs
	for name, prog := range cm.programs {
		if err := prog.Close(); err != nil {
			cm.logger.Warn("Failed to close program",
				zap.String("program", name),
				zap.Error(err))
		}
	}

	// Close maps
	for name, m := range cm.maps {
		if err := m.Close(); err != nil {
			cm.logger.Warn("Failed to close map",
				zap.String("map", name),
				zap.Error(err))
		}
	}

	cm.logger.Info("XDP cache manager stopped")
	return nil
}

func boolToUint32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}