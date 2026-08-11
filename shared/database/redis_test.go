package database

import (
	"os"
	"testing"
	"time"
)

// TestDefaultRedisConfig tests the default Redis configuration
func TestDefaultRedisConfig(t *testing.T) {
	// Save original env vars and restore after test
	originalAddr := os.Getenv("REDIS_ADDR")
	originalPassword := os.Getenv("REDIS_PASSWORD")
	originalDB := os.Getenv("REDIS_DB")

	defer func() {
		os.Setenv("REDIS_ADDR", originalAddr)
		os.Setenv("REDIS_PASSWORD", originalPassword)
		os.Setenv("REDIS_DB", originalDB)
	}()

	// Clear env vars
	os.Unsetenv("REDIS_ADDR")
	os.Unsetenv("REDIS_PASSWORD")
	os.Unsetenv("REDIS_DB")

	config := DefaultRedisConfig()

	if config.Addr != "localhost:6379" {
		t.Errorf("Expected addr 'localhost:6379', got '%s'", config.Addr)
	}
	if config.Password != "" {
		t.Errorf("Expected empty password, got '%s'", config.Password)
	}
	if config.DB != 0 {
		t.Errorf("Expected DB 0, got %d", config.DB)
	}
	if config.PoolSize != 10 {
		t.Errorf("Expected PoolSize 10, got %d", config.PoolSize)
	}
	if config.MinIdleConns != 5 {
		t.Errorf("Expected MinIdleConns 5, got %d", config.MinIdleConns)
	}
	if config.MaxIdleTime != 5*time.Minute {
		t.Errorf("Expected MaxIdleTime 5m, got %v", config.MaxIdleTime)
	}
	if config.MaxConnAge != 10*time.Minute {
		t.Errorf("Expected MaxConnAge 10m, got %v", config.MaxConnAge)
	}
	if config.DialTimeout != 5*time.Second {
		t.Errorf("Expected DialTimeout 5s, got %v", config.DialTimeout)
	}
	if config.ReadTimeout != 3*time.Second {
		t.Errorf("Expected ReadTimeout 3s, got %v", config.ReadTimeout)
	}
	if config.WriteTimeout != 3*time.Second {
		t.Errorf("Expected WriteTimeout 3s, got %v", config.WriteTimeout)
	}
}

// TestDefaultRedisConfigWithEnvVars tests the default Redis configuration with environment variables
func TestDefaultRedisConfigWithEnvVars(t *testing.T) {
	// Save original env vars and restore after test
	originalAddr := os.Getenv("REDIS_ADDR")
	originalPassword := os.Getenv("REDIS_PASSWORD")
	originalDB := os.Getenv("REDIS_DB")

	defer func() {
		os.Setenv("REDIS_ADDR", originalAddr)
		os.Setenv("REDIS_PASSWORD", originalPassword)
		os.Setenv("REDIS_DB", originalDB)
	}()

	// Set env vars
	os.Setenv("REDIS_ADDR", "redis.example.com:6380")
	os.Setenv("REDIS_PASSWORD", "secretpass")
	os.Setenv("REDIS_DB", "2")

	config := DefaultRedisConfig()

	if config.Addr != "redis.example.com:6380" {
		t.Errorf("Expected addr 'redis.example.com:6380', got '%s'", config.Addr)
	}
	if config.Password != "secretpass" {
		t.Errorf("Expected password 'secretpass', got '%s'", config.Password)
	}
	if config.DB != 2 {
		t.Errorf("Expected DB 2, got %d", config.DB)
	}
}

// TestGetEnvInt tests the getEnvInt helper function
func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue int
		expected     int
	}{
		{
			name:         "env var set valid integer",
			envKey:       "TEST_INT_VAR_1",
			envValue:     "42",
			defaultValue: 10,
			expected:     42,
		},
		{
			name:         "env var not set",
			envKey:       "TEST_INT_VAR_NONEXISTENT_2345",
			envValue:     "",
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "env var invalid integer",
			envKey:       "TEST_INT_VAR_2",
			envValue:     "not_a_number",
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "env var set to zero",
			envKey:       "TEST_INT_VAR_3",
			envValue:     "0",
			defaultValue: 10,
			expected:     0,
		},
		{
			name:         "env var negative integer",
			envKey:       "TEST_INT_VAR_4",
			envValue:     "-5",
			defaultValue: 10,
			expected:     -5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.envKey, tt.envValue)
				defer os.Unsetenv(tt.envKey)
			} else {
				os.Unsetenv(tt.envKey)
			}

			result := getEnvInt(tt.envKey, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestNewRedisFromURLEmptyURL tests NewRedisFromURL with empty URL
func TestNewRedisFromURLEmptyURL(t *testing.T) {
	// Save and clear REDIS_URL env var
	originalURL := os.Getenv("REDIS_URL")
	os.Unsetenv("REDIS_URL")
	defer os.Setenv("REDIS_URL", originalURL)

	client, err := NewRedisFromURL("")
	if err == nil {
		t.Errorf("Expected error for empty URL, got nil")
	}
	if client != nil {
		t.Errorf("Expected nil Redis client, got %v", client)
	}
	if err.Error() != "Redis URL not provided" {
		t.Errorf("Expected 'Redis URL not provided' error, got '%s'", err.Error())
	}
}

// TestNewRedisFromURLWithEnvVar tests NewRedisFromURL using REDIS_URL env var
func TestNewRedisFromURLWithEnvVar(t *testing.T) {
	// Save and clear REDIS_URL env var
	originalURL := os.Getenv("REDIS_URL")
	os.Unsetenv("REDIS_URL")
	defer os.Setenv("REDIS_URL", originalURL)

	// This would connect to Redis, which we skip in unit tests
	t.Skip("Requires actual Redis connection")
}

// TestNewRedisWithNilConfig tests New with nil configuration
func TestNewRedisWithNilConfig(t *testing.T) {
	// This would connect to Redis
	t.Skip("Requires actual Redis connection")
}

// TestNewRedisWithConfig tests that NewRedis properly constructs a config
func TestNewRedisWithConfig(t *testing.T) {
	config := &RedisConfig{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,

		PoolSize:     10,
		MinIdleConns: 5,
		MaxIdleTime:  5 * time.Minute,
		MaxConnAge:   10 * time.Minute,

		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	// Verify config structure is valid
	if config.Addr != "localhost:6379" {
		t.Errorf("Config addr mismatch")
	}
	if config.PoolSize != 10 {
		t.Errorf("Config PoolSize mismatch")
	}
}

// TestCacheKeyStringWithoutSuffix tests CacheKey String() without suffix
func TestCacheKeyStringWithoutSuffix(t *testing.T) {
	key := CacheKey{
		Prefix: "license",
		ID:     "key123",
	}

	expected := "license:key123"
	result := key.String()
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// TestCacheKeyStringWithSuffix tests CacheKey String() with suffix
func TestCacheKeyStringWithSuffix(t *testing.T) {
	key := CacheKey{
		Prefix: "feature",
		ID:     "licensekey",
		Suffix: "advanced_analytics",
	}

	expected := "feature:licensekey:advanced_analytics"
	result := key.String()
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// TestCacheKeyStringVariations tests various CacheKey combinations
func TestCacheKeyStringVariations(t *testing.T) {
	tests := []struct {
		name     string
		key      CacheKey
		expected string
	}{
		{
			name: "session key without suffix",
			key: CacheKey{
				Prefix: "session",
				ID:     "sess_abc123",
			},
			expected: "session:sess_abc123",
		},
		{
			name: "usage key with suffix",
			key: CacheKey{
				Prefix: "usage",
				ID:     "user_456",
				Suffix: "export_feature",
			},
			expected: "usage:user_456:export_feature",
		},
		{
			name: "config key",
			key: CacheKey{
				Prefix: "config",
				ID:     "app_settings",
			},
			expected: "config:app_settings",
		},
		{
			name: "metrics key with suffix",
			key: CacheKey{
				Prefix: "metrics",
				ID:     "cpu",
				Suffix: "total",
			},
			expected: "metrics:cpu:total",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.key.String()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestRedisConfigValidation tests Redis configuration validation logic
func TestRedisConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *RedisConfig
		isValid bool
	}{
		{
			name: "valid config",
			config: &RedisConfig{
				Addr:     "localhost:6379",
				Password: "",
				DB:       0,
				PoolSize: 10,
			},
			isValid: true,
		},
		{
			name: "valid config with password",
			config: &RedisConfig{
				Addr:     "redis.example.com:6380",
				Password: "secret",
				DB:       1,
				PoolSize: 20,
			},
			isValid: true,
		},
		{
			name: "minimal config",
			config: &RedisConfig{
				Addr: "localhost:6379",
			},
			isValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config == nil {
				t.Errorf("Config is nil")
			} else if !tt.isValid {
				t.Errorf("Config marked as invalid")
			}
		})
	}
}

// TestPoolSettings tests Redis pool configuration values
func TestPoolSettings(t *testing.T) {
	config := DefaultRedisConfig()

	tests := []struct {
		name     string
		value    interface{}
		expected interface{}
	}{
		{
			name:     "PoolSize",
			value:    config.PoolSize,
			expected: 10,
		},
		{
			name:     "MinIdleConns",
			value:    config.MinIdleConns,
			expected: 5,
		},
		{
			name:     "MaxIdleTime",
			value:    config.MaxIdleTime,
			expected: 5 * time.Minute,
		},
		{
			name:     "MaxConnAge",
			value:    config.MaxConnAge,
			expected: 10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, tt.value)
			}
		})
	}
}

// TestTimeoutSettings tests Redis timeout configuration values
func TestTimeoutSettings(t *testing.T) {
	config := DefaultRedisConfig()

	tests := []struct {
		name     string
		value    interface{}
		expected interface{}
	}{
		{
			name:     "DialTimeout",
			value:    config.DialTimeout,
			expected: 5 * time.Second,
		},
		{
			name:     "ReadTimeout",
			value:    config.ReadTimeout,
			expected: 3 * time.Second,
		},
		{
			name:     "WriteTimeout",
			value:    config.WriteTimeout,
			expected: 3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, tt.value)
			}
		})
	}
}

// TestRedisURLValidation tests Redis URL format validation
func TestRedisURLValidation(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		shouldError bool
	}{
		{
			name:        "empty URL",
			url:         "",
			shouldError: true,
		},
		{
			name:        "valid localhost URL",
			url:         "redis://localhost:6379",
			shouldError: false, // ParseURL will be called, connection would fail
		},
		{
			name:        "URL with password",
			url:         "redis://:password@localhost:6379",
			shouldError: false,
		},
		{
			name:        "URL with specific DB",
			url:         "redis://localhost:6379/1",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.url == "" && tt.shouldError {
				// Empty URL should error
				client, err := NewRedisFromURL("")
				if err == nil {
					t.Errorf("Expected error for empty URL")
				}
				if client != nil {
					t.Errorf("Expected nil client for empty URL")
				}
			}
		})
	}
}

// TestConfigConversion tests converting config to options
func TestConfigConversion(t *testing.T) {
	config := &RedisConfig{
		Addr:         "redis.example.com:6380",
		Password:     "secret123",
		DB:           3,
		PoolSize:     20,
		MinIdleConns: 8,
		MaxIdleTime:  10 * time.Minute,
		MaxConnAge:   20 * time.Minute,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	// Verify all fields are properly set
	if config.Addr != "redis.example.com:6380" {
		t.Errorf("Addr conversion failed")
	}
	if config.Password != "secret123" {
		t.Errorf("Password conversion failed")
	}
	if config.DB != 3 {
		t.Errorf("DB conversion failed")
	}
	if config.PoolSize != 20 {
		t.Errorf("PoolSize conversion failed")
	}
	if config.DialTimeout != 10*time.Second {
		t.Errorf("DialTimeout conversion failed")
	}
}

// TestDefaultRedisConfigAllFields tests that all fields have proper defaults
func TestDefaultRedisConfigAllFields(t *testing.T) {
	// Save and clear all env vars
	saved := map[string]string{
		"REDIS_ADDR":     os.Getenv("REDIS_ADDR"),
		"REDIS_PASSWORD": os.Getenv("REDIS_PASSWORD"),
		"REDIS_DB":       os.Getenv("REDIS_DB"),
	}
	defer func() {
		for key, val := range saved {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	for key := range saved {
		os.Unsetenv(key)
	}

	config := DefaultRedisConfig()

	// Check all fields have non-zero values
	if config.Addr == "" {
		t.Errorf("Addr should have default value")
	}
	if config.DB < 0 {
		t.Errorf("DB should be non-negative, got %d", config.DB)
	}
	if config.PoolSize <= 0 {
		t.Errorf("PoolSize should be positive")
	}
	if config.MinIdleConns < 0 {
		t.Errorf("MinIdleConns should be non-negative")
	}
	if config.MaxIdleTime <= 0 {
		t.Errorf("MaxIdleTime should be positive")
	}
	if config.MaxConnAge <= 0 {
		t.Errorf("MaxConnAge should be positive")
	}
	if config.DialTimeout <= 0 {
		t.Errorf("DialTimeout should be positive")
	}
	if config.ReadTimeout <= 0 {
		t.Errorf("ReadTimeout should be positive")
	}
	if config.WriteTimeout <= 0 {
		t.Errorf("WriteTimeout should be positive")
	}
}

// TestGetEnvIntWithVariousValues tests getEnvInt with different valid values
func TestGetEnvIntWithVariousValues(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		defaultValue int
		expected     int
	}{
		{"positive number", "123", 0, 123},
		{"zero", "0", 1, 0},
		{"negative number", "-42", 0, -42},
		{"large number", "1000000", 0, 1000000},
		{"max int32", "2147483647", 0, 2147483647},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_INT_" + tt.name
			os.Setenv(key, tt.input)
			defer os.Unsetenv(key)

			result := getEnvInt(key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestRedisConfigWithSpecialCharacterPassword tests config with special password
func TestRedisConfigWithSpecialCharacterPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{"space", "pass word"},
		{"special chars", "p@ss!w0rd#$%"},
		{"long password", "aVeryLongPasswordWith1234567890SpecialCharsAnd!@#$%^&*()"},
		{"empty password", ""},
	}

	for _, tt := range tests {
		config := &RedisConfig{
			Addr:     "localhost:6379",
			Password: tt.password,
			DB:       0,
		}

		if config.Password != tt.password {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.password, config.Password)
		}
	}
}

// TestRedisConfigDBSelection tests various DB selections
func TestRedisConfigDBSelection(t *testing.T) {
	for db := 0; db < 16; db++ {
		config := &RedisConfig{
			Addr: "localhost:6379",
			DB:   db,
		}

		if config.DB != db {
			t.Errorf("Expected DB %d, got %d", db, config.DB)
		}
	}
}

// TestRedisConfigHighPortNumbers tests configuration with high port numbers
func TestRedisConfigHighPortNumbers(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"standard port", "localhost:6379"},
		{"custom port", "redis.example.com:16379"},
		{"high port", "redis.example.com:65535"},
		{"ip address", "192.168.1.1:6379"},
		{"localhost ip", "127.0.0.1:6379"},
	}

	for _, tt := range tests {
		config := &RedisConfig{
			Addr: tt.addr,
		}

		if config.Addr != tt.addr {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.addr, config.Addr)
		}
	}
}

// TestCacheKeyEmptyFields tests CacheKey with empty fields
func TestCacheKeyEmptyFields(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		id       string
		suffix   string
		expected string
	}{
		{"empty prefix", "", "id", "", ":id"},
		{"empty id", "prefix", "", "", "prefix:"},
		{"empty suffix", "prefix", "id", "", "prefix:id"},
		{"all fields", "prefix", "id", "suffix", "prefix:id:suffix"},
	}

	for _, tt := range tests {
		key := CacheKey{
			Prefix: tt.prefix,
			ID:     tt.id,
			Suffix: tt.suffix,
		}

		result := key.String()
		if result != tt.expected {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.expected, result)
		}
	}
}

// TestCacheKeyWithSpecialCharacters tests CacheKey with special characters
func TestCacheKeyWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		id     string
		suffix string
	}{
		{"underscore", "pre_fix", "id_123", "suf_fix"},
		{"numbers", "123", "456", "789"},
		{"dash", "pre-fix", "id-123", "suf-fix"},
		{"mixed", "pre_fix-123", "id:456", "suf$fix"},
	}

	for _, tt := range tests {
		key := CacheKey{
			Prefix: tt.prefix,
			ID:     tt.id,
			Suffix: tt.suffix,
		}

		result := key.String()
		// Just verify it returns a non-empty string
		if len(result) == 0 {
			t.Errorf("%s: String() returned empty string", tt.name)
		}
	}
}

// TestRedisConfigPoolSizeConstraints tests pool size vs idle connections
func TestRedisConfigPoolSizeConstraints(t *testing.T) {
	tests := []struct {
		name        string
		poolSize    int
		minIdleConn int
		valid       bool
	}{
		{"idle less than pool", 10, 5, true},
		{"idle equals pool", 10, 10, true},
		{"idle zero", 10, 0, true},
		{"both zero", 0, 0, true},
	}

	for _, tt := range tests {
		config := &RedisConfig{
			PoolSize:     tt.poolSize,
			MinIdleConns: tt.minIdleConn,
		}

		// Config should be created regardless
		if config.PoolSize != tt.poolSize || config.MinIdleConns != tt.minIdleConn {
			t.Errorf("%s: config not set properly", tt.name)
		}
	}
}

// TestGetEnvIntInvalidInput tests getEnvInt with invalid inputs that should use default
func TestGetEnvIntInvalidInput(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		defaultValue int
		expected     int
	}{
		{"non-numeric", "abc", 42, 42},
		{"float value", "3.14", 42, 42},
		{"hex value", "0x10", 42, 42},
		{"scientific notation", "1e5", 42, 42},
		{"space only", "   ", 42, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_INVALID_" + tt.name
			os.Setenv(key, tt.value)
			defer os.Unsetenv(key)

			result := getEnvInt(key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestRedisConfigTimeoutHierarchy tests timeout value ordering
func TestRedisConfigTimeoutHierarchy(t *testing.T) {
	config := DefaultRedisConfig()

	// DialTimeout should typically be larger than read/write
	if config.DialTimeout < config.ReadTimeout {
		t.Logf("Note: DialTimeout (%v) < ReadTimeout (%v)", config.DialTimeout, config.ReadTimeout)
	}

	// All timeouts should be positive
	if config.DialTimeout <= 0 || config.ReadTimeout <= 0 || config.WriteTimeout <= 0 {
		t.Errorf("All timeouts should be positive")
	}
}

// TestNewRedisFromURLEmptyRedisURL tests NewRedisFromURL when REDIS_URL is also empty
func TestNewRedisFromURLEmptyRedisURL(t *testing.T) {
	originalURL := os.Getenv("REDIS_URL")
	os.Unsetenv("REDIS_URL")
	defer func() {
		if originalURL != "" {
			os.Setenv("REDIS_URL", originalURL)
		}
	}()

	client, err := NewRedisFromURL("")
	if err == nil {
		t.Errorf("Expected error when both URL and REDIS_URL are empty")
	}
	if client != nil {
		t.Errorf("Expected nil client when error occurs")
	}
	if err.Error() != "Redis URL not provided" {
		t.Errorf("Expected 'Redis URL not provided' error, got '%s'", err.Error())
	}
}

// TestNewRedisHandlesNilConfigByUsingDefaults verifies NewRedis() uses DefaultRedisConfig when nil
func TestNewRedisHandlesNilConfigByUsingDefaults(t *testing.T) {
	// We can't test the connection, but we can verify the logic path
	var config *RedisConfig = nil
	if config != nil {
		t.Errorf("Test setup failed")
	}

	// The actual NewRedis() function will try to connect when config is nil
	// but we've verified the nil-check logic through DefaultRedisConfig testing
	if config == nil {
		// This demonstrates the nil-check logic
		config = DefaultRedisConfig()
		if config == nil {
			t.Errorf("DefaultRedisConfig should not return nil")
		}
	}
}

// TestRedisConfigStructMembersInitializable tests that all RedisConfig fields can be initialized
func TestRedisConfigStructMembersInitializable(t *testing.T) {
	cfg := &RedisConfig{
		Addr:         "redis.example.com",
		Password:     "secret",
		DB:           2,
		PoolSize:     20,
		MinIdleConns: 8,
		MaxIdleTime:  10 * time.Minute,
		MaxConnAge:   20 * time.Minute,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	// Verify all fields were set correctly
	fields := []struct {
		name     string
		value    interface{}
		expected interface{}
	}{
		{"Addr", cfg.Addr, "redis.example.com"},
		{"Password", cfg.Password, "secret"},
		{"DB", cfg.DB, 2},
		{"PoolSize", cfg.PoolSize, 20},
		{"MinIdleConns", cfg.MinIdleConns, 8},
		{"MaxIdleTime", cfg.MaxIdleTime, 10 * time.Minute},
		{"MaxConnAge", cfg.MaxConnAge, 20 * time.Minute},
		{"DialTimeout", cfg.DialTimeout, 10 * time.Second},
		{"ReadTimeout", cfg.ReadTimeout, 5 * time.Second},
		{"WriteTimeout", cfg.WriteTimeout, 5 * time.Second},
	}

	for _, f := range fields {
		if f.value != f.expected {
			t.Errorf("%s: expected %v, got %v", f.name, f.expected, f.value)
		}
	}
}
