package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Config holds the proxy configuration
type Config struct {
	// Server
	ListenAddr string
	ListenPort int

	// gRPC API
	GRPCAddr string
	GRPCPort int

	// Metrics/Health
	MetricsAddr string
	MetricsPort int

	// Pool
	MaxConnectionsPerRoute int
	MaxConnectionsPerHost  int

	// Rate limiting
	DefaultConnectionRate int
	DefaultQueryRate      int

	// Redis config watch
	RedisAddr   string
	RedisPort   int
	RedisDB     int
	RedisPrefix string

	// Performance/XDP
	XDPEnabled bool

	logger *zap.Logger
	mu     sync.RWMutex
}

// NewConfig creates a new config from environment variables
func NewConfig(logger *zap.Logger) (*Config, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	cfg := &Config{
		ListenAddr:             getEnv("DBPROXY_LISTEN_ADDR", "0.0.0.0"),
		ListenPort:             getEnvInt("DBPROXY_LISTEN_PORT", 5432),
		GRPCAddr:               getEnv("DBPROXY_GRPC_ADDR", "0.0.0.0"),
		GRPCPort:               getEnvInt("DBPROXY_GRPC_PORT", 50051),
		MetricsAddr:            getEnv("DBPROXY_METRICS_ADDR", "0.0.0.0:9090"),
		MetricsPort:            getEnvInt("DBPROXY_METRICS_PORT", 9090),
		MaxConnectionsPerRoute: getEnvInt("DBPROXY_MAX_CONNS_PER_ROUTE", 10),
		MaxConnectionsPerHost:  getEnvInt("DBPROXY_MAX_CONNS_PER_HOST", 100),
		DefaultConnectionRate:  getEnvInt("DBPROXY_CONN_RATE_LIMIT", 100),
		DefaultQueryRate:       getEnvInt("DBPROXY_QUERY_RATE_LIMIT", 1000),
		RedisAddr:              getEnv("DBPROXY_REDIS_ADDR", "localhost"),
		RedisPort:              getEnvInt("DBPROXY_REDIS_PORT", 6379),
		RedisDB:                getEnvInt("DBPROXY_REDIS_DB", 0),
		RedisPrefix:            getEnv("DBPROXY_REDIS_PREFIX", "nest:dbproxy"),
		XDPEnabled:             getEnvBool("DBPROXY_XDP_ENABLED", false),
		logger:                 logger,
	}

	logger.Info("configuration loaded",
		zap.String("listen_addr", cfg.ListenAddr),
		zap.Int("listen_port", cfg.ListenPort),
		zap.String("redis_addr", cfg.RedisAddr),
		zap.Bool("xdp_enabled", cfg.XDPEnabled),
	)

	return cfg, nil
}

// GetRedisClient returns a Redis client for config watching
func (c *Config) GetRedisClient(ctx context.Context) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", c.RedisAddr, c.RedisPort),
		DB:   c.RedisDB,
	})

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	c.logger.Info("Redis client connected",
		zap.String("addr", client.Options().Addr),
	)

	return client, nil
}

// RouterConfig represents routing configuration
type RouterConfig struct {
	Routes map[string]*Route `json:"routes"`
}

// Route represents a single route with primary and optional replicas
type Route struct {
	Protocol       string           `json:"protocol"`
	Tenant         string           `json:"tenant"`
	Primary        *RouteEndpoint   `json:"primary"`         // Primary backend (write)
	Replicas       []*RouteEndpoint `json:"replicas"`        // Replica backends (read)
	MaxConnections int              `json:"max_connections"` // Deprecated: per-endpoint max_connections now
}

// RouteEndpoint represents a specific backend endpoint (primary or replica)
type RouteEndpoint struct {
	Name           string `json:"name"`    // "primary", "replica-1", etc.
	Backend        string `json:"backend"` // hostname or IP
	Port           int    `json:"port"`
	MaxConnections int    `json:"max_connections"` // per-endpoint limit
}

// SecurityConfig represents security/blocking configuration
type SecurityConfig struct {
	BlockedResources []string `json:"blocked_resources"`
	AllowedResources []string `json:"allowed_resources"`
	EnableInjection  bool     `json:"enable_injection_check"`
}

// CacheConfig represents caching configuration
type CacheConfig struct {
	Enabled   bool `json:"enabled"`
	TTLSecs   int  `json:"ttl_secs"`
	MaxSizeKB int  `json:"max_size_kb"`
}

// LoadRouterConfigFromRedis loads router config from Redis
func (c *Config) LoadRouterConfigFromRedis(ctx context.Context, client *redis.Client) (*RouterConfig, error) {
	key := fmt.Sprintf("%s:routes", c.RedisPrefix)

	val, err := client.Get(ctx, key).Result()
	if err == redis.Nil {
		c.logger.Debug("no routes config in Redis", zap.String("key", key))
		return &RouterConfig{Routes: make(map[string]*Route)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load routes from Redis: %w", err)
	}

	var cfg RouterConfig
	if err := json.Unmarshal([]byte(val), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse routes config: %w", err)
	}

	c.logger.Debug("routes loaded from Redis", zap.Int("count", len(cfg.Routes)))
	return &cfg, nil
}

// LoadSecurityConfigFromRedis loads security config from Redis
func (c *Config) LoadSecurityConfigFromRedis(ctx context.Context, client *redis.Client) (*SecurityConfig, error) {
	key := fmt.Sprintf("%s:security", c.RedisPrefix)

	val, err := client.Get(ctx, key).Result()
	if err == redis.Nil {
		c.logger.Debug("no security config in Redis", zap.String("key", key))
		return &SecurityConfig{EnableInjection: true}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load security config: %w", err)
	}

	var cfg SecurityConfig
	if err := json.Unmarshal([]byte(val), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse security config: %w", err)
	}

	c.logger.Debug("security config loaded from Redis",
		zap.Bool("injection_check", cfg.EnableInjection),
		zap.Int("blocked_resources", len(cfg.BlockedResources)),
	)
	return &cfg, nil
}

// SaveRouterConfigToRedis saves router config to Redis (for manager to call)
func (c *Config) SaveRouterConfigToRedis(ctx context.Context, client *redis.Client, cfg *RouterConfig) error {
	key := fmt.Sprintf("%s:routes", c.RedisPrefix)

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal routes config: %w", err)
	}

	if err := client.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to save routes to Redis: %w", err)
	}

	c.logger.Debug("routes saved to Redis", zap.String("key", key))
	return nil
}

// Helper functions
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1" || val == "yes"
	}
	return defaultVal
}
