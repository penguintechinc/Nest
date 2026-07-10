package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/penguintechinc/nest/services/db-proxy/internal/config"
)

// TestRedisConfigLoading tests that Redis config is properly loaded
// Note: This requires a running Redis instance at localhost:6379
func TestRedisConfigLoading(t *testing.T) {
	t.Skip("requires redis-server running on localhost:6379")

	cfg := &config.Config{
		RedisAddr:   "localhost",
		RedisPort:   6379,
		RedisDB:     0,
		RedisPrefix: "nest:dbproxy:test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get Redis client
	redisClient, err := cfg.GetRedisClient(ctx)
	if err != nil {
		t.Fatalf("failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// Set test config in Redis
	testRoutes := &config.RouterConfig{
		Routes: map[string]*config.Route{
			"test-route": {
				Protocol: "mysql",
				Tenant:   "test-tenant",
				Primary: &config.RouteEndpoint{
					Name:           "primary",
					Backend:        "test.example.com",
					Port:           3306,
					MaxConnections: 20,
				},
				Replicas: []*config.RouteEndpoint{},
			},
		},
	}

	routesData, _ := json.Marshal(testRoutes)
	redisKey := cfg.RedisPrefix + ":routes"
	if err := redisClient.Set(ctx, redisKey, routesData, 0).Err(); err != nil {
		t.Fatalf("failed to set config in Redis: %v", err)
	}

	defer redisClient.Del(ctx, redisKey) // cleanup

	// Load config from Redis
	loadedRoutes, err := cfg.LoadRouterConfigFromRedis(ctx, redisClient)
	if err != nil {
		t.Fatalf("failed to load routes from Redis: %v", err)
	}

	if len(loadedRoutes.Routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(loadedRoutes.Routes))
	}

	route, ok := loadedRoutes.Routes["test-route"]
	if !ok {
		t.Fatal("test-route not found in loaded config")
	}

	if route.Protocol != "mysql" {
		t.Errorf("expected protocol mysql, got %s", route.Protocol)
	}

	if route.Primary.Backend != "test.example.com" {
		t.Errorf("expected backend test.example.com, got %s", route.Primary.Backend)
	}

	if route.Primary.Port != 3306 {
		t.Errorf("expected port 3306, got %d", route.Primary.Port)
	}
}

// TestRedisSecurityConfigLoading tests security config loading
func TestRedisSecurityConfigLoading(t *testing.T) {
	t.Skip("requires redis-server running on localhost:6379")

	cfg := &config.Config{
		RedisAddr:   "localhost",
		RedisPort:   6379,
		RedisDB:     0,
		RedisPrefix: "nest:dbproxy:test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisClient, err := cfg.GetRedisClient(ctx)
	if err != nil {
		t.Fatalf("failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// Set test security config
	testSecurity := &config.SecurityConfig{
		BlockedResources: []string{"DROP TABLE", "TRUNCATE"},
		AllowedResources: []string{},
		EnableInjection:  true,
	}

	secData, _ := json.Marshal(testSecurity)
	redisKey := cfg.RedisPrefix + ":security"
	if err := redisClient.Set(ctx, redisKey, secData, 0).Err(); err != nil {
		t.Fatalf("failed to set config in Redis: %v", err)
	}

	defer redisClient.Del(ctx, redisKey)

	// Load config from Redis
	loadedSec, err := cfg.LoadSecurityConfigFromRedis(ctx, redisClient)
	if err != nil {
		t.Fatalf("failed to load security config from Redis: %v", err)
	}

	if !loadedSec.EnableInjection {
		t.Error("expected EnableInjection to be true")
	}

	if len(loadedSec.BlockedResources) != 2 {
		t.Errorf("expected 2 blocked resources, got %d", len(loadedSec.BlockedResources))
	}
}

// TestRedisConfigPubSub tests Pub/Sub notifications
func TestRedisConfigPubSub(t *testing.T) {
	t.Skip("requires redis-server running on localhost:6379")

	cfg := &config.Config{
		RedisAddr:   "localhost",
		RedisPort:   6379,
		RedisDB:     0,
		RedisPrefix: "nest:dbproxy:test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisClient, err := cfg.GetRedisClient(ctx)
	if err != nil {
		t.Fatalf("failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	channel := cfg.RedisPrefix + ":config:updated"

	// Subscribe in a goroutine
	pubsub := redisClient.Subscribe(ctx, channel)
	defer pubsub.Close()

	// Publish a message in another goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		if err := redisClient.Publish(ctx, channel, "routes").Err(); err != nil {
			t.Logf("failed to publish: %v", err)
		}
	}()

	// Receive message
	msg, err := pubsub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("failed to receive message: %v", err)
	}

	if msg.Payload != "routes" {
		t.Errorf("expected payload 'routes', got %s", msg.Payload)
	}
}
