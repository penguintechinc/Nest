package handlers

import (
	"testing"

	"github.com/go-redis/redismock/v8"
	"github.com/penguintechinc/articdbm/proxy/internal/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewMySQLHandler(t *testing.T) {
	db, _ := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		MySQLBackends:         []config.Backend{},
		SQLInjectionDetection: true,
		SeedDefaultBlocked:    false,
	}
	logger := zap.NewNop()

	handler := NewMySQLHandler(cfg, db, logger, nil, nil, nil)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.authManager)
	assert.NotNil(t, handler.secChecker)
	assert.NotNil(t, handler.pools)
}

func TestNewPostgreSQLHandler(t *testing.T) {
	db, _ := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		PostgreSQLBackends:    []config.Backend{},
		SQLInjectionDetection: true,
		SeedDefaultBlocked:    false,
	}
	logger := zap.NewNop()

	handler := NewPostgreSQLHandler(cfg, db, logger)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.authManager)
	assert.NotNil(t, handler.secChecker)
	assert.NotNil(t, handler.pools)
}

func TestNewMongoDBHandler(t *testing.T) {
	db, _ := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		MongoDBBackends:       []config.Backend{},
		SQLInjectionDetection: true,
		SeedDefaultBlocked:    false,
	}
	logger := zap.NewNop()

	handler := NewMongoDBHandler(cfg, db, logger)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.authManager)
	assert.NotNil(t, handler.secChecker)
}

func TestNewRedisProxyHandler(t *testing.T) {
	db, _ := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		RedisBackends:         []config.Backend{},
		SQLInjectionDetection: true,
		SeedDefaultBlocked:    false,
	}
	logger := zap.NewNop()

	handler := NewRedisProxyHandler(cfg, db, logger)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.authManager)
	assert.NotNil(t, handler.secChecker)
}

func TestNewMSSQLHandler(t *testing.T) {
	db, _ := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		MSSQLBackends:         []config.Backend{},
		SQLInjectionDetection: true,
		SeedDefaultBlocked:    false,
	}
	logger := zap.NewNop()

	handler := NewMSSQLHandler(cfg, db, logger)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.authManager)
	assert.NotNil(t, handler.secChecker)
	assert.NotNil(t, handler.pools)
}

func TestAllHandlersNotNil(t *testing.T) {
	db, _ := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		SQLInjectionDetection: true,
		SeedDefaultBlocked:    false,
	}
	logger := zap.NewNop()

	assert.NotNil(t, NewMySQLHandler(cfg, db, logger, nil, nil, nil))
	assert.NotNil(t, NewPostgreSQLHandler(cfg, db, logger))
	assert.NotNil(t, NewMongoDBHandler(cfg, db, logger))
	assert.NotNil(t, NewRedisProxyHandler(cfg, db, logger))
	assert.NotNil(t, NewMSSQLHandler(cfg, db, logger))
}
