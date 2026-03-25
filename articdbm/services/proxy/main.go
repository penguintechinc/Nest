package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/penguintechinc/articdbm/proxy/internal/config"
	"github.com/penguintechinc/articdbm/proxy/internal/handlers"
	"github.com/penguintechinc/articdbm/proxy/internal/metrics"
	"github.com/penguintechinc/articdbm/proxy/internal/numa"
	"github.com/penguintechinc/articdbm/proxy/internal/xdp"
	"github.com/penguintechinc/articdbm/proxy/internal/afxdp"
	"github.com/penguintechinc/articdbm/proxy/internal/cache"
	"github.com/penguintechinc/articdbm/proxy/internal/multiwrite"
	"github.com/penguintechinc/articdbm/proxy/internal/bluegreen"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"net/http"
)

var (
	logger *zap.Logger
	cfg    *config.Config
	xdpController *xdp.Controller
	afxdpManager *afxdp.SocketManager
	cacheManager *cache.MultiTierCache
	multiwriteManager *multiwrite.Manager
	deploymentManager *bluegreen.DeploymentManager
)

func init() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
}

func main() {
	defer logger.Sync()

	cfg = config.LoadConfig()
	logger.Info("Starting ArticDBM Proxy with XDP acceleration",
		zap.String("version", cfg.Version),
		zap.Int("port", cfg.ProxyPort))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize NUMA topology and optimize memory allocation
	topology, err := numa.NewTopologyInfo(logger)
	if err != nil {
		logger.Warn("Failed to discover NUMA topology, continuing without optimization", zap.Error(err))
	} else {
		logger.Info("NUMA topology discovered",
			zap.Int("numa_nodes", len(topology.NumaNodes)),
			zap.Int("total_cpus", topology.TotalCores))
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Error("Failed to connect to Redis", zap.Error(err))
		os.Exit(1)
	}

	// Initialize XDP controller
	xdpConfig := &xdp.XDPConfig{
		Interfaces:    []string{cfg.XDPInterface},
		EnableDDoS:    true,
		MaxBlockedIPs: 10000,
		MaxBlockedCIDRs: 1000,
		StatsInterval: 10 * time.Second,
	}
	xdpController, err = xdp.NewController(xdpConfig, logger)
	if err != nil {
		logger.Warn("Failed to initialize XDP controller, continuing without XDP acceleration", zap.Error(err))
	} else {
		logger.Info("XDP controller initialized")
		defer xdpController.Close()
	}

	// Initialize AF_XDP socket manager
	afxdpManager, err = afxdp.NewSocketManager([]string{cfg.XDPInterface}, topology, logger)
	if err != nil {
		logger.Warn("Failed to initialize AF_XDP socket manager", zap.Error(err))
	} else {
		logger.Info("AF_XDP socket manager initialized")
		defer afxdpManager.Close()
	}

	// Initialize multi-tier cache
	cacheConfig := &cache.MultiTierConfig{
		Enabled:    true,
		DefaultTTL: 5 * time.Minute,
	}
	cacheManager, err = cache.NewMultiTierCache(cacheConfig, redisClient, topology, logger)
	if err != nil {
		logger.Warn("Failed to initialize multi-tier cache", zap.Error(err))
	} else {
		logger.Info("Multi-tier cache system initialized")
	}

	// Initialize multi-write manager
	mwConfig := &multiwrite.MultiWriteConfig{
		Enabled:         true,
		DefaultStrategy: multiwrite.StrategyBestEffort,
		Timeout:         30 * time.Second,
		MaxRetries:      3,
		RetryBackoff:    time.Second,
		Groups:          make(map[string]*multiwrite.ClusterGroup),
		Rules:           make([]*multiwrite.WriteRule, 0),
	}
	multiwriteManager, err = multiwrite.NewManager(mwConfig, topology, logger)
	if err != nil {
		logger.Warn("Failed to initialize multi-write manager", zap.Error(err))
	} else {
		logger.Info("Multi-write manager initialized")
	}

	// Initialize blue/green deployment manager
	bgConfig := &bluegreen.DeploymentConfig{
		Enabled:         true,
		DefaultStrategy: bluegreen.StrategyPercentage,
		Environments:    make(map[string]*bluegreen.Environment),
		HealthChecks:    &bluegreen.HealthCheckConfig{Enabled: true, Interval: 30 * time.Second, Timeout: 5 * time.Second},
		Failover:        &bluegreen.FailoverConfig{Enabled: true},
	}
	deploymentManager, err = bluegreen.NewDeploymentManager(bgConfig, multiwriteManager, topology, logger)
	if err != nil {
		logger.Warn("Failed to initialize deployment manager", zap.Error(err))
	} else {
		logger.Info("Blue/green deployment manager initialized")
	}

	metrics.InitMetrics()

	var wg sync.WaitGroup
	proxies := make(map[string]net.Listener)

	if cfg.MySQLEnabled {
		listener, err := startMySQLProxy(ctx, cfg, redisClient, &wg)
		if err != nil {
			logger.Error("Failed to start MySQL proxy", zap.Error(err))
		} else {
			proxies["mysql"] = listener
		}
	}

	if cfg.PostgreSQLEnabled {
		listener, err := startPostgreSQLProxy(ctx, cfg, redisClient, &wg)
		if err != nil {
			logger.Error("Failed to start PostgreSQL proxy", zap.Error(err))
		} else {
			proxies["postgresql"] = listener
		}
	}

	if cfg.MSSQLEnabled {
		listener, err := startMSSQLProxy(ctx, cfg, redisClient, &wg)
		if err != nil {
			logger.Error("Failed to start MSSQL proxy", zap.Error(err))
		} else {
			proxies["mssql"] = listener
		}
	}

	if cfg.MongoDBEnabled {
		listener, err := startMongoDBProxy(ctx, cfg, redisClient, &wg)
		if err != nil {
			logger.Error("Failed to start MongoDB proxy", zap.Error(err))
		} else {
			proxies["mongodb"] = listener
		}
	}

	if cfg.RedisProxyEnabled {
		listener, err := startRedisProxy(ctx, cfg, redisClient, &wg)
		if err != nil {
			logger.Error("Failed to start Redis proxy", zap.Error(err))
		} else {
			proxies["redis"] = listener
		}
	}

	go startConfigSync(ctx, redisClient, cfg)

	// Start XDP rule synchronization
	if xdpController != nil {
		go startXDPRuleSync(ctx, redisClient, xdpController)
	}

	// Start deployment monitoring
	go startDeploymentMonitoring(ctx, redisClient, deploymentManager)

	go startMetricsServer(cfg.MetricsPort)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down ArticDBM Proxy")
	cancel()

	// Graceful shutdown of XDP resources
	if xdpController != nil {
		xdpController.Close()
	}
	if afxdpManager != nil {
		afxdpManager.Close()
	}

	for name, listener := range proxies {
		logger.Info("Closing listener", zap.String("proxy", name))
		listener.Close()
	}

	wg.Wait()
	logger.Info("ArticDBM Proxy stopped")
}

func startMySQLProxy(ctx context.Context, cfg *config.Config, redis *redis.Client, wg *sync.WaitGroup) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.MySQLPort))
	if err != nil {
		return nil, err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		handler := handlers.NewMySQLHandler(cfg, redis, logger, xdpController, cacheManager, multiwriteManager)
		handler.Start(ctx, listener)
	}()

	logger.Info("MySQL proxy started", zap.Int("port", cfg.MySQLPort))
	return listener, nil
}

func startPostgreSQLProxy(ctx context.Context, cfg *config.Config, redis *redis.Client, wg *sync.WaitGroup) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.PostgreSQLPort))
	if err != nil {
		return nil, err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		handler := handlers.NewPostgreSQLHandler(cfg, redis, logger)
		handler.Start(ctx, listener)
	}()

	logger.Info("PostgreSQL proxy started", zap.Int("port", cfg.PostgreSQLPort))
	return listener, nil
}

func startMSSQLProxy(ctx context.Context, cfg *config.Config, redis *redis.Client, wg *sync.WaitGroup) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.MSSQLPort))
	if err != nil {
		return nil, err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		handler := handlers.NewMSSQLHandler(cfg, redis, logger)
		handler.Start(ctx, listener)
	}()

	logger.Info("MSSQL proxy started", zap.Int("port", cfg.MSSQLPort))
	return listener, nil
}

func startMongoDBProxy(ctx context.Context, cfg *config.Config, redis *redis.Client, wg *sync.WaitGroup) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.MongoDBPort))
	if err != nil {
		return nil, err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		handler := handlers.NewMongoDBHandler(cfg, redis, logger)
		handler.Start(ctx, listener)
	}()

	logger.Info("MongoDB proxy started", zap.Int("port", cfg.MongoDBPort))
	return listener, nil
}

func startRedisProxy(ctx context.Context, cfg *config.Config, redis *redis.Client, wg *sync.WaitGroup) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.RedisProxyPort))
	if err != nil {
		return nil, err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		handler := handlers.NewRedisProxyHandler(cfg, redis, logger)
		handler.Start(ctx, listener)
	}()

	logger.Info("Redis proxy started", zap.Int("port", cfg.RedisProxyPort))
	return listener, nil
}

func startConfigSync(ctx context.Context, redisClient *redis.Client, cfg *config.Config) {
	ticker := time.NewTicker(time.Duration(45+time.Now().Unix()%30) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := cfg.RefreshFromRedis(ctx, redisClient); err != nil {
				logger.Error("Failed to refresh config from Redis", zap.Error(err))
			} else {
				logger.Debug("Config refreshed from Redis")
			}
			ticker.Reset(time.Duration(45+time.Now().Unix()%30) * time.Second)
		}
	}
}

func startXDPRuleSync(ctx context.Context, redisClient *redis.Client, controller *xdp.Controller) {
	pubsub := redisClient.Subscribe(ctx, "articdbm:xdp:rule_update")
	defer pubsub.Close()

	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if err := controller.ProcessRuleUpdate(msg.Payload); err != nil {
				logger.Error("Failed to process XDP rule update", zap.Error(err))
			}
		}
	}
}

func startDeploymentMonitoring(ctx context.Context, redisClient *redis.Client, manager *bluegreen.DeploymentManager) {
	pubsub := redisClient.Subscribe(ctx, "articdbm:deployment:update", "articdbm:deployment:rollback")
	defer pubsub.Close()

	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if err := manager.ProcessDeploymentEvent(msg.Channel, msg.Payload); err != nil {
				logger.Error("Failed to process deployment event", zap.Error(err))
			}
		}
	}
}

func startMetricsServer(port int) {
	router := mux.NewRouter()
	router.Handle("/metrics", promhttp.Handler())
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	logger.Info("Metrics server started", zap.Int("port", port))
	if err := server.ListenAndServe(); err != nil {
		logger.Error("Metrics server failed", zap.Error(err))
	}
}