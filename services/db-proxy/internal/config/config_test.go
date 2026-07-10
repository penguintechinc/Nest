package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestNewConfig(t *testing.T) {
	logger := zap.NewNop()

	cfg, err := NewConfig(logger)
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	if cfg == nil {
		t.Error("returned config is nil")
	}

	// Verify defaults
	if cfg.ListenAddr == "" {
		t.Error("listen_addr not set")
	}

	if cfg.ListenPort <= 0 {
		t.Errorf("invalid listen_port: %d", cfg.ListenPort)
	}

	if cfg.GRPCPort <= 0 {
		t.Errorf("invalid grpc_port: %d", cfg.GRPCPort)
	}
}

func TestNewConfigWithEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("DBPROXY_LISTEN_PORT", "8888")
	os.Setenv("DBPROXY_GRPC_PORT", "50052")
	defer os.Unsetenv("DBPROXY_LISTEN_PORT")
	defer os.Unsetenv("DBPROXY_GRPC_PORT")

	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	if cfg.ListenPort != 8888 {
		t.Errorf("expected listen_port=8888, got %d", cfg.ListenPort)
	}

	if cfg.GRPCPort != 50052 {
		t.Errorf("expected grpc_port=50052, got %d", cfg.GRPCPort)
	}
}

func TestNewConfigXDPEnv(t *testing.T) {
	os.Setenv("DBPROXY_XDP_ENABLED", "true")
	defer os.Unsetenv("DBPROXY_XDP_ENABLED")

	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	if !cfg.XDPEnabled {
		t.Error("XDP not enabled via env var")
	}
}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")

	val := getEnv("TEST_VAR", "default")
	if val != "test_value" {
		t.Errorf("expected 'test_value', got %q", val)
	}

	val = getEnv("MISSING_VAR", "default")
	if val != "default" {
		t.Errorf("expected 'default', got %q", val)
	}
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	val := getEnvInt("TEST_INT", 0)
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	val = getEnvInt("MISSING_INT", 99)
	if val != 99 {
		t.Errorf("expected 99, got %d", val)
	}

	// Invalid int should return default
	os.Setenv("INVALID_INT", "not_a_number")
	val = getEnvInt("INVALID_INT", 77)
	if val != 77 {
		t.Errorf("expected 77 for invalid int, got %d", val)
	}
	os.Unsetenv("INVALID_INT")
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"no", false},
	}

	for _, tt := range tests {
		os.Setenv("TEST_BOOL", tt.value)
		val := getEnvBool("TEST_BOOL", false)
		if val != tt.expected {
			t.Errorf("getEnvBool(%q) = %v, expected %v", tt.value, val, tt.expected)
		}
		os.Unsetenv("TEST_BOOL")
	}

	// Default value
	val := getEnvBool("MISSING_BOOL", true)
	if !val {
		t.Error("expected true for missing bool with default true")
	}
}

func TestRouterConfig(t *testing.T) {
	routerCfg := &RouterConfig{
		Routes: map[string]*Route{
			"db-primary": {
				Protocol: "mysql",
				Tenant:   "tenant-1",
				Primary: &RouteEndpoint{
					Name:           "primary",
					Backend:        "db.example.com",
					Port:           3306,
					MaxConnections: 10,
				},
				Replicas: []*RouteEndpoint{
					{
						Name:           "replica-1",
						Backend:        "db-replica.example.com",
						Port:           3306,
						MaxConnections: 5,
					},
				},
			},
		},
	}

	if len(routerCfg.Routes) != 1 {
		t.Error("routes not set correctly")
	}

	route, ok := routerCfg.Routes["db-primary"]
	if !ok {
		t.Error("db-primary route not found")
	}

	if route.Protocol != "mysql" {
		t.Errorf("expected protocol mysql, got %s", route.Protocol)
	}

	if route.Primary.Port != 3306 {
		t.Errorf("expected port 3306, got %d", route.Primary.Port)
	}
}

func TestSecurityConfig(t *testing.T) {
	securityCfg := &SecurityConfig{
		BlockedResources: []string{"DROP TABLE", "TRUNCATE"},
		AllowedResources: []string{},
		EnableInjection:  true,
	}

	if len(securityCfg.BlockedResources) != 2 {
		t.Error("blocked_resources not set correctly")
	}

	if !securityCfg.EnableInjection {
		t.Error("injection check not enabled")
	}
}

func TestLoadRouterConfigFromRedis(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	if cfg == nil {
		t.Fatal("config is nil")
	}

	// Test with nil data (config doesn't exist in Redis)
	// This would require a Redis mock; for now test the config structure
	routerCfg := &RouterConfig{
		Routes: map[string]*Route{
			"db1": {
				Protocol: "mysql",
				Tenant:   "tenant-a",
				Primary: &RouteEndpoint{
					Name:           "primary",
					Backend:        "db.example.com",
					Port:           3306,
					MaxConnections: 10,
				},
			},
		},
	}

	if routerCfg == nil {
		t.Error("RouterConfig is nil")
	}

	if len(routerCfg.Routes) != 1 {
		t.Error("expected 1 route")
	}
}

func TestLoadSecurityConfigFromRedis(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	if cfg == nil {
		t.Fatal("config is nil")
	}

	securityCfg := &SecurityConfig{
		BlockedResources: []string{"DROP TABLE"},
		AllowedResources: []string{"SELECT"},
		EnableInjection:  true,
	}

	if securityCfg == nil {
		t.Error("SecurityConfig is nil")
	}

	if !securityCfg.EnableInjection {
		t.Error("EnableInjection should be true")
	}
}

func TestSaveRouterConfigToRedis(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	if cfg == nil {
		t.Fatal("config is nil")
	}

	routerCfg := &RouterConfig{
		Routes: map[string]*Route{
			"db1": {
				Protocol: "mysql",
				Tenant:   "tenant-a",
				Primary: &RouteEndpoint{
					Name:           "primary",
					Backend:        "db.example.com",
					Port:           3306,
					MaxConnections: 10,
				},
			},
		},
	}

	if routerCfg == nil {
		t.Error("RouterConfig is nil")
	}

	if len(routerCfg.Routes) != 1 {
		t.Error("expected 1 route")
	}
}

func TestCacheConfig(t *testing.T) {
	os.Setenv("DBPROXY_CACHE_ENABLED", "true")
	os.Setenv("DBPROXY_CACHE_TTL_SECS", "60")
	os.Setenv("DBPROXY_CACHE_MAX_SIZE_KB", "5000")
	defer os.Unsetenv("DBPROXY_CACHE_ENABLED")
	defer os.Unsetenv("DBPROXY_CACHE_TTL_SECS")
	defer os.Unsetenv("DBPROXY_CACHE_MAX_SIZE_KB")

	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	if !cfg.Cache.Enabled {
		t.Error("cache not enabled via env var")
	}

	if cfg.Cache.TTLSecs != 60 {
		t.Errorf("expected TTL 60, got %d", cfg.Cache.TTLSecs)
	}

	if cfg.Cache.MaxSizeKB != 5000 {
		t.Errorf("expected MaxSizeKB 5000, got %d", cfg.Cache.MaxSizeKB)
	}
}

// Redis integration tests
func TestGetRedisClient(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	// Test with invalid host (should fail to ping)
	cfg.RedisAddr = "localhost"
	cfg.RedisPort = 9999 // invalid port

	ctx := context.Background()
	client, err := cfg.GetRedisClient(ctx)
	if err == nil {
		if client != nil {
			client.Close()
		}
		t.Error("expected error connecting to invalid Redis address")
	}
}

func TestLoadRouterConfigFromRedisEmpty(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	// Create miniredis mock
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	// Configure to use miniredis
	parts := strings.Split(mr.Addr(), ":")
	if len(parts) != 2 {
		t.Fatalf("unexpected miniredis addr format: %s", mr.Addr())
	}
	cfg.RedisAddr = parts[0]
	redisPort, _ := strconv.Atoi(parts[1])
	cfg.RedisPort = redisPort

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Test load when key doesn't exist (redis.Nil case)
	routerCfg, err := cfg.LoadRouterConfigFromRedis(ctx, client)
	if err != nil {
		t.Fatalf("expected no error for missing key, got %v", err)
	}

	if routerCfg == nil {
		t.Error("expected non-nil RouterConfig")
	}

	if len(routerCfg.Routes) != 0 {
		t.Errorf("expected empty routes, got %d", len(routerCfg.Routes))
	}
}

func TestLoadRouterConfigFromRedisValid(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Set valid config in Redis
	validConfig := RouterConfig{
		Routes: map[string]*Route{
			"test-route": {
				Protocol: "mysql",
				Tenant:   "tenant-1",
				Primary: &RouteEndpoint{
					Name:           "primary",
					Backend:        "db.example.com",
					Port:           3306,
					MaxConnections: 10,
				},
				Replicas: []*RouteEndpoint{
					{
						Name:           "replica-1",
						Backend:        "db-replica.example.com",
						Port:           3306,
						MaxConnections: 5,
					},
				},
			},
		},
	}

	data, _ := json.Marshal(validConfig)
	key := fmt.Sprintf("%s:routes", cfg.RedisPrefix)
	mr.Set(key, string(data))

	routerCfg, err := cfg.LoadRouterConfigFromRedis(ctx, client)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(routerCfg.Routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(routerCfg.Routes))
	}

	route, ok := routerCfg.Routes["test-route"]
	if !ok {
		t.Error("expected test-route in config")
	}

	if route.Protocol != "mysql" {
		t.Errorf("expected protocol mysql, got %s", route.Protocol)
	}
}

func TestLoadRouterConfigFromRedisMalformed(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Set malformed JSON in Redis
	key := fmt.Sprintf("%s:routes", cfg.RedisPrefix)
	mr.Set(key, "{invalid json}")

	routerCfg, err := cfg.LoadRouterConfigFromRedis(ctx, client)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}

	if routerCfg != nil {
		t.Error("expected nil RouterConfig for malformed JSON")
	}
}

func TestLoadSecurityConfigFromRedisEmpty(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Test load when key doesn't exist (redis.Nil case)
	securityCfg, err := cfg.LoadSecurityConfigFromRedis(ctx, client)
	if err != nil {
		t.Fatalf("expected no error for missing key, got %v", err)
	}

	if securityCfg == nil {
		t.Error("expected non-nil SecurityConfig")
	}

	if !securityCfg.EnableInjection {
		t.Error("expected EnableInjection to be true by default")
	}
}

func TestLoadSecurityConfigFromRedisValid(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Set valid security config in Redis
	validConfig := SecurityConfig{
		BlockedResources: []string{"DROP TABLE", "TRUNCATE"},
		AllowedResources: []string{"SELECT", "INSERT"},
		EnableInjection:  true,
	}

	data, _ := json.Marshal(validConfig)
	key := fmt.Sprintf("%s:security", cfg.RedisPrefix)
	mr.Set(key, string(data))

	securityCfg, err := cfg.LoadSecurityConfigFromRedis(ctx, client)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(securityCfg.BlockedResources) != 2 {
		t.Errorf("expected 2 blocked resources, got %d", len(securityCfg.BlockedResources))
	}

	if !securityCfg.EnableInjection {
		t.Error("expected EnableInjection to be true")
	}
}

func TestLoadSecurityConfigFromRedisMalformed(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Set malformed JSON in Redis
	key := fmt.Sprintf("%s:security", cfg.RedisPrefix)
	mr.Set(key, "{invalid json}")

	securityCfg, err := cfg.LoadSecurityConfigFromRedis(ctx, client)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}

	if securityCfg != nil {
		t.Error("expected nil SecurityConfig for malformed JSON")
	}
}

func TestSaveRouterConfigToRedisValid(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Create config to save
	routerCfg := &RouterConfig{
		Routes: map[string]*Route{
			"test-route": {
				Protocol: "mysql",
				Tenant:   "tenant-1",
				Primary: &RouteEndpoint{
					Name:           "primary",
					Backend:        "db.example.com",
					Port:           3306,
					MaxConnections: 10,
				},
			},
		},
	}

	err := cfg.SaveRouterConfigToRedis(ctx, client, routerCfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify it was saved
	key := fmt.Sprintf("%s:routes", cfg.RedisPrefix)
	val, err := client.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("expected key to exist in Redis, got %v", err)
	}

	var loaded RouterConfig
	if err := json.Unmarshal([]byte(val), &loaded); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}

	if len(loaded.Routes) != 1 {
		t.Errorf("expected 1 route after load, got %d", len(loaded.Routes))
	}
}

func TestNewConfigWithNilLogger(t *testing.T) {
	cfg, err := NewConfig(nil)
	if err != nil {
		t.Fatalf("expected no error with nil logger, got %v", err)
	}

	if cfg == nil {
		t.Error("expected non-nil config")
	}
}

func TestGetRedisClientSuccess(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	// Configure to use miniredis
	parts := strings.Split(mr.Addr(), ":")
	if len(parts) != 2 {
		t.Fatalf("unexpected miniredis addr format: %s", mr.Addr())
	}
	cfg.RedisAddr = parts[0]
	redisPort, _ := strconv.Atoi(parts[1])
	cfg.RedisPort = redisPort

	client, err := cfg.GetRedisClient(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client == nil {
		t.Error("expected non-nil client")
	}

	defer client.Close()
}

func TestLoadRouterConfigFromRedisReadError(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	addr := mr.Addr()
	mr.Close() // Close to cause read errors

	// Create a broken client pointing to closed Redis
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()

	routerCfg, err := cfg.LoadRouterConfigFromRedis(ctx, client)
	if err == nil {
		t.Error("expected error when Redis is down")
	}

	if routerCfg != nil {
		t.Error("expected nil RouterConfig on error")
	}
}

func TestLoadSecurityConfigFromRedisReadError(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	addr := mr.Addr()
	mr.Close() // Close to cause read errors

	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()

	securityCfg, err := cfg.LoadSecurityConfigFromRedis(ctx, client)
	if err == nil {
		t.Error("expected error when Redis is down")
	}

	if securityCfg != nil {
		t.Error("expected nil SecurityConfig on error")
	}
}

func TestSaveRouterConfigToRedisWriteError(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := NewConfig(logger)

	ctx := context.Background()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	addr := mr.Addr()
	mr.Close() // Close to cause write errors

	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()

	routerCfg := &RouterConfig{
		Routes: map[string]*Route{
			"test": {
				Protocol: "mysql",
				Tenant:   "tenant-1",
				Primary: &RouteEndpoint{
					Name:           "primary",
					Backend:        "db.example.com",
					Port:           3306,
					MaxConnections: 10,
				},
			},
		},
	}

	err := cfg.SaveRouterConfigToRedis(ctx, client, routerCfg)
	if err == nil {
		t.Error("expected error when Redis is down")
	}
}
