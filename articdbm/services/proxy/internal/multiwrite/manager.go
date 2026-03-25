package multiwrite

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/penguintechinc/articdbm/proxy/internal/numa"
	"go.uber.org/zap"
)

// Multi-write strategies
const (
	StrategySync       = "sync"        // All writes must succeed
	StrategyAsync      = "async"       // Fire-and-forget to secondaries
	StrategyBestEffort = "best_effort" // Primary must succeed, secondaries best effort
	StrategyQuorum     = "quorum"      // Majority of clusters must succeed
)

// Write result status
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusTimeout = "timeout"
	StatusSkipped = "skipped"
)

type Manager struct {
	config      *MultiWriteConfig
	clusters    map[string]*ClusterGroup
	topology    *numa.TopologyInfo
	logger      *zap.Logger
	stats       *Stats
	ctx         context.Context
	cancel      context.CancelFunc
	workerPool  *WorkerPool
}

type MultiWriteConfig struct {
	Enabled         bool                     `json:"enabled"`
	DefaultStrategy string                   `json:"default_strategy"`
	Timeout         time.Duration            `json:"timeout"`
	MaxRetries      int                      `json:"max_retries"`
	RetryBackoff    time.Duration            `json:"retry_backoff"`
	Groups          map[string]*ClusterGroup `json:"groups"`
	Rules           []*WriteRule             `json:"rules"`
}

type ClusterGroup struct {
	Name        string             `json:"name"`
	Strategy    string             `json:"strategy"`
	Primary     *ClusterConfig     `json:"primary"`
	Secondaries []*ClusterConfig   `json:"secondaries"`
	Enabled     bool               `json:"enabled"`
	Weight      int                `json:"weight"`
	MaxLag      time.Duration      `json:"max_lag"`
	HealthCheck *HealthCheckConfig `json:"health_check"`
}

type ClusterConfig struct {
	Name         string        `json:"name"`
	Type         string        `json:"type"` // mysql, postgresql, mongodb, redis
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	Database     string        `json:"database"`
	Username     string        `json:"username"`
	Password     string        `json:"password"`
	MaxConns     int           `json:"max_conns"`
	Timeout      time.Duration `json:"timeout"`
	Priority     int           `json:"priority"`
	Enabled      bool          `json:"enabled"`
	TLS          bool          `json:"tls"`
	NumaNode     int           `json:"numa_node"`
}

type WriteRule struct {
	Name        string   `json:"name"`
	Match       *Match   `json:"match"`
	GroupName   string   `json:"group_name"`
	Strategy    string   `json:"strategy"`
	Enabled     bool     `json:"enabled"`
	Priority    int      `json:"priority"`
	Conditions  []string `json:"conditions"`
}

type Match struct {
	Database    string   `json:"database"`
	Table       string   `json:"table"`
	Operation   string   `json:"operation"` // INSERT, UPDATE, DELETE, etc.
	UserPattern string   `json:"user_pattern"`
	QueryMatch  string   `json:"query_match"`
	TimeWindow  []string `json:"time_window"` // ["09:00", "17:00"]
}

type HealthCheckConfig struct {
	Enabled  bool          `json:"enabled"`
	Interval time.Duration `json:"interval"`
	Timeout  time.Duration `json:"timeout"`
	Query    string        `json:"query"`
}

type WriteRequest struct {
	ID          string                 `json:"id"`
	Query       string                 `json:"query"`
	Args        []interface{}          `json:"args"`
	Database    string                 `json:"database"`
	Table       string                 `json:"table"`
	Operation   string                 `json:"operation"`
	Username    string                 `json:"username"`
	GroupName   string                 `json:"group_name"`
	Strategy    string                 `json:"strategy"`
	Timeout     time.Duration          `json:"timeout"`
	Metadata    map[string]interface{} `json:"metadata"`
	StartTime   time.Time              `json:"start_time"`
}

type WriteResult struct {
	ID          string                 `json:"id"`
	Success     bool                   `json:"success"`
	Results     map[string]*ClusterResult `json:"results"`
	Duration    time.Duration          `json:"duration"`
	Strategy    string                 `json:"strategy"`
	Error       error                  `json:"error"`
	Timestamp   time.Time              `json:"timestamp"`
}

type ClusterResult struct {
	ClusterName   string        `json:"cluster_name"`
	Status        string        `json:"status"`
	Duration      time.Duration `json:"duration"`
	RowsAffected  int64         `json:"rows_affected"`
	Error         error         `json:"error"`
	Retries       int           `json:"retries"`
	IsPrimary     bool          `json:"is_primary"`
}

type Stats struct {
	TotalRequests     uint64            `json:"total_requests"`
	SuccessfulWrites  uint64            `json:"successful_writes"`
	FailedWrites      uint64            `json:"failed_writes"`
	TimeoutWrites     uint64            `json:"timeout_writes"`
	StrategyStats     map[string]uint64 `json:"strategy_stats"`
	ClusterStats      map[string]*ClusterStats `json:"cluster_stats"`
	AvgDuration       time.Duration     `json:"avg_duration"`
	LastUpdate        time.Time         `json:"last_update"`
}

type ClusterStats struct {
	Writes       uint64        `json:"writes"`
	Successes    uint64        `json:"successes"`
	Failures     uint64        `json:"failures"`
	Timeouts     uint64        `json:"timeouts"`
	AvgDuration  time.Duration `json:"avg_duration"`
	LastWrite    time.Time     `json:"last_write"`
	Health       string        `json:"health"`
}

func NewManager(config *MultiWriteConfig, topology *numa.TopologyInfo, logger *zap.Logger) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		config:   config,
		clusters: make(map[string]*ClusterGroup),
		topology: topology,
		logger:   logger,
		stats: &Stats{
			StrategyStats: make(map[string]uint64),
			ClusterStats:  make(map[string]*ClusterStats),
		},
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize cluster groups
	for name, group := range config.Groups {
		m.clusters[name] = group
		m.stats.ClusterStats[name] = &ClusterStats{}
	}

	// Create worker pool
	poolSize := 10
	if topology != nil {
		poolSize = len(topology.CPUCores)
	}

	workerPool, err := NewWorkerPool(poolSize, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create worker pool: %w", err)
	}
	m.workerPool = workerPool

	// Start background tasks
	go m.healthMonitor()
	go m.statsCollector()

	logger.Info("Multi-write manager initialized",
		zap.Int("groups", len(m.clusters)),
		zap.Int("workers", poolSize))

	return m, nil
}

func (m *Manager) ExecuteWrite(ctx context.Context, request *WriteRequest) (*WriteResult, error) {
	if !m.config.Enabled {
		return nil, fmt.Errorf("multi-write is disabled")
	}

	atomic.AddUint64(&m.stats.TotalRequests, 1)
	request.StartTime = time.Now()

	// Generate unique ID if not provided
	if request.ID == "" {
		request.ID = fmt.Sprintf("mw_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&m.stats.TotalRequests, 1))
	}

	// Determine which rule applies
	rule := m.matchRule(request)
	if rule == nil {
		return nil, fmt.Errorf("no matching rule found for request")
	}

	request.GroupName = rule.GroupName
	request.Strategy = rule.Strategy
	if request.Strategy == "" {
		request.Strategy = m.config.DefaultStrategy
	}

	// Get cluster group
	group, exists := m.clusters[request.GroupName]
	if !exists || !group.Enabled {
		return nil, fmt.Errorf("cluster group %s not found or disabled", request.GroupName)
	}

	// Execute based on strategy
	result := &WriteResult{
		ID:        request.ID,
		Results:   make(map[string]*ClusterResult),
		Strategy:  request.Strategy,
		Timestamp: time.Now(),
	}

	var err error
	switch request.Strategy {
	case StrategySync:
		err = m.executeSyncWrite(ctx, request, group, result)
	case StrategyAsync:
		err = m.executeAsyncWrite(ctx, request, group, result)
	case StrategyBestEffort:
		err = m.executeBestEffortWrite(ctx, request, group, result)
	case StrategyQuorum:
		err = m.executeQuorumWrite(ctx, request, group, result)
	default:
		err = fmt.Errorf("unknown strategy: %s", request.Strategy)
	}

	result.Duration = time.Since(request.StartTime)
	result.Error = err
	result.Success = err == nil

	// Update statistics
	m.updateStats(result)

	m.logger.Info("Multi-write executed",
		zap.String("id", request.ID),
		zap.String("strategy", request.Strategy),
		zap.Bool("success", result.Success),
		zap.Duration("duration", result.Duration))

	return result, err
}

func (m *Manager) executeSyncWrite(ctx context.Context, request *WriteRequest, group *ClusterGroup, result *WriteResult) error {
	// All writes must succeed for sync strategy
	clusters := []*ClusterConfig{group.Primary}
	clusters = append(clusters, group.Secondaries...)

	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	for _, cluster := range clusters {
		if !cluster.Enabled {
			continue
		}

		wg.Add(1)
		go func(c *ClusterConfig) {
			defer wg.Done()

			clusterResult := m.executeOnCluster(ctx, request, c, c == group.Primary)

			mu.Lock()
			result.Results[c.Name] = clusterResult
			if clusterResult.Error != nil {
				errors = append(errors, clusterResult.Error)
			}
			mu.Unlock()
		}(cluster)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("sync write failed: %d clusters failed", len(errors))
	}

	return nil
}

func (m *Manager) executeAsyncWrite(ctx context.Context, request *WriteRequest, group *ClusterGroup, result *WriteResult) error {
	// Primary must succeed, secondaries are fire-and-forget
	primaryResult := m.executeOnCluster(ctx, request, group.Primary, true)
	result.Results[group.Primary.Name] = primaryResult

	if primaryResult.Error != nil {
		return fmt.Errorf("async write failed on primary: %w", primaryResult.Error)
	}

	// Fire secondaries asynchronously
	for _, secondary := range group.Secondaries {
		if !secondary.Enabled {
			continue
		}

		go func(c *ClusterConfig) {
			clusterResult := m.executeOnCluster(ctx, request, c, false)
			result.Results[c.Name] = clusterResult
		}(secondary)
	}

	return nil
}

func (m *Manager) executeBestEffortWrite(ctx context.Context, request *WriteRequest, group *ClusterGroup, result *WriteResult) error {
	// Primary must succeed, secondaries best effort
	primaryResult := m.executeOnCluster(ctx, request, group.Primary, true)
	result.Results[group.Primary.Name] = primaryResult

	if primaryResult.Error != nil {
		return fmt.Errorf("best effort write failed on primary: %w", primaryResult.Error)
	}

	// Try secondaries with best effort
	var wg sync.WaitGroup
	for _, secondary := range group.Secondaries {
		if !secondary.Enabled {
			continue
		}

		wg.Add(1)
		go func(c *ClusterConfig) {
			defer wg.Done()
			clusterResult := m.executeOnCluster(ctx, request, c, false)
			result.Results[c.Name] = clusterResult
		}(secondary)
	}

	wg.Wait()
	return nil
}

func (m *Manager) executeQuorumWrite(ctx context.Context, request *WriteRequest, group *ClusterGroup, result *WriteResult) error {
	// Majority of clusters must succeed
	clusters := []*ClusterConfig{group.Primary}
	clusters = append(clusters, group.Secondaries...)

	enabledClusters := make([]*ClusterConfig, 0)
	for _, cluster := range clusters {
		if cluster.Enabled {
			enabledClusters = append(enabledClusters, cluster)
		}
	}

	requiredSuccesses := (len(enabledClusters) / 2) + 1

	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0

	for _, cluster := range enabledClusters {
		wg.Add(1)
		go func(c *ClusterConfig) {
			defer wg.Done()

			clusterResult := m.executeOnCluster(ctx, request, c, c == group.Primary)

			mu.Lock()
			result.Results[c.Name] = clusterResult
			if clusterResult.Error == nil {
				successes++
			}
			mu.Unlock()
		}(cluster)
	}

	wg.Wait()

	if successes < requiredSuccesses {
		return fmt.Errorf("quorum write failed: only %d/%d clusters succeeded, need %d",
			successes, len(enabledClusters), requiredSuccesses)
	}

	return nil
}

func (m *Manager) executeOnCluster(ctx context.Context, request *WriteRequest, cluster *ClusterConfig, isPrimary bool) *ClusterResult {
	result := &ClusterResult{
		ClusterName: cluster.Name,
		IsPrimary:   isPrimary,
		Status:      StatusFailed,
	}

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	// Apply timeout
	timeout := request.Timeout
	if timeout == 0 {
		timeout = cluster.Timeout
	}
	if timeout == 0 {
		timeout = m.config.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute with retries
	var lastError error
	for attempt := 0; attempt <= m.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				result.Status = StatusTimeout
				result.Error = ctx.Err()
				return result
			case <-time.After(m.config.RetryBackoff):
			}
			result.Retries++
		}

		rowsAffected, err := m.executeQuery(ctx, cluster, request.Query, request.Args)
		if err == nil {
			result.Status = StatusSuccess
			result.RowsAffected = rowsAffected
			return result
		}

		lastError = err
		m.logger.Warn("Write attempt failed",
			zap.String("cluster", cluster.Name),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
	}

	result.Error = lastError
	return result
}

func (m *Manager) executeQuery(ctx context.Context, cluster *ClusterConfig, query string, args []interface{}) (int64, error) {
	// This is a simplified version - in reality, you'd have different implementations
	// for different database types (MySQL, PostgreSQL, etc.)

	// For now, return success with mock data
	// In real implementation, this would:
	// 1. Get connection from pool
	// 2. Execute the query
	// 3. Return rows affected

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(10 * time.Millisecond): // Simulate query execution
		return 1, nil
	}
}

func (m *Manager) matchRule(request *WriteRequest) *WriteRule {
	// Sort rules by priority (highest first)
	sortedRules := make([]*WriteRule, 0, len(m.config.Rules))
	for _, rule := range m.config.Rules {
		if rule.Enabled {
			sortedRules = append(sortedRules, rule)
		}
	}

	// Simple sorting by priority (in real implementation, use sort.Slice)
	for _, rule := range sortedRules {
		if m.ruleMatches(rule, request) {
			return rule
		}
	}

	return nil
}

func (m *Manager) ruleMatches(rule *WriteRule, request *WriteRequest) bool {
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

	if match.UserPattern != "" && match.UserPattern != "*" {
		// Simple pattern matching - in reality, use regexp
		if match.UserPattern != request.Username {
			return false
		}
	}

	return true
}

func (m *Manager) updateStats(result *WriteResult) {
	val := m.stats.StrategyStats[result.Strategy]
	val++
	m.stats.StrategyStats[result.Strategy] = val

	if result.Success {
		atomic.AddUint64(&m.stats.SuccessfulWrites, 1)
	} else {
		atomic.AddUint64(&m.stats.FailedWrites, 1)
	}

	// Update cluster stats
	for clusterName, clusterResult := range result.Results {
		stats, exists := m.stats.ClusterStats[clusterName]
		if !exists {
			stats = &ClusterStats{}
			m.stats.ClusterStats[clusterName] = stats
		}

		atomic.AddUint64(&stats.Writes, 1)
		if clusterResult.Error == nil {
			atomic.AddUint64(&stats.Successes, 1)
		} else {
			atomic.AddUint64(&stats.Failures, 1)
		}
		stats.LastWrite = time.Now()
	}
}

func (m *Manager) healthMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkClusterHealth()
		}
	}
}

func (m *Manager) checkClusterHealth() {
	for groupName, group := range m.clusters {
		if !group.Enabled || group.HealthCheck == nil || !group.HealthCheck.Enabled {
			continue
		}

		clusters := []*ClusterConfig{group.Primary}
		clusters = append(clusters, group.Secondaries...)

		for _, cluster := range clusters {
			go m.checkSingleClusterHealth(cluster, groupName)
		}
	}
}

func (m *Manager) checkSingleClusterHealth(cluster *ClusterConfig, groupName string) {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()

	group := m.clusters[groupName]
	healthQuery := group.HealthCheck.Query
	if healthQuery == "" {
		healthQuery = "SELECT 1"
	}

	_, err := m.executeQuery(ctx, cluster, healthQuery, nil)

	stats := m.stats.ClusterStats[cluster.Name]
	if stats == nil {
		stats = &ClusterStats{}
		m.stats.ClusterStats[cluster.Name] = stats
	}

	if err != nil {
		stats.Health = "unhealthy"
		m.logger.Warn("Cluster health check failed",
			zap.String("cluster", cluster.Name),
			zap.Error(err))
	} else {
		stats.Health = "healthy"
	}
}

func (m *Manager) statsCollector() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.stats.LastUpdate = time.Now()
		}
	}
}

func (m *Manager) GetStats() *Stats {
	return m.stats
}

func (m *Manager) Close() error {
	m.cancel()

	if m.workerPool != nil {
		m.workerPool.Close()
	}

	m.logger.Info("Multi-write manager stopped")
	return nil
}