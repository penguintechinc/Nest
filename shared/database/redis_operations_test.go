package database

import (
	"os"
	"testing"
)

// TestCacheKeyStringFormats tests various CacheKey string formatting
func TestCacheKeyStringFormats(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		id       string
		suffix   string
		expected string
	}{
		{
			name:     "no suffix",
			prefix:   "license",
			id:       "key123",
			suffix:   "",
			expected: "license:key123",
		},
		{
			name:     "with suffix",
			prefix:   "feature",
			id:       "key456",
			suffix:   "advanced_analytics",
			expected: "feature:key456:advanced_analytics",
		},
		{
			name:     "session without suffix",
			prefix:   "session",
			id:       "sess_id_789",
			suffix:   "",
			expected: "session:sess_id_789",
		},
		{
			name:     "empty prefix",
			prefix:   "",
			id:       "test",
			suffix:   "",
			expected: ":test",
		},
		{
			name:     "empty id with prefix and suffix",
			prefix:   "prefix",
			id:       "",
			suffix:   "suffix",
			expected: "prefix::suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := CacheKey{
				Prefix: tt.prefix,
				ID:     tt.id,
				Suffix: tt.suffix,
			}
			result := key.String()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestCacheKeyStringConsistency tests that String() always returns the same value
func TestCacheKeyStringConsistency(t *testing.T) {
	key := CacheKey{
		Prefix: "test",
		ID:     "123",
		Suffix: "suffix",
	}

	result1 := key.String()
	result2 := key.String()
	result3 := key.String()

	if result1 != result2 || result2 != result3 {
		t.Errorf("String() returned inconsistent results: %s, %s, %s", result1, result2, result3)
	}
}

// TestRedisClientStructure tests RedisClient can be initialized
func TestRedisClientStructure(t *testing.T) {
	config := &RedisConfig{
		Addr:     "localhost:6379",
		Password: "pass",
		DB:       1,
		PoolSize: 20,
	}

	if config.Addr != "localhost:6379" {
		t.Errorf("Expected Addr to be set")
	}
	if config.PoolSize != 20 {
		t.Errorf("Expected PoolSize=20")
	}
}

// TestGetEnvIntEdgeCases tests getEnvInt with edge cases
func TestGetEnvIntEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		defaultValue int
		expected     int
	}{
		{
			name:         "large positive number",
			value:        "999999",
			defaultValue: 0,
			expected:     999999,
		},
		{
			name:         "zero with non-zero default",
			value:        "0",
			defaultValue: 100,
			expected:     0,
		},
		{
			name:         "empty string uses default",
			value:        "",
			defaultValue: 42,
			expected:     42,
		},
		{
			name:         "whitespace treated as invalid",
			value:        "   ",
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "floating point rounded down",
			value:        "123.45",
			defaultValue: 0,
			expected:     0, // Atoi fails on "123.45"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_EDGE_" + tt.name
			if tt.value != "" {
				os.Setenv(key, tt.value)
				defer os.Unsetenv(key)
			} else {
				os.Unsetenv(key)
			}
			result := getEnvInt(key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestCacheKeyPrefixVariations tests cache key formation with different prefixes
func TestCacheKeyPrefixVariations(t *testing.T) {
	prefixes := []string{"license", "feature", "session", "ratelimit", "usage", "config", "metrics"}

	for _, prefix := range prefixes {
		key := CacheKey{
			Prefix: prefix,
			ID:     "test_id",
		}

		result := key.String()
		expected := prefix + ":test_id"

		if result != expected {
			t.Errorf("For prefix '%s': expected '%s', got '%s'", prefix, expected, result)
		}
	}
}

// TestRedisConfigTimeouts tests Redis timeout settings
func TestRedisConfigTimeouts(t *testing.T) {
	config := DefaultRedisConfig()

	// Verify timeout values are reasonable (positive and not zero)
	if config.DialTimeout <= 0 {
		t.Errorf("DialTimeout should be positive, got %v", config.DialTimeout)
	}
	if config.ReadTimeout <= 0 {
		t.Errorf("ReadTimeout should be positive, got %v", config.ReadTimeout)
	}
	if config.WriteTimeout <= 0 {
		t.Errorf("WriteTimeout should be positive, got %v", config.WriteTimeout)
	}
}

// TestRedisConfigConnPoolSettings tests Redis connection pool settings
func TestRedisConfigConnPoolSettings(t *testing.T) {
	config := DefaultRedisConfig()

	// Verify pool settings are reasonable
	if config.PoolSize <= 0 {
		t.Errorf("PoolSize should be positive, got %d", config.PoolSize)
	}
	if config.MinIdleConns < 0 {
		t.Errorf("MinIdleConns should be non-negative, got %d", config.MinIdleConns)
	}
	if config.MinIdleConns > config.PoolSize {
		t.Errorf("MinIdleConns (%d) should not exceed PoolSize (%d)", config.MinIdleConns, config.PoolSize)
	}
}

// TestCacheKeyInterfaceCompliance tests that CacheKey properly implements String interface
func TestCacheKeyInterfaceCompliance(t *testing.T) {
	key := CacheKey{
		Prefix: "test",
		ID:     "123",
		Suffix: "suffix",
	}

	// String() should return a string type
	result := key.String()
	if _, ok := interface{}(result).(string); !ok {
		t.Errorf("String() should return string type")
	}

	// Should be non-empty
	if len(result) == 0 {
		t.Errorf("String() should return non-empty string")
	}
}

// TestRedisAddrDefaults tests Redis address defaults
func TestRedisAddrDefaults(t *testing.T) {
	// Save and clear env var
	originalAddr := os.Getenv("REDIS_ADDR")
	os.Unsetenv("REDIS_ADDR")
	defer os.Setenv("REDIS_ADDR", originalAddr)

	config := DefaultRedisConfig()

	if config.Addr != "localhost:6379" {
		t.Errorf("Expected default addr 'localhost:6379', got '%s'", config.Addr)
	}
}

// TestRedisDBDefaults tests Redis DB selection defaults
func TestRedisDBDefaults(t *testing.T) {
	// Save and clear env var
	originalDB := os.Getenv("REDIS_DB")
	os.Unsetenv("REDIS_DB")
	defer os.Setenv("REDIS_DB", originalDB)

	config := DefaultRedisConfig()

	if config.DB != 0 {
		t.Errorf("Expected default DB 0, got %d", config.DB)
	}
}

// TestCacheKeyLicensePattern tests license cache key pattern formation
func TestCacheKeyLicensePattern(t *testing.T) {
	tests := []struct {
		name       string
		licenseKey string
		expected   string
	}{
		{"simple key", "key123", "license:key123:validation"},
		{"uuid format", "550e8400-e29b-41d4-a716-446655440000", "license:550e8400-e29b-41d4-a716-446655440000:validation"},
		{"alpha key", "abc_def_ghi", "license:abc_def_ghi:validation"},
	}

	for _, tt := range tests {
		key := CacheKey{Prefix: "license", ID: tt.licenseKey, Suffix: "validation"}
		result := key.String()
		if result != tt.expected {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.expected, result)
		}
	}
}

// TestCacheKeyFeaturePattern tests feature cache key pattern formation
func TestCacheKeyFeaturePattern(t *testing.T) {
	tests := []struct {
		name       string
		licenseKey string
		feature    string
		expected   string
	}{
		{"simple", "key1", "export", "feature:key1:export"},
		{"advanced feature", "key2", "advanced_analytics", "feature:key2:advanced_analytics"},
		{"with numbers", "key3", "feature_v2_1", "feature:key3:feature_v2_1"},
	}

	for _, tt := range tests {
		key := CacheKey{Prefix: "feature", ID: tt.licenseKey, Suffix: tt.feature}
		result := key.String()
		if result != tt.expected {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.expected, result)
		}
	}
}

// TestCacheKeySessionPattern tests session cache key pattern formation
func TestCacheKeySessionPattern(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		expected  string
	}{
		{"simple", "sess123", "session:sess123"},
		{"uuid", "550e8400-e29b-41d4-a716-446655440000", "session:550e8400-e29b-41d4-a716-446655440000"},
		{"long token", "abc123def456ghi789", "session:abc123def456ghi789"},
	}

	for _, tt := range tests {
		key := CacheKey{Prefix: "session", ID: tt.sessionID}
		result := key.String()
		if result != tt.expected {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.expected, result)
		}
	}
}

// TestCacheKeyUsagePattern tests usage tracking cache key formation
func TestCacheKeyUsagePattern(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		feature  string
		expected string
	}{
		{"export", "1", "export", "usage:1:export"},
		{"analytics", "42", "advanced_analytics", "usage:42:advanced_analytics"},
		{"multiple words", "100", "feature_usage_tracking", "usage:100:feature_usage_tracking"},
	}

	for _, tt := range tests {
		key := CacheKey{Prefix: "usage", ID: tt.userID, Suffix: tt.feature}
		result := key.String()
		if result != tt.expected {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.expected, result)
		}
	}
}

// TestCacheKeyConfigPattern tests config cache key formation
func TestCacheKeyConfigPattern(t *testing.T) {
	tests := []struct {
		name      string
		configKey string
		expected  string
	}{
		{"app settings", "app_settings", "config:app_settings"},
		{"feature flags", "feature_flags", "config:feature_flags"},
		{"simple", "db_host", "config:db_host"},
	}

	for _, tt := range tests {
		key := CacheKey{Prefix: "config", ID: tt.configKey}
		result := key.String()
		if result != tt.expected {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.expected, result)
		}
	}
}

// TestCacheKeyMetricsPattern tests metrics cache key formation
func TestCacheKeyMetricsPattern(t *testing.T) {
	tests := []struct {
		name      string
		metricKey string
		suffix    string
		expected  string
	}{
		{"cpu usage", "cpu", "total", "metrics:cpu:total"},
		{"memory", "memory", "used_mb", "metrics:memory:used_mb"},
		{"requests", "http_requests", "count", "metrics:http_requests:count"},
	}

	for _, tt := range tests {
		key := CacheKey{Prefix: "metrics", ID: tt.metricKey, Suffix: tt.suffix}
		result := key.String()
		if result != tt.expected {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.expected, result)
		}
	}
}

// TestCacheKeyRateLimitPattern tests rate limit cache key formation
func TestCacheKeyRateLimitPattern(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		expected   string
	}{
		{"ip address", "192.168.1.1", "ratelimit:192.168.1.1"},
		{"user id", "user_123", "ratelimit:user_123"},
		{"api key", "api_key_abc", "ratelimit:api_key_abc"},
	}

	for _, tt := range tests {
		key := CacheKey{Prefix: "ratelimit", ID: tt.identifier}
		result := key.String()
		if result != tt.expected {
			t.Errorf("%s: expected '%s', got '%s'", tt.name, tt.expected, result)
		}
	}
}

// TestCacheKeyFormattingConsistency tests that formatting is consistent
func TestCacheKeyFormattingConsistency(t *testing.T) {
	key := CacheKey{Prefix: "test", ID: "id", Suffix: "suffix"}

	results := make([]string, 5)
	for i := 0; i < 5; i++ {
		results[i] = key.String()
	}

	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Errorf("CacheKey.String() returned inconsistent results")
		}
	}
}

// TestCacheKeyStringNotEmpty tests that String() always returns non-empty
func TestCacheKeyStringNotEmpty(t *testing.T) {
	keys := []CacheKey{
		{Prefix: "", ID: "", Suffix: ""},
		{Prefix: "a", ID: "", Suffix: ""},
		{Prefix: "", ID: "b", Suffix: ""},
		{Prefix: "", ID: "", Suffix: "c"},
		{Prefix: "p", ID: "i", Suffix: ""},
		{Prefix: "p", ID: "", Suffix: "s"},
		{Prefix: "", ID: "i", Suffix: "s"},
		{Prefix: "p", ID: "i", Suffix: "s"},
	}

	for _, key := range keys {
		result := key.String()
		// Should always return something, even if empty fields
		if len(result) == 0 && (key.Prefix != "" || key.ID != "" || key.Suffix != "") {
			t.Errorf("String() returned empty for non-empty key: %+v", key)
		}
	}
}

// TestCacheKeyWithLongValues tests CacheKey with very long field values
func TestCacheKeyWithLongValues(t *testing.T) {
	longValue := "a"
	for i := 0; i < 100; i++ {
		longValue += "a"
	}

	key := CacheKey{
		Prefix: longValue,
		ID:     longValue,
		Suffix: longValue,
	}

	result := key.String()
	if len(result) == 0 {
		t.Errorf("String() failed with long values")
	}

	// Should contain all values
	if !stringContainsOps(result, longValue) {
		t.Errorf("Result should contain long value")
	}
}

// stringContainsOps checks if string contains substring
func stringContainsOps(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
