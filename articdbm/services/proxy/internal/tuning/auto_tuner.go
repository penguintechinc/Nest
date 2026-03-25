package tuning

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/penguintechinc/articdbm/proxy/internal/config"
	"github.com/penguintechinc/articdbm/proxy/internal/metrics"
	"github.com/penguintechinc/articdbm/proxy/internal/numa"
	"github.com/penguintechinc/articdbm/proxy/internal/xdp"
	"github.com/go-redis/redis/v8"
)

// AutoTuner automatically optimizes ArticDBM performance parameters
type AutoTuner struct {
	config        *config.Config
	xdpController *xdp.Controller
	topology      *numa.TopologyInfo
	redisClient   *redis.Client
	logger        *zap.Logger

	// Current performance metrics
	currentMetrics *PerformanceMetrics
	historicalData *HistoricalMetrics

	// Tuning parameters
	tuningConfig *TuningConfig
	activeRules  map[string]*TuningRule

	// Control
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	isRunning   bool
	lastTuning  time.Time
}

// PerformanceMetrics represents current system performance
type PerformanceMetrics struct {
	Timestamp        time.Time                    `json:"timestamp"`
	PacketsPerSecond float64                      `json:"packets_per_second"`
	LatencyP50       time.Duration                `json:"latency_p50"`
	LatencyP95       time.Duration                `json:"latency_p95"`
	LatencyP99       time.Duration                `json:"latency_p99"`
	CPUUtilization   map[int]float64              `json:"cpu_utilization"`
	MemoryUsage      map[int]MemoryStats          `json:"memory_usage"`
	CacheHitRatio    float64                      `json:"cache_hit_ratio"`
	ErrorRate        float64                      `json:"error_rate"`
	ConnectionCount  int                          `json:"connection_count"`
	NUMAEfficiency   map[int]float64              `json:"numa_efficiency"`
	XDPMetrics       map[string]XDPPerfMetrics    `json:"xdp_metrics"`
	SystemLoad       SystemLoadMetrics            `json:"system_load"`
}

type MemoryStats struct {
	Used      uint64  `json:"used"`
	Free      uint64  `json:"free"`
	Cached    uint64  `json:"cached"`
	Available uint64  `json:"available"`
	Pressure  float64 `json:"pressure"` // 0.0-1.0
}

type XDPPerfMetrics struct {
	Interface        string    `json:"interface"`
	ProcessingRate   float64   `json:"processing_rate"`
	DropRate         float64   `json:"drop_rate"`
	CacheHitRatio    float64   `json:"cache_hit_ratio"`
	RateLimitHits    float64   `json:"rate_limit_hits"`
	NUMANode         int       `json:"numa_node"`
	RingUtilization  float64   `json:"ring_utilization"`
	BatchEfficiency  float64   `json:"batch_efficiency"`
}

type SystemLoadMetrics struct {
	Load1Min       float64 `json:"load_1min"`
	Load5Min       float64 `json:"load_5min"`
	Load15Min      float64 `json:"load_15min"`
	ContextSwitches float64 `json:"context_switches"`
	Interrupts     float64 `json:"interrupts"`
	NetworkRxPPS   float64 `json:"network_rx_pps"`
	NetworkTxPPS   float64 `json:"network_tx_pps"`
}

// HistoricalMetrics stores historical performance data
type HistoricalMetrics struct {
	DataPoints    []PerformanceMetrics `json:"data_points"`
	MaxDataPoints int                  `json:"max_data_points"`
	Trends        map[string]Trend     `json:"trends"`
}

type Trend struct {
	Metric      string    `json:"metric"`
	Direction   string    `json:"direction"` // "increasing", "decreasing", "stable"
	Confidence  float64   `json:"confidence"` // 0.0-1.0
	Rate        float64   `json:"rate"`       // Rate of change
	LastUpdate  time.Time `json:"last_update"`
}

// TuningConfig defines tuning behavior
type TuningConfig struct {
	Enabled              bool                        `json:"enabled"`
	TuningInterval       time.Duration               `json:"tuning_interval"`
	AggressiveMode       bool                        `json:"aggressive_mode"`
	SafetyThresholds     SafetyThresholds           `json:"safety_thresholds"`
	TuningRules          map[string]*TuningRule     `json:"tuning_rules"`
	PerformanceTargets   PerformanceTargets         `json:"performance_targets"`
	AdaptationWeights    map[string]float64         `json:"adaptation_weights"`
	MLModelConfig        MLModelConfig              `json:"ml_model_config"`
}

type SafetyThresholds struct {
	MaxCPUUtilization    float64 `json:"max_cpu_utilization"`
	MaxMemoryUtilization float64 `json:"max_memory_utilization"`
	MinCacheHitRatio     float64 `json:"min_cache_hit_ratio"`
	MaxLatencyP99        time.Duration `json:"max_latency_p99"`
	MaxErrorRate         float64 `json:"max_error_rate"`
}

type PerformanceTargets struct {
	TargetPPS           float64       `json:"target_pps"`
	TargetLatencyP95    time.Duration `json:"target_latency_p95"`
	TargetCacheHitRatio float64       `json:"target_cache_hit_ratio"`
	TargetCPUUsage      float64       `json:"target_cpu_usage"`
	TargetNUMAEfficiency float64      `json:"target_numa_efficiency"`
}

type TuningRule struct {
	Name        string                 `json:"name"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority"`
	Conditions  []TuningCondition      `json:"conditions"`
	Actions     []TuningAction         `json:"actions"`
	Cooldown    time.Duration          `json:"cooldown"`
	LastApplied time.Time              `json:"last_applied"`
	SuccessRate float64                `json:"success_rate"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type TuningCondition struct {
	Metric    string      `json:"metric"`
	Operator  string      `json:"operator"` // "gt", "lt", "eq", "gte", "lte"
	Value     float64     `json:"value"`
	Duration  time.Duration `json:"duration"` // Must be true for this duration
	Weight    float64     `json:"weight"`   // Importance weight
}

type TuningAction struct {
	Type       string                 `json:"type"` // "adjust_parameter", "restart_component", "scale"
	Target     string                 `json:"target"` // What to adjust
	Value      interface{}            `json:"value"`  // New value or adjustment
	Gradual    bool                   `json:"gradual"` // Apply gradually
	Steps      int                    `json:"steps"`   // Number of steps for gradual
	Metadata   map[string]interface{} `json:"metadata"`
}

type MLModelConfig struct {
	Enabled           bool              `json:"enabled"`
	ModelType         string            `json:"model_type"` // "linear", "neural", "ensemble"
	TrainingInterval  time.Duration     `json:"training_interval"`
	PredictionWindow  time.Duration     `json:"prediction_window"`
	Features          []string          `json:"features"`
	Targets           []string          `json:"targets"`
	HyperParameters   map[string]float64 `json:"hyper_parameters"`
}

func NewAutoTuner(config *config.Config, xdpController *xdp.Controller, topology *numa.TopologyInfo, redisClient *redis.Client, logger *zap.Logger) *AutoTuner {
	ctx, cancel := context.WithCancel(context.Background())

	tuner := &AutoTuner{
		config:        config,
		xdpController: xdpController,
		topology:      topology,
		redisClient:   redisClient,
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
		activeRules:   make(map[string]*TuningRule),
		historicalData: &HistoricalMetrics{
			DataPoints:    make([]PerformanceMetrics, 0),
			MaxDataPoints: 1000,
			Trends:        make(map[string]Trend),
		},
	}

	tuner.loadTuningConfig()
	tuner.initializeRules()

	// Start tuning loop
	go tuner.tuningLoop()
	go tuner.metricsCollector()
	go tuner.trendAnalyzer()

	logger.Info("Auto-tuner initialized",
		zap.Bool("enabled", tuner.tuningConfig.Enabled),
		zap.Duration("interval", tuner.tuningConfig.TuningInterval))

	return tuner
}

func (at *AutoTuner) loadTuningConfig() {
	at.tuningConfig = &TuningConfig{
		Enabled:        true,
		TuningInterval: 30 * time.Second,
		AggressiveMode: false,
		SafetyThresholds: SafetyThresholds{
			MaxCPUUtilization:    0.85,
			MaxMemoryUtilization: 0.9,
			MinCacheHitRatio:     0.8,
			MaxLatencyP99:        10 * time.Millisecond,
			MaxErrorRate:         0.01,
		},
		PerformanceTargets: PerformanceTargets{
			TargetPPS:            100000000, // 100M PPS
			TargetLatencyP95:     1 * time.Millisecond,
			TargetCacheHitRatio:  0.95,
			TargetCPUUsage:       0.7,
			TargetNUMAEfficiency: 0.95,
		},
		AdaptationWeights: map[string]float64{
			"latency":        0.3,
			"throughput":     0.25,
			"cache_hit":      0.2,
			"cpu_efficiency": 0.15,
			"numa_locality":  0.1,
		},
		MLModelConfig: MLModelConfig{
			Enabled:          true,
			ModelType:        "ensemble",
			TrainingInterval: 5 * time.Minute,
			PredictionWindow: 1 * time.Minute,
			Features: []string{
				"packets_per_second", "cpu_utilization", "memory_usage",
				"cache_hit_ratio", "numa_efficiency", "connection_count",
			},
			Targets: []string{"latency_p95", "throughput", "error_rate"},
		},
	}

	// Try to load from Redis
	if configData, err := at.redisClient.Get(at.ctx, "articdbm:tuning:config").Result(); err == nil {
		if err := json.Unmarshal([]byte(configData), at.tuningConfig); err == nil {
			at.logger.Info("Loaded tuning configuration from Redis")
		}
	}
}

func (at *AutoTuner) initializeRules() {
	defaultRules := []*TuningRule{
		{
			Name:     "xdp_cache_size_optimization",
			Enabled:  true,
			Priority: 1,
			Conditions: []TuningCondition{
				{Metric: "cache_hit_ratio", Operator: "lt", Value: 0.9, Duration: 60 * time.Second, Weight: 1.0},
				{Metric: "memory_pressure", Operator: "lt", Value: 0.7, Duration: 30 * time.Second, Weight: 0.8},
			},
			Actions: []TuningAction{
				{Type: "adjust_parameter", Target: "xdp_cache_size", Value: 1.2, Gradual: true, Steps: 5},
			},
			Cooldown: 5 * time.Minute,
		},
		{
			Name:     "numa_worker_rebalancing",
			Enabled:  true,
			Priority: 2,
			Conditions: []TuningCondition{
				{Metric: "numa_efficiency", Operator: "lt", Value: 0.9, Duration: 90 * time.Second, Weight: 1.0},
				{Metric: "cross_numa_traffic", Operator: "gt", Value: 0.1, Duration: 60 * time.Second, Weight: 0.9},
			},
			Actions: []TuningAction{
				{Type: "rebalance_workers", Target: "numa_nodes", Value: "auto", Gradual: false},
			},
			Cooldown: 10 * time.Minute,
		},
		{
			Name:     "xdp_rate_limit_adjustment",
			Enabled:  true,
			Priority: 3,
			Conditions: []TuningCondition{
				{Metric: "packets_per_second", Operator: "gt", Value: 50000000, Duration: 30 * time.Second, Weight: 1.0},
				{Metric: "drop_rate", Operator: "gt", Value: 0.001, Duration: 30 * time.Second, Weight: 0.8},
			},
			Actions: []TuningAction{
				{Type: "adjust_parameter", Target: "xdp_rate_limit", Value: 1.1, Gradual: true, Steps: 3},
			},
			Cooldown: 2 * time.Minute,
		},
		{
			Name:     "connection_pool_scaling",
			Enabled:  true,
			Priority: 4,
			Conditions: []TuningCondition{
				{Metric: "connection_utilization", Operator: "gt", Value: 0.8, Duration: 120 * time.Second, Weight: 1.0},
				{Metric: "latency_p95", Operator: "gt", Value: 0.005, Duration: 60 * time.Second, Weight: 0.7}, // 5ms
			},
			Actions: []TuningAction{
				{Type: "adjust_parameter", Target: "max_connections", Value: 1.25, Gradual: true, Steps: 4},
			},
			Cooldown: 15 * time.Minute,
		},
		{
			Name:     "afxdp_batch_size_optimization",
			Enabled:  true,
			Priority: 5,
			Conditions: []TuningCondition{
				{Metric: "afxdp_ring_utilization", Operator: "gt", Value: 0.7, Duration: 60 * time.Second, Weight: 1.0},
				{Metric: "batch_efficiency", Operator: "lt", Value: 0.8, Duration: 90 * time.Second, Weight: 0.9},
			},
			Actions: []TuningAction{
				{Type: "adjust_parameter", Target: "afxdp_batch_size", Value: 1.5, Gradual: true, Steps: 3},
			},
			Cooldown: 3 * time.Minute,
		},
		{
			Name:     "emergency_performance_mode",
			Enabled:  true,
			Priority: 10,
			Conditions: []TuningCondition{
				{Metric: "latency_p99", Operator: "gt", Value: 0.02, Duration: 30 * time.Second, Weight: 1.0}, // 20ms
				{Metric: "error_rate", Operator: "gt", Value: 0.05, Duration: 30 * time.Second, Weight: 1.0},
			},
			Actions: []TuningAction{
				{Type: "enable_emergency_mode", Target: "all_components", Value: true, Gradual: false},
				{Type: "adjust_parameter", Target: "cache_aggressive_eviction", Value: true, Gradual: false},
			},
			Cooldown: 30 * time.Second,
		},
	}

	for _, rule := range defaultRules {
		at.activeRules[rule.Name] = rule
	}

	at.logger.Info("Initialized tuning rules", zap.Int("count", len(at.activeRules)))
}

func (at *AutoTuner) tuningLoop() {
	ticker := time.NewTicker(at.tuningConfig.TuningInterval)
	defer ticker.Stop()

	for {
		select {
		case <-at.ctx.Done():
			return
		case <-ticker.C:
			if at.tuningConfig.Enabled {
				at.performTuning()
			}
		}
	}
}

func (at *AutoTuner) performTuning() {
	at.mu.Lock()
	defer at.mu.Unlock()

	if at.currentMetrics == nil {
		at.logger.Debug("No current metrics available, skipping tuning")
		return
	}

	at.logger.Debug("Starting performance tuning cycle")
	start := time.Now()

	// Check safety thresholds first
	if !at.checkSafetyThresholds() {
		at.logger.Warn("Safety thresholds violated, skipping tuning")
		return
	}

	// Evaluate rules
	applicableRules := at.evaluateRules()
	if len(applicableRules) == 0 {
		at.logger.Debug("No applicable tuning rules")
		return
	}

	// Sort by priority and apply
	at.applyTuningRules(applicableRules)

	// Update metrics
	at.lastTuning = time.Now()
	duration := time.Since(start)

	at.logger.Info("Completed tuning cycle",
		zap.Duration("duration", duration),
		zap.Int("rules_applied", len(applicableRules)))

	// Store tuning event in Redis
	at.recordTuningEvent(applicableRules, duration)
}

func (at *AutoTuner) checkSafetyThresholds() bool {
	thresholds := at.tuningConfig.SafetyThresholds
	metrics := at.currentMetrics

	// Check CPU utilization
	maxCPU := 0.0
	for _, usage := range metrics.CPUUtilization {
		if usage > maxCPU {
			maxCPU = usage
		}
	}
	if maxCPU > thresholds.MaxCPUUtilization {
		at.logger.Warn("CPU utilization threshold exceeded",
			zap.Float64("current", maxCPU),
			zap.Float64("threshold", thresholds.MaxCPUUtilization))
		return false
	}

	// Check memory utilization
	maxMemPressure := 0.0
	for _, memStats := range metrics.MemoryUsage {
		if memStats.Pressure > maxMemPressure {
			maxMemPressure = memStats.Pressure
		}
	}
	if maxMemPressure > thresholds.MaxMemoryUtilization {
		at.logger.Warn("Memory pressure threshold exceeded",
			zap.Float64("current", maxMemPressure),
			zap.Float64("threshold", thresholds.MaxMemoryUtilization))
		return false
	}

	// Check cache hit ratio
	if metrics.CacheHitRatio < thresholds.MinCacheHitRatio {
		at.logger.Warn("Cache hit ratio threshold not met",
			zap.Float64("current", metrics.CacheHitRatio),
			zap.Float64("threshold", thresholds.MinCacheHitRatio))
		return false
	}

	// Check latency
	if metrics.LatencyP99 > thresholds.MaxLatencyP99 {
		at.logger.Warn("Latency threshold exceeded",
			zap.Duration("current", metrics.LatencyP99),
			zap.Duration("threshold", thresholds.MaxLatencyP99))
		return false
	}

	// Check error rate
	if metrics.ErrorRate > thresholds.MaxErrorRate {
		at.logger.Warn("Error rate threshold exceeded",
			zap.Float64("current", metrics.ErrorRate),
			zap.Float64("threshold", thresholds.MaxErrorRate))
		return false
	}

	return true
}

func (at *AutoTuner) evaluateRules() []*TuningRule {
	var applicableRules []*TuningRule
	now := time.Now()

	for _, rule := range at.activeRules {
		if !rule.Enabled {
			continue
		}

		// Check cooldown
		if now.Sub(rule.LastApplied) < rule.Cooldown {
			continue
		}

		// Evaluate conditions
		if at.evaluateConditions(rule.Conditions) {
			applicableRules = append(applicableRules, rule)
		}
	}

	return applicableRules
}

func (at *AutoTuner) evaluateConditions(conditions []TuningCondition) bool {
	totalWeight := 0.0
	metWeight := 0.0

	for _, condition := range conditions {
		totalWeight += condition.Weight

		value := at.getMetricValue(condition.Metric)
		if value == nil {
			continue
		}

		conditionMet := false
		switch condition.Operator {
		case "gt":
			conditionMet = *value > condition.Value
		case "lt":
			conditionMet = *value < condition.Value
		case "gte":
			conditionMet = *value >= condition.Value
		case "lte":
			conditionMet = *value <= condition.Value
		case "eq":
			conditionMet = math.Abs(*value - condition.Value) < 0.001
		}

		if conditionMet {
			metWeight += condition.Weight
		}
	}

	// Require at least 70% of weighted conditions to be met
	return totalWeight > 0 && (metWeight / totalWeight) >= 0.7
}

func (at *AutoTuner) getMetricValue(metric string) *float64 {
	if at.currentMetrics == nil {
		return nil
	}

	var value float64
	switch metric {
	case "packets_per_second":
		value = at.currentMetrics.PacketsPerSecond
	case "cache_hit_ratio":
		value = at.currentMetrics.CacheHitRatio
	case "error_rate":
		value = at.currentMetrics.ErrorRate
	case "connection_count":
		value = float64(at.currentMetrics.ConnectionCount)
	case "latency_p95":
		value = float64(at.currentMetrics.LatencyP95.Nanoseconds()) / 1e9
	case "latency_p99":
		value = float64(at.currentMetrics.LatencyP99.Nanoseconds()) / 1e9
	case "memory_pressure":
		maxPressure := 0.0
		for _, memStats := range at.currentMetrics.MemoryUsage {
			if memStats.Pressure > maxPressure {
				maxPressure = memStats.Pressure
			}
		}
		value = maxPressure
	case "numa_efficiency":
		totalEfficiency := 0.0
		count := 0
		for _, efficiency := range at.currentMetrics.NUMAEfficiency {
			totalEfficiency += efficiency
			count++
		}
		if count > 0 {
			value = totalEfficiency / float64(count)
		}
	default:
		return nil
	}

	return &value
}

func (at *AutoTuner) applyTuningRules(rules []*TuningRule) {
	// Sort by priority (higher priority first)
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[i].Priority < rules[j].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}

	for _, rule := range rules {
		at.logger.Info("Applying tuning rule", zap.String("rule", rule.Name))

		success := true
		for _, action := range rule.Actions {
			if err := at.executeAction(action); err != nil {
				at.logger.Error("Failed to execute tuning action",
					zap.String("rule", rule.Name),
					zap.String("action", action.Type),
					zap.Error(err))
				success = false
			}
		}

		// Update rule statistics
		rule.LastApplied = time.Now()
		if success {
			rule.SuccessRate = rule.SuccessRate*0.9 + 0.1 // Exponential moving average
		} else {
			rule.SuccessRate = rule.SuccessRate * 0.9
		}

		at.activeRules[rule.Name] = rule
	}
}

func (at *AutoTuner) executeAction(action TuningAction) error {
	switch action.Type {
	case "adjust_parameter":
		return at.adjustParameter(action)
	case "rebalance_workers":
		return at.rebalanceWorkers(action)
	case "enable_emergency_mode":
		return at.enableEmergencyMode(action)
	case "restart_component":
		return at.restartComponent(action)
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

func (at *AutoTuner) adjustParameter(action TuningAction) error {
	target := action.Target
	multiplier, ok := action.Value.(float64)
	if !ok {
		return fmt.Errorf("invalid value type for parameter adjustment: %T", action.Value)
	}

	switch target {
	case "xdp_cache_size":
		currentSize := at.config.XDPCacheSize
		newSize := uint32(float64(currentSize) * multiplier)
		if err := at.xdpController.UpdateCacheSize(newSize); err != nil {
			return err
		}
		at.config.XDPCacheSize = newSize

	case "xdp_rate_limit":
		currentLimit := at.config.XDPRateLimitPPS
		newLimit := uint64(float64(currentLimit) * multiplier)
		if err := at.xdpController.UpdateRateLimit(newLimit); err != nil {
			return err
		}
		at.config.XDPRateLimitPPS = newLimit

	case "afxdp_batch_size":
		currentSize := at.config.AFXDPBatchSize
		newSize := int(float64(currentSize) * multiplier)
		if newSize < 1 {
			newSize = 1
		} else if newSize > 512 {
			newSize = 512
		}
		at.config.AFXDPBatchSize = newSize

	case "max_connections":
		currentMax := at.config.MaxConnections
		newMax := int(float64(currentMax) * multiplier)
		if newMax < 10 {
			newMax = 10
		}
		at.config.MaxConnections = newMax

	default:
		return fmt.Errorf("unknown parameter target: %s", target)
	}

	at.logger.Info("Adjusted parameter",
		zap.String("parameter", target),
		zap.Float64("multiplier", multiplier))

	return nil
}

func (at *AutoTuner) rebalanceWorkers(action TuningAction) error {
	if at.topology == nil {
		return fmt.Errorf("NUMA topology not available")
	}

	// Rebalance AF_XDP workers across NUMA nodes
	// This would implement worker rebalancing logic
	at.logger.Info("Rebalancing workers across NUMA nodes")

	return nil
}

func (at *AutoTuner) enableEmergencyMode(action TuningAction) error {
	enabled, ok := action.Value.(bool)
	if !ok {
		return fmt.Errorf("invalid value type for emergency mode: %T", action.Value)
	}

	if at.xdpController != nil {
		if err := at.xdpController.SetEmergencyMode(enabled); err != nil {
			return err
		}
	}

	at.logger.Warn("Emergency mode toggled", zap.Bool("enabled", enabled))
	return nil
}

func (at *AutoTuner) restartComponent(action TuningAction) error {
	component := action.Target
	at.logger.Info("Restarting component", zap.String("component", component))

	// This would implement component restart logic
	return nil
}

func (at *AutoTuner) metricsCollector() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-at.ctx.Done():
			return
		case <-ticker.C:
			at.collectCurrentMetrics()
		}
	}
}

func (at *AutoTuner) collectCurrentMetrics() {
	metrics := &PerformanceMetrics{
		Timestamp: time.Now(),
	}

	// This would collect actual metrics from various sources
	// For now, we'll simulate some basic collection
	if at.xdpController != nil {
		if stats, err := at.xdpController.GetStatistics(); err == nil {
			metrics.PacketsPerSecond = at.calculatePPS(stats)
			metrics.CacheHitRatio = at.calculateCacheHitRatio(stats)
			metrics.ErrorRate = at.calculateErrorRate(stats)
		}
	}

	at.mu.Lock()
	at.currentMetrics = metrics
	at.addToHistory(metrics)
	at.mu.Unlock()
}

func (at *AutoTuner) calculatePPS(stats *xdp.Statistics) float64 {
	// Calculate packets per second across all interfaces
	totalPackets := uint64(0)
	for _, ifaceStats := range stats.Interfaces {
		for _, progStats := range ifaceStats.Programs {
			totalPackets += progStats.PacketsProcessed
		}
	}
	return float64(totalPackets)
}

func (at *AutoTuner) calculateCacheHitRatio(stats *xdp.Statistics) float64 {
	totalHits := uint64(0)
	totalRequests := uint64(0)

	for _, ifaceStats := range stats.Interfaces {
		if cacheStats := ifaceStats.Cache; cacheStats != nil {
			totalHits += cacheStats.Hits
			totalRequests += cacheStats.Hits + cacheStats.Misses
		}
	}

	if totalRequests > 0 {
		return float64(totalHits) / float64(totalRequests)
	}
	return 0.0
}

func (at *AutoTuner) calculateErrorRate(stats *xdp.Statistics) float64 {
	totalErrors := uint64(0)
	totalRequests := uint64(0)

	for _, ifaceStats := range stats.Interfaces {
		for _, progStats := range ifaceStats.Programs {
			totalErrors += progStats.PacketsAborted + progStats.PacketsDropped
			totalRequests += progStats.PacketsProcessed
		}
	}

	if totalRequests > 0 {
		return float64(totalErrors) / float64(totalRequests)
	}
	return 0.0
}

func (at *AutoTuner) addToHistory(metrics *PerformanceMetrics) {
	history := at.historicalData

	history.DataPoints = append(history.DataPoints, *metrics)

	// Keep only the latest data points
	if len(history.DataPoints) > history.MaxDataPoints {
		history.DataPoints = history.DataPoints[1:]
	}
}

func (at *AutoTuner) trendAnalyzer() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-at.ctx.Done():
			return
		case <-ticker.C:
			at.analyzeTrends()
		}
	}
}

func (at *AutoTuner) analyzeTrends() {
	at.mu.RLock()
	defer at.mu.RUnlock()

	if len(at.historicalData.DataPoints) < 10 {
		return // Need more data points
	}

	// Analyze trends for key metrics
	metrics := []string{"packets_per_second", "latency_p95", "cache_hit_ratio", "error_rate"}

	for _, metricName := range metrics {
		trend := at.calculateTrend(metricName)
		if trend != nil {
			at.historicalData.Trends[metricName] = *trend
		}
	}
}

func (at *AutoTuner) calculateTrend(metricName string) *Trend {
	dataPoints := at.historicalData.DataPoints
	if len(dataPoints) < 10 {
		return nil
	}

	// Get values for the metric
	values := make([]float64, len(dataPoints))
	for i, point := range dataPoints {
		switch metricName {
		case "packets_per_second":
			values[i] = point.PacketsPerSecond
		case "latency_p95":
			values[i] = float64(point.LatencyP95.Nanoseconds())
		case "cache_hit_ratio":
			values[i] = point.CacheHitRatio
		case "error_rate":
			values[i] = point.ErrorRate
		}
	}

	// Simple linear regression for trend detection
	n := float64(len(values))
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0

	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)

	direction := "stable"
	if slope > 0.01 {
		direction = "increasing"
	} else if slope < -0.01 {
		direction = "decreasing"
	}

	// Calculate R-squared for confidence
	avgY := sumY / n
	ssRes, ssTot := 0.0, 0.0
	for i, y := range values {
		predicted := slope*float64(i) + (sumY-slope*sumX)/n
		ssRes += (y - predicted) * (y - predicted)
		ssTot += (y - avgY) * (y - avgY)
	}

	rSquared := 1 - ssRes/ssTot
	if rSquared < 0 {
		rSquared = 0
	}

	return &Trend{
		Metric:     metricName,
		Direction:  direction,
		Confidence: rSquared,
		Rate:       slope,
		LastUpdate: time.Now(),
	}
}

func (at *AutoTuner) recordTuningEvent(rules []*TuningRule, duration time.Duration) {
	event := map[string]interface{}{
		"timestamp":    time.Now().Unix(),
		"duration_ms":  duration.Milliseconds(),
		"rules_count":  len(rules),
		"rules":        rules,
		"metrics":      at.currentMetrics,
	}

	eventData, _ := json.Marshal(event)
	key := fmt.Sprintf("articdbm:tuning:events:%d", time.Now().Unix())
	at.redisClient.Set(at.ctx, key, eventData, 24*time.Hour)
}

// GetCurrentMetrics returns the current performance metrics
func (at *AutoTuner) GetCurrentMetrics() *PerformanceMetrics {
	at.mu.RLock()
	defer at.mu.RUnlock()
	return at.currentMetrics
}

// GetTuningRules returns all active tuning rules
func (at *AutoTuner) GetTuningRules() map[string]*TuningRule {
	at.mu.RLock()
	defer at.mu.RUnlock()

	rules := make(map[string]*TuningRule)
	for name, rule := range at.activeRules {
		rules[name] = rule
	}
	return rules
}

// UpdateTuningRule updates or adds a tuning rule
func (at *AutoTuner) UpdateTuningRule(name string, rule *TuningRule) {
	at.mu.Lock()
	defer at.mu.Unlock()

	rule.Name = name
	at.activeRules[name] = rule

	at.logger.Info("Updated tuning rule", zap.String("rule", name))
}

// EnableAggressiveMode enables or disables aggressive tuning
func (at *AutoTuner) EnableAggressiveMode(enabled bool) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.tuningConfig.AggressiveMode = enabled

	if enabled {
		at.tuningConfig.TuningInterval = 15 * time.Second
		at.tuningConfig.SafetyThresholds.MaxCPUUtilization = 0.95
		at.tuningConfig.SafetyThresholds.MaxMemoryUtilization = 0.95
	} else {
		at.tuningConfig.TuningInterval = 30 * time.Second
		at.tuningConfig.SafetyThresholds.MaxCPUUtilization = 0.85
		at.tuningConfig.SafetyThresholds.MaxMemoryUtilization = 0.9
	}

	at.logger.Info("Aggressive mode toggled", zap.Bool("enabled", enabled))
}

// Close shuts down the auto-tuner
func (at *AutoTuner) Close() error {
	at.cancel()
	at.logger.Info("Auto-tuner stopped")
	return nil
}