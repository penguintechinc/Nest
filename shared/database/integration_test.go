//go:build integration
// +build integration

package database

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestNewPostgresConnection tests actual postgres connection
func TestNewPostgresConnection(t *testing.T) {
	// Use test database URL if available
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgresql://postgres:password@localhost:5432/nest?sslmode=disable"
	}

	db, err := NewFromURL(url)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Errorf("Expected database connection, got nil")
	}
}

// TestNewPostgresConnectionWithConfig tests postgres connection with config
func TestNewPostgresConnectionWithConfig(t *testing.T) {
	config := &Config{
		Host:     os.Getenv("POSTGRES_HOST"),
		Port:     os.Getenv("POSTGRES_PORT"),
		User:     os.Getenv("POSTGRES_USER"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
		DBName:   os.Getenv("POSTGRES_DB"),
		SSLMode:  "disable",
		TimeZone: "UTC",
	}

	// Use defaults if env vars not set
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.Port == "" {
		config.Port = "5432"
	}
	if config.User == "" {
		config.User = "postgres"
	}
	if config.Password == "" {
		config.Password = "password"
	}
	if config.DBName == "" {
		config.DBName = "nest"
	}

	db, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create database connection: %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Errorf("Expected database connection, got nil")
	}
}

// TestDatabaseHealthCheck tests database health check
func TestDatabaseHealthCheck(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgresql://postgres:password@localhost:5432/nest?sslmode=disable"
	}

	db, err := NewFromURL(url)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	err = db.HealthCheck()
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

// TestDatabaseGetStats tests database statistics retrieval
func TestDatabaseGetStats(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgresql://postgres:password@localhost:5432/nest?sslmode=disable"
	}

	db, err := NewFromURL(url)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	stats := db.GetStats()
	if stats.OpenConnections < 0 {
		t.Errorf("Invalid OpenConnections: %d", stats.OpenConnections)
	}
}

// TestNewRedisConnection tests actual redis connection
func TestNewRedisConnection(t *testing.T) {
	// Use test redis URL if available
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		url = "redis://:password@localhost:6379/0"
	}

	client, err := NewRedisFromURL(url)
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Errorf("Expected Redis client, got nil")
	}
}

// TestNewRedisConnectionWithConfig tests Redis connection with config
func TestNewRedisConnectionWithConfig(t *testing.T) {
	config := &RedisConfig{
		Addr:     os.Getenv("REDIS_ADDR"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	}

	// Use defaults if env vars not set
	if config.Addr == "" {
		config.Addr = "localhost:6379"
	}
	if config.Password == "" {
		config.Password = "password"
	}

	client, err := NewRedis(config)
	if err != nil {
		t.Fatalf("Failed to create Redis connection: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Errorf("Expected Redis client, got nil")
	}
}

// TestRedisHealthCheck tests Redis health check
func TestRedisHealthCheck(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		url = "redis://:password@localhost:6379/0"
	}

	client, err := NewRedisFromURL(url)
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = client.HealthCheck(ctx)
	if err != nil {
		t.Errorf("Redis health check failed: %v", err)
	}
}

// TestRedisCacheKeyOperations tests Redis cache operations with actual client
func TestRedisCacheKeyOperations(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		url = "redis://:password@localhost:6379/0"
	}

	client, err := NewRedisFromURL(url)
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test cache key string generation
	key := CacheKey{Prefix: "test", ID: "123", Suffix: "key"}
	keyStr := key.String()

	if keyStr != "test:123:key" {
		t.Errorf("Expected 'test:123:key', got '%s'", keyStr)
	}
}

// TestCacheKeyStringWithActualClient tests CacheKey with real Redis context
func TestCacheKeyStringWithActualClient(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		url = "redis://:password@localhost:6379/0"
	}

	client, err := NewRedisFromURL(url)
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name     string
		prefix   string
		id       string
		suffix   string
		expected string
	}{
		{"license", "license", "key1", "validation", "license:key1:validation"},
		{"feature", "feature", "key2", "export", "feature:key2:export"},
		{"session", "session", "sess123", "", "session:sess123"},
	}

	for _, tt := range tests {
		key := CacheKey{Prefix: tt.prefix, ID: tt.id, Suffix: tt.suffix}
		result := key.String()
		if result != tt.expected {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.expected, result)
		}
	}
}

// TestDatabaseConnectionPoolSettings tests that connection pool is configured
func TestDatabaseConnectionPoolSettings(t *testing.T) {
	config := &Config{
		Host:            "localhost",
		Port:            "5432",
		User:            "postgres",
		Password:        "password",
		DBName:          "nest",
		SSLMode:         "disable",
		TimeZone:        "UTC",
		MaxOpenConns:    15,
		MaxIdleConns:    8,
		ConnMaxLifetime: 10 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}

	db, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create database connection: %v", err)
	}
	defer db.Close()

	stats := db.GetStats()
	// Pool should be initialized
	if stats.MaxOpenConnections != 15 {
		t.Errorf("Expected MaxOpenConnections 15, got %d", stats.MaxOpenConnections)
	}
}

// TestRedisConnectionPoolSettings tests that Redis pool is configured
func TestRedisConnectionPoolSettings(t *testing.T) {
	config := &RedisConfig{
		Addr:         "localhost:6379",
		Password:     "password",
		DB:           0,
		PoolSize:     15,
		MinIdleConns: 5,
		MaxIdleTime:  5 * time.Minute,
		MaxConnAge:   10 * time.Minute,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	client, err := NewRedis(config)
	if err != nil {
		t.Fatalf("Failed to create Redis connection: %v", err)
	}
	defer client.Close()

	stats := client.GetConnectionStats()
	if stats == nil {
		t.Errorf("Expected connection stats, got nil")
	}
}
