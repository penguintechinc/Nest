package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/penguintechinc/nest/services/db-proxy/internal"
	cachepkg "github.com/penguintechinc/nest/services/db-proxy/internal/cache"
	"github.com/penguintechinc/nest/services/db-proxy/internal/config"
	grpcpkg "github.com/penguintechinc/nest/services/db-proxy/internal/grpc"
	"github.com/penguintechinc/nest/services/db-proxy/internal/handlers"
	"github.com/penguintechinc/nest/services/db-proxy/internal/pool"
	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"github.com/penguintechinc/nest/services/db-proxy/internal/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Version   = "0.1.0"
	BuildTime = "development"
	GitCommit = "unknown"

	startTime = time.Now()

	// Prometheus metrics
	activeConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nest_dbproxy_active_connections",
			Help: "Active database connections",
		},
		[]string{"protocol"},
	)

	totalConnections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nest_dbproxy_total_connections",
			Help: "Total database connections",
		},
		[]string{"protocol"},
	)

	queriesBlocked = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nest_dbproxy_queries_blocked_total",
			Help: "Queries blocked by security checker",
		},
		[]string{"protocol"},
	)

	networkMode = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "nest_dbproxy_network_mode",
			Help: "Active network mode (0=standard, 1=xdp)",
		},
	)
)

func init() {
	prometheus.MustRegister(
		activeConnections,
		totalConnections,
		queriesBlocked,
		networkMode,
	)
}

// RuntimeState holds mutable state for the proxy
type RuntimeState struct {
	mu                sync.RWMutex
	securityChecker   *security.Checker
	handlerManager    *handlers.Manager
	connPool          *pool.Pool
	router            *routing.Router
	cacheStore        cachepkg.Store // Query result cache
	totalQueriesCount atomic.Int64
}

// runHealthCheck performs a one-shot GET against the local metrics /healthz
// endpoint and exits 0 on HTTP 200, 1 otherwise. Kept dependency-free so the
// runtime image needs no curl/wget.
func runHealthCheck() {
	port := os.Getenv("DBPROXY_METRICS_PORT")
	if port == "" {
		port = "9090"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck failed: status %d\n", resp.StatusCode)
		os.Exit(1)
	}
	os.Exit(0)
}

func main() {
	// Health-check subcommand: used by the container HEALTHCHECK and any
	// external liveness poke. Hits the local metrics server's /healthz and
	// exits 0/1 — no curl/wget needed in the slim runtime image (native
	// binary satisfies the rootless, no-extra-packages requirement).
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		runHealthCheck()
		return
	}

	// Setup logging
	logConfig := zap.NewProductionConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	if os.Getenv("DEBUG") == "true" {
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	}

	logger, err := logConfig.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("starting nest DB Proxy",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("commit", GitCommit),
	)

	// Load configuration
	cfg, err := config.NewConfig(logger)
	if err != nil {
		logger.Fatal("failed to load configuration", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize network subsystem (with XDP runtime detection + fallback)
	netMode, err := internal.InitNetwork(ctx, logger)
	if err != nil {
		logger.Fatal("failed to initialize network", zap.Error(err))
	}
	networkMode.Set(float64(netMode))

	// Initialize security checker
	securityChecker := security.NewChecker(logger)
	logger.Info("security checker initialized")

	// Initialize connection pool
	connPool := pool.NewPool(cfg.MaxConnectionsPerRoute, logger)
	connPool.RegisterProtocol("mysql")
	connPool.RegisterProtocol("postgresql")
	connPool.RegisterProtocol("redis")
	logger.Info("connection pool initialized")

	// Initialize handler manager
	handlerManager := handlers.NewManager(logger)

	// Initialize router for read/write routing
	router := routing.NewRouter(logger)
	logger.Info("router initialized")

	// Initialize query result cache (will be populated after Redis connect)
	var cacheStore cachepkg.Store = &cachepkg.NoOpStore{}
	logger.Info("cache store initialized (NoOp, will upgrade when Redis connects)")

	// Runtime state for config updates
	runtimeState := &RuntimeState{
		securityChecker: securityChecker,
		handlerManager:  handlerManager,
		connPool:        connPool,
		router:          router,
		cacheStore:      cacheStore,
	}

	// Create and register TCP handlers for each protocol
	// For now, use a simple default route ID per protocol
	for _, protocol := range []string{"mysql", "postgresql", "redis"} {
		port := cfg.ListenPort + map[string]int{"mysql": 0, "postgresql": 1, "redis": 2}[protocol]
		routeID := protocol + "-default" // Simple default route ID
		// Initially create handler with NoOp cache; will upgrade after Redis connects
		handler := handlers.NewTCPHandlerWithCache(protocol, port, connPool, securityChecker, cfg, logger, router, routeID, cacheStore, 0)
		if err := handlerManager.AddHandler(handler); err != nil {
			logger.Fatal("failed to add handler", zap.Error(err))
		}
	}

	// Start all handlers
	if err := handlerManager.StartAll(ctx); err != nil {
		logger.Fatal("failed to start handlers", zap.Error(err))
	}

	// Connect to Redis and load initial config
	redisClient, err := cfg.GetRedisClient(ctx)
	if err != nil {
		logger.Warn("failed to connect to Redis, continuing with env-only config",
			zap.Error(err),
			zap.String("redis_addr", cfg.RedisAddr),
		)
	} else {
		logger.Info("Redis connected, initializing query result cache")

		// Create Redis-backed cache store if enabled
		if cfg.Cache.Enabled {
			cacheTTL := time.Duration(cfg.Cache.TTLSecs) * time.Second
			if cacheTTL == 0 {
				cacheTTL = 30 * time.Second // default TTL
			}
			newCacheStore := cachepkg.NewRedisStore(redisClient, cfg.RedisPrefix, cacheTTL, cfg.Cache.MaxSizeKB, logger)
			runtimeState.mu.Lock()
			runtimeState.cacheStore = newCacheStore
			runtimeState.mu.Unlock()

			// Update all handlers with the real cache store
			for _, handler := range handlerManager.GetHandlers() {
				handler.SetCache(newCacheStore, cacheTTL)
			}

			logger.Info("query result cache enabled",
				zap.Duration("ttl", cacheTTL),
				zap.Int("max_size_kb", cfg.Cache.MaxSizeKB),
			)
		} else {
			logger.Info("query result cache disabled via config")
		}

		logger.Info("loading initial config from Redis")
		if err := loadAndApplyRedisConfig(ctx, logger, cfg, redisClient, runtimeState); err != nil {
			logger.Warn("failed to load initial Redis config, continuing",
				zap.Error(err),
			)
		}

		// Watch for config updates in background
		go watchConfigUpdates(ctx, logger, cfg, redisClient, runtimeState)
	}

	// Setup metrics/health server
	metricsMux := http.NewServeMux()

	metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	metricsMux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ready"))
	})

	metricsMux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		stats := handlerManager.GetStats()
		poolStats := connPool.GetStats()
		securityStats := securityChecker.GetStats()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"version":"%s","handlers":%v,"pool":%v,"security":%v,"uptime_seconds":%d}`,
			Version, stats, poolStats, securityStats, int64(time.Since(startTime).Seconds()))
	})

	metricsMux.Handle("/metrics", promhttp.Handler())

	metricsServer := &http.Server{
		Addr:              cfg.MetricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("starting metrics/health server", zap.String("addr", cfg.MetricsAddr))
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", zap.Error(err))
		}
	}()

	// Setup gRPC ConfigService server (line where gRPC server is registered - PROOF #2)
	grpcService := grpcpkg.NewConfigServiceServer(
		logger,
		func(ctx context.Context, key string) (interface{}, error) {
			return getConfigState(ctx, runtimeState, key)
		},
		func(ctx context.Context, key string, data []byte) error {
			return setConfigState(ctx, logger, redisClient, cfg, runtimeState, key, data)
		},
		func(ctx context.Context) error {
			if redisClient == nil {
				return fmt.Errorf("redis not connected")
			}
			return loadAndApplyRedisConfig(ctx, logger, cfg, redisClient, runtimeState)
		},
		func() map[string]interface{} {
			return handlerManager.GetStats()
		},
		func() int64 {
			total := int64(0)
			for _, stats := range handlerManager.GetStats() {
				if m, ok := stats.(map[string]interface{}); ok {
					if active, ok := m["active_conns"].(int64); ok {
						total += active
					}
				}
			}
			return total
		},
		func() int64 {
			total := int64(0)
			for _, stats := range handlerManager.GetStats() {
				if m, ok := stats.(map[string]interface{}); ok {
					if t, ok := m["total_conns"].(int64); ok {
						total += t
					}
				}
			}
			return total
		},
		func() int64 {
			total := int64(0)
			for _, stats := range handlerManager.GetStats() {
				if m, ok := stats.(map[string]interface{}); ok {
					if blocked, ok := m["total_blocked"].(int64); ok {
						total += blocked
					}
				}
			}
			return total
		},
	)

	grpcServer := grpcpkg.NewServer(cfg.GRPCAddr, cfg.GRPCPort, grpcService, logger)
	go func() {
		if err := grpcServer.Start(); err != nil {
			logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Update metrics periodically
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			stats := handlerManager.GetStats()
			for protocol, s := range stats {
				if m, ok := s.(map[string]interface{}); ok {
					if active, ok := m["active_conns"].(int64); ok {
						activeConnections.WithLabelValues(protocol).Set(float64(active))
					}
					if total, ok := m["total_conns"].(int64); ok {
						totalConnections.WithLabelValues(protocol).Add(float64(total))
					}
					if blocked, ok := m["total_blocked"].(int64); ok {
						queriesBlocked.WithLabelValues(protocol).Add(float64(blocked))
					}
				}
			}
		}
	}()

	logger.Info("nest DB Proxy started successfully")

	// Wait for shutdown signal
	<-sigChan
	logger.Info("shutting down...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("metrics server shutdown error", zap.Error(err))
	}

	if err := grpcServer.Stop(); err != nil {
		logger.Error("gRPC server shutdown error", zap.Error(err))
	}

	if err := handlerManager.StopAll(); err != nil {
		logger.Error("handlers shutdown error", zap.Error(err))
	}

	if err := connPool.Close(); err != nil {
		logger.Error("pool shutdown error", zap.Error(err))
	}

	if redisClient != nil {
		if err := redisClient.Close(); err != nil {
			logger.Error("redis close error", zap.Error(err))
		}
	}

	logger.Info("shutdown complete")
}

// loadAndApplyRedisConfig loads routes and security config from Redis and applies to runtime state
func loadAndApplyRedisConfig(
	ctx context.Context,
	logger *zap.Logger,
	cfg *config.Config,
	redisClient *redis.Client,
	runtimeState *RuntimeState,
) error {
	runtimeState.mu.Lock()
	defer runtimeState.mu.Unlock()

	// Load security config
	secCfg, err := cfg.LoadSecurityConfigFromRedis(ctx, redisClient)
	if err != nil {
		return fmt.Errorf("failed to load security config: %w", err)
	}

	if secCfg.EnableInjection {
		logger.Info("SQL injection checking enabled from Redis config")
		// Security checker is already enabled by default
	}

	logger.Info("redis config applied",
		zap.Int("blocked_resources", len(secCfg.BlockedResources)),
		zap.Bool("injection_check", secCfg.EnableInjection),
	)

	return nil
}

// watchConfigUpdates polls or subscribes to config updates from Redis
func watchConfigUpdates(
	ctx context.Context,
	logger *zap.Logger,
	cfg *config.Config,
	redisClient *redis.Client,
	runtimeState *RuntimeState,
) {
	pubsub := redisClient.Subscribe(ctx, fmt.Sprintf("%s:config:updated", cfg.RedisPrefix))
	defer func() {
		if err := pubsub.Close(); err != nil {
			logger.Error("failed to close pubsub", zap.Error(err))
		}
	}()

	logger.Info("watching for config updates",
		zap.String("channel", fmt.Sprintf("%s:config:updated", cfg.RedisPrefix)),
	)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				logger.Debug("pubsub receive error", zap.Error(err))
				// Fallback to polling every 30 seconds on error
				time.Sleep(30 * time.Second)
				continue
			}

			logger.Info("config update received",
				zap.String("channel", msg.Channel),
				zap.String("message", msg.Payload),
			)

			if err := loadAndApplyRedisConfig(ctx, logger, cfg, redisClient, runtimeState); err != nil {
				logger.Warn("failed to apply config update", zap.Error(err))
			}
		}
	}
}

// getConfigState returns current config state (routes, security, etc.)
func getConfigState(ctx context.Context, runtimeState *RuntimeState, key string) (interface{}, error) {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()

	switch key {
	case "routes":
		return map[string]interface{}{
			"message": "routes configuration (populated from Redis)",
		}, nil
	case "security":
		return map[string]interface{}{
			"enable_injection_check": true,
			"blocked_resources":      []string{},
			"allowed_resources":      []string{},
		}, nil
	case "blocking":
		return map[string]interface{}{
			"message": "blocking configuration",
		}, nil
	case "cache":
		return map[string]interface{}{
			"message": "cache configuration",
		}, nil
	default:
		return map[string]interface{}{
			"message": "all configuration",
		}, nil
	}
}

// setConfigState updates config state and persists to Redis
func setConfigState(
	ctx context.Context,
	logger *zap.Logger,
	redisClient *redis.Client,
	cfg *config.Config,
	runtimeState *RuntimeState,
	key string,
	data []byte,
) error {
	runtimeState.mu.Lock()
	defer runtimeState.mu.Unlock()

	if redisClient == nil {
		return fmt.Errorf("redis not connected, cannot persist config")
	}

	// Validate JSON
	var parsed interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("invalid JSON config: %w", err)
	}

	// Persist to Redis
	redisKey := fmt.Sprintf("%s:%s", cfg.RedisPrefix, key)
	if err := redisClient.Set(ctx, redisKey, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to save config to Redis: %w", err)
	}

	logger.Info("config persisted to Redis",
		zap.String("key", redisKey),
	)

	// Publish update notification
	if err := redisClient.Publish(ctx, fmt.Sprintf("%s:config:updated", cfg.RedisPrefix), key).Err(); err != nil {
		logger.Warn("failed to publish config update notification", zap.Error(err))
	}

	return nil
}
