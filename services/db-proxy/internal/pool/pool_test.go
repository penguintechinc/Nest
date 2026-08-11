package pool

import (
	"testing"

	"go.uber.org/zap"
)

func TestPoolRegisterProtocol(t *testing.T) {
	logger := zap.NewNop()
	pool := NewPool(5, logger)

	pool.RegisterProtocol("mysql")

	// Verify protocol is registered
	if len(pool.pools) != 1 {
		t.Errorf("expected 1 pool, got %d", len(pool.pools))
	}

	if _, ok := pool.pools["mysql"]; !ok {
		t.Error("mysql protocol not registered")
	}
}

func TestPoolGet(t *testing.T) {
	logger := zap.NewNop()
	pool := NewPool(5, logger)
	pool.RegisterProtocol("mysql")

	// Get a connection
	conn, err := pool.Get("mysql")
	if err != nil {
		t.Fatalf("failed to get connection: %v", err)
	}

	if conn == nil {
		t.Error("returned connection is nil")
	}
}

func TestPoolGetUnregisteredProtocol(t *testing.T) {
	logger := zap.NewNop()
	pool := NewPool(5, logger)

	conn, err := pool.Get("unknown")
	if err == nil {
		t.Error("expected error for unregistered protocol")
	}

	if conn != nil {
		t.Error("expected nil connection for unregistered protocol")
	}
}

func TestPoolPut(t *testing.T) {
	logger := zap.NewNop()
	pool := NewPool(5, logger)
	pool.RegisterProtocol("mysql")

	conn, _ := pool.Get("mysql")
	err := pool.Put("mysql", conn)
	if err != nil {
		t.Fatalf("failed to put connection: %v", err)
	}
}

func TestPoolClose(t *testing.T) {
	logger := zap.NewNop()
	pool := NewPool(5, logger)
	pool.RegisterProtocol("mysql")

	conn, _ := pool.Get("mysql")
	pool.Put("mysql", conn)

	err := pool.Close()
	if err != nil {
		t.Fatalf("failed to close pool: %v", err)
	}
}

func TestPoolGetStats(t *testing.T) {
	logger := zap.NewNop()
	pool := NewPool(5, logger)
	pool.RegisterProtocol("mysql")

	// Get a connection
	conn, _ := pool.Get("mysql")

	stats := pool.GetStats()
	if stats["mysql"] == nil {
		t.Error("no stats for mysql protocol")
	}

	if mysqlStats, ok := stats["mysql"].(map[string]interface{}); ok {
		if active, ok := mysqlStats["active_connections"].(int32); !ok || active == 0 {
			t.Errorf("expected active_connections > 0, got %v", active)
		}
	}

	pool.Put("mysql", conn)
}

func TestPoolMaxConnections(t *testing.T) {
	logger := zap.NewNop()
	pool := NewPool(2, logger)
	pool.RegisterProtocol("mysql")

	// Get max connections
	conn1, _ := pool.Get("mysql")
	conn2, _ := pool.Get("mysql")

	// Third get should fail
	conn3, err := pool.Get("mysql")
	if err == nil {
		t.Error("expected error when pool exhausted")
		pool.Put("mysql", conn3)
	}

	pool.Put("mysql", conn1)
	pool.Put("mysql", conn2)
}

func TestPoolMultipleProtocols(t *testing.T) {
	logger := zap.NewNop()
	pool := NewPool(5, logger)

	protocols := []string{"mysql", "postgresql", "redis"}
	for _, proto := range protocols {
		pool.RegisterProtocol(proto)
	}

	stats := pool.GetStats()
	if len(stats) != 3 {
		t.Errorf("expected 3 protocols, got %d", len(stats))
	}

	for _, proto := range protocols {
		conn, err := pool.Get(proto)
		if err != nil {
			t.Errorf("failed to get connection for %s: %v", proto, err)
		}
		pool.Put(proto, conn)
	}

	pool.Close()
}
