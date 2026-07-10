package config

import (
	"os"
	"testing"

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
