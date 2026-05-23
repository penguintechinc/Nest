package config

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg := LoadConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, true, cfg.MySQLEnabled)
	assert.Equal(t, 9090, cfg.MetricsPort)
	assert.Equal(t, 1000, cfg.MaxConnections)
	assert.Equal(t, 3306, cfg.MySQLPort)
	assert.Equal(t, 5432, cfg.PostgreSQLPort)
	assert.Equal(t, 1433, cfg.MSSQLPort)
	assert.Equal(t, 27017, cfg.MongoDBPort)
	assert.Equal(t, 6379, cfg.RedisProxyPort)
	assert.Equal(t, "localhost:6379", cfg.RedisAddr)
	assert.Equal(t, 30*time.Second, cfg.ConnectionTimeout)
	assert.Equal(t, 60*time.Second, cfg.QueryTimeout)
	assert.NotNil(t, cfg.Users)
	assert.NotNil(t, cfg.Permissions)
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	t.Setenv("MYSQL_PORT", "3307")
	t.Setenv("METRICS_PORT", "9999")
	t.Setenv("MAX_CONNECTIONS", "2000")
	t.Setenv("CONNECTION_TIMEOUT", "45")
	t.Setenv("REDIS_ADDR", "redis.example.com:6379")

	cfg := LoadConfig()

	assert.Equal(t, 3307, cfg.MySQLPort)
	assert.Equal(t, 9999, cfg.MetricsPort)
	assert.Equal(t, 2000, cfg.MaxConnections)
	assert.Equal(t, 45*time.Second, cfg.ConnectionTimeout)
	assert.Equal(t, "redis.example.com:6379", cfg.RedisAddr)
}

func TestGetUser(t *testing.T) {
	cfg := &Config{
		Users: map[string]*User{
			"testuser": {
				Username: "testuser",
				Enabled:  true,
			},
		},
	}

	user, ok := cfg.GetUser("testuser")
	assert.True(t, ok)
	assert.NotNil(t, user)
	assert.Equal(t, "testuser", user.Username)

	user, ok = cfg.GetUser("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, user)
}

func TestGetPermission(t *testing.T) {
	cfg := &Config{
		Permissions: map[string]*Permission{
			"testuser": {
				UserID:   "testuser",
				Database: "testdb",
				Actions:  []string{"read", "write"},
			},
		},
	}

	perm, ok := cfg.GetPermission("testuser")
	assert.True(t, ok)
	assert.NotNil(t, perm)
	assert.Equal(t, "testdb", perm.Database)
	assert.Equal(t, []string{"read", "write"}, perm.Actions)

	perm, ok = cfg.GetPermission("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, perm)
}

func TestGetReadBackends(t *testing.T) {
	cfg := &Config{
		MySQLBackends: []Backend{
			{Host: "db1.example.com", Port: 3306, Type: "read"},
			{Host: "db2.example.com", Port: 3306, Type: "write"},
			{Host: "db3.example.com", Port: 3306, Type: "read"},
		},
	}

	readBackends := cfg.GetReadBackends("mysql")
	assert.Len(t, readBackends, 2)
	assert.Equal(t, "db1.example.com", readBackends[0].Host)
	assert.Equal(t, "db3.example.com", readBackends[1].Host)
}

func TestGetWriteBackends(t *testing.T) {
	cfg := &Config{
		PostgreSQLBackends: []Backend{
			{Host: "pg1.example.com", Port: 5432, Type: "read"},
			{Host: "pg2.example.com", Port: 5432, Type: "write"},
			{Host: "pg3.example.com", Port: 5432, Type: "write"},
		},
	}

	writeBackends := cfg.GetWriteBackends("postgresql")
	assert.Len(t, writeBackends, 2)
	assert.Equal(t, "pg2.example.com", writeBackends[0].Host)
	assert.Equal(t, "pg3.example.com", writeBackends[1].Host)
}

func TestGetReadBackendsUnknownType(t *testing.T) {
	cfg := &Config{}

	backends := cfg.GetReadBackends("unknown")
	assert.Empty(t, backends)
}

func TestGetWriteBackendsUnknownType(t *testing.T) {
	cfg := &Config{}

	backends := cfg.GetWriteBackends("unknown")
	assert.Empty(t, backends)
}

func TestGetReadBackendsNoType(t *testing.T) {
	cfg := &Config{
		MySQLBackends: []Backend{
			{Host: "db1.example.com", Port: 3306, Type: ""},
			{Host: "db2.example.com", Port: 3306, Type: ""},
		},
	}

	readBackends := cfg.GetReadBackends("mysql")
	assert.Len(t, readBackends, 2)
}

func TestGetWriteBackendsNoType(t *testing.T) {
	cfg := &Config{
		MySQLBackends: []Backend{
			{Host: "db1.example.com", Port: 3306, Type: ""},
			{Host: "db2.example.com", Port: 3306, Type: ""},
		},
	}

	writeBackends := cfg.GetWriteBackends("mysql")
	assert.Len(t, writeBackends, 2)
}

func TestParseBackendsJSON(t *testing.T) {
	backends := []Backend{
		{Host: "db1.example.com", Port: 3306, Type: "read"},
		{Host: "db2.example.com", Port: 3306, Type: "write"},
	}
	backendsJSON, err := json.Marshal(backends)
	assert.NoError(t, err)

	t.Setenv("MYSQL_BACKENDS", string(backendsJSON))

	cfg := LoadConfig()

	assert.Len(t, cfg.MySQLBackends, 2)
	assert.Equal(t, "db1.example.com", cfg.MySQLBackends[0].Host)
	assert.Equal(t, "db2.example.com", cfg.MySQLBackends[1].Host)
}

func TestParseBackendsInvalidJSON(t *testing.T) {
	t.Setenv("MYSQL_BACKENDS", "{invalid json")

	cfg := LoadConfig()

	assert.Empty(t, cfg.MySQLBackends)
}
