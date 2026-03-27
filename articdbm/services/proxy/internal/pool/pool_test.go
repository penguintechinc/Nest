package pool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewConnectionPoolBadDriver(t *testing.T) {
	pool := NewConnectionPool("nonexistent_driver", "bad_dsn", 10)
	assert.Nil(t, pool)
}

func TestNewConnectionPoolValid(t *testing.T) {
	// Note: This test doesn't actually connect, it just verifies the pool is created
	// We use a driver that's registered but with a potentially invalid DSN
	// sql.Open doesn't actually connect until the first query
	pool := NewConnectionPool("mysql", "user:pass@tcp(localhost:3306)/db", 10)

	// If mysql driver is not available, pool will be nil
	if pool == nil {
		t.Skip("mysql driver not available")
	}
	defer pool.Close()

	assert.NotNil(t, pool)
	assert.Equal(t, "mysql", pool.driver)
	assert.Equal(t, "user:pass@tcp(localhost:3306)/db", pool.dsn)
	assert.Equal(t, 10, pool.maxConns)
}

func TestPoolStats(t *testing.T) {
	pool := NewConnectionPool("mysql", "user:pass@tcp(localhost:3306)/db", 20)

	if pool == nil {
		t.Skip("mysql driver not available")
	}
	defer pool.Close()

	stats := pool.Stats()
	assert.Equal(t, 20, stats.MaxOpenConnections)
}

func TestPoolClose(t *testing.T) {
	pool := NewConnectionPool("mysql", "user:pass@tcp(localhost:3306)/db", 10)

	if pool == nil {
		t.Skip("mysql driver not available")
	}

	err := pool.Close()
	assert.NoError(t, err)
}

func TestPoolPingNoConnection(t *testing.T) {
	pool := NewConnectionPool("mysql", "invalid:invalid@tcp(localhost:9999)/invalid", 10)

	if pool == nil {
		t.Skip("mysql driver not available")
	}
	defer pool.Close()

	// Ping should fail because the connection is invalid
	err := pool.Ping()
	assert.Error(t, err)
}
