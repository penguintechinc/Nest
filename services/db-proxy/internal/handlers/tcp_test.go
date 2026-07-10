package handlers

import (
	"context"
	"testing"

	"github.com/penguintechinc/nest/services/db-proxy/internal/config"
	"github.com/penguintechinc/nest/services/db-proxy/internal/pool"
	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"github.com/penguintechinc/nest/services/db-proxy/internal/security"
	"go.uber.org/zap"
)

func TestTCPHandlerStartStop(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.Config{
		ListenPort:            15432,
		DefaultConnectionRate: 100,
		DefaultQueryRate:      1000,
	}

	connPool := pool.NewPool(10, logger)
	connPool.RegisterProtocol("mysql")

	checker := security.NewChecker(logger)

	router := routing.NewRouter(logger)

	handler := NewTCPHandler("mysql", 15432, connPool, checker, cfg, logger, router, "test-route")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test Start
	err := handler.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start handler: %v", err)
	}
	defer handler.Stop()

	// Verify it's running
	stats := handler.GetStats()
	if running, ok := stats["running"].(bool); !ok || !running {
		t.Error("handler not reported as running")
	}

	if protocol, ok := stats["protocol"].(string); !ok || protocol != "mysql" {
		t.Error("protocol not set correctly")
	}
}

func TestSecurityCheckerBlocksInjection(t *testing.T) {
	logger := zap.NewNop()
	checker := security.NewChecker(logger)

	tests := []struct {
		name        string
		query       string
		shouldBlock bool
	}{
		{
			name:        "legitimate select",
			query:       "SELECT id, name FROM users WHERE id = 1",
			shouldBlock: false,
		},
		{
			name:        "union-based injection",
			query:       "SELECT * FROM users WHERE id = 1 UNION SELECT 1, 2, 3",
			shouldBlock: true,
		},
		{
			name:        "comment-based injection",
			query:       "SELECT * FROM users WHERE id = 1 -- malicious",
			shouldBlock: true,
		},
		{
			name:        "stacked query",
			query:       "SELECT * FROM users; DROP TABLE users;",
			shouldBlock: true,
		},
		{
			name:        "time-based blind",
			query:       "SELECT * FROM users WHERE id = SLEEP(5)",
			shouldBlock: true,
		},
		{
			name:        "boolean-based",
			query:       "SELECT * FROM users WHERE id = 1 OR 1=1",
			shouldBlock: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, reason := checker.CheckQuery(tt.query)
			if blocked != tt.shouldBlock {
				t.Errorf("expected blocked=%v, got %v (reason: %s)", tt.shouldBlock, blocked, reason)
			}

			stats := checker.GetStats()
			if inspected, ok := stats["inspected_count"].(int64); !ok || inspected == 0 {
				t.Error("inspected_count not incremented")
			}

			if tt.shouldBlock {
				if blocked, ok := stats["blocked_count"].(int64); !ok || blocked == 0 {
					t.Error("blocked_count not incremented")
				}
			}
		})
	}
}

func TestConnectionPooling(t *testing.T) {
	logger := zap.NewNop()
	connPool := pool.NewPool(5, logger)
	connPool.RegisterProtocol("mysql")

	// Get a connection
	conn1, err := connPool.Get("mysql")
	if err != nil {
		t.Fatalf("failed to get connection: %v", err)
	}

	// Return it
	err = connPool.Put("mysql", conn1)
	if err != nil {
		t.Fatalf("failed to put connection: %v", err)
	}

	// Get it again (should come from pool)
	conn2, err := connPool.Get("mysql")
	if err != nil {
		t.Fatalf("failed to get second connection: %v", err)
	}

	stats := connPool.GetStats()
	if stats["mysql"] == nil {
		t.Error("no stats for mysql protocol")
	}

	err = connPool.Put("mysql", conn2)
	if err != nil {
		t.Fatalf("failed to put second connection: %v", err)
	}

	connPool.Close()
}

func TestRateLimiting(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.Config{
		ListenPort:            15433,
		DefaultConnectionRate: 10, // Low limit for testing
		DefaultQueryRate:      10,
	}

	connPool := pool.NewPool(10, logger)
	connPool.RegisterProtocol("mysql")

	checker := security.NewChecker(logger)
	router := routing.NewRouter(logger)
	handler := NewTCPHandler("mysql", 15433, connPool, checker, cfg, logger, router, "test-route")

	// Allow should work initially
	if !handler.connLimiter.Allow() {
		t.Error("connection limiter should allow first request")
	}

	// Drain the limiter
	for i := 0; i < cfg.DefaultConnectionRate; i++ {
		handler.connLimiter.Allow()
	}

	// Should reject when limit exceeded
	if handler.connLimiter.Allow() {
		t.Error("connection limiter should reject when limit exceeded")
	}
}

// TestProxyCopyWithRouting is REMOVED - replaced by ProxyLoop which uses proper packet framing
// and per-query routing. See handlers_e2e_test.go for new integration tests.

func TestHandlerStats(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.Config{
		ListenPort:            15435,
		DefaultConnectionRate: 100,
		DefaultQueryRate:      1000,
	}

	connPool := pool.NewPool(10, logger)
	connPool.RegisterProtocol("mysql")

	checker := security.NewChecker(logger)
	router := routing.NewRouter(logger)
	handler := NewTCPHandler("mysql", 15435, connPool, checker, cfg, logger, router, "test-route")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = handler.Start(ctx)
	defer handler.Stop()

	// Simulate some activity
	handler.activeConns.Add(5)
	handler.totalConns.Add(10)
	handler.totalBlocked.Add(2)

	stats := handler.GetStats()

	if v, ok := stats["active_conns"].(int64); !ok || v != 5 {
		t.Errorf("expected active_conns=5, got %v", stats["active_conns"])
	}

	if v, ok := stats["total_conns"].(int64); !ok || v != 10 {
		t.Errorf("expected total_conns=10, got %v", stats["total_conns"])
	}

	if v, ok := stats["total_blocked"].(int64); !ok || v != 2 {
		t.Errorf("expected total_blocked=2, got %v", stats["total_blocked"])
	}

	if protocol, ok := stats["protocol"].(string); !ok || protocol != "mysql" {
		t.Errorf("expected protocol=mysql, got %v", stats["protocol"])
	}
}

func TestManagerHandlers(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)

	cfg := &config.Config{
		ListenPort:            0, // Use port 0 for random free port
		DefaultConnectionRate: 100,
		DefaultQueryRate:      1000,
	}

	connPool := pool.NewPool(10, logger)
	for _, proto := range []string{"mysql", "postgresql", "redis"} {
		connPool.RegisterProtocol(proto)
	}

	checker := security.NewChecker(logger)
	router := routing.NewRouter(logger)

	// Add handlers on different high ports (sequential)
	ports := []int{25432, 25433, 25434}
	for i, proto := range []string{"mysql", "postgresql", "redis"} {
		handler := NewTCPHandler(proto, ports[i], connPool, checker, cfg, logger, router, "test-route")
		if err := manager.AddHandler(handler); err != nil {
			t.Fatalf("failed to add handler for %s: %v", proto, err)
		}
	}

	// Test GetStats without actually starting handlers (avoid port conflicts)
	stats := manager.GetStats()
	if len(stats) != 3 {
		t.Errorf("expected 3 protocol stats, got %d", len(stats))
	}

	// Verify all protocols are present
	for _, proto := range []string{"mysql", "postgresql", "redis"} {
		if _, ok := stats[proto]; !ok {
			t.Errorf("protocol %s not in stats", proto)
		}
	}
}

func BenchmarkSecurityCheck(b *testing.B) {
	logger := zap.NewNop()
	checker := security.NewChecker(logger)

	query := "SELECT id, name, email FROM users WHERE id = 123 AND active = true"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckQuery(query)
	}
}

// BenchmarkProxyCopy is REMOVED - replaced by ProxyLoop
// Old benchmark tested the dual-goroutine io.Copy pattern which is obsolete
