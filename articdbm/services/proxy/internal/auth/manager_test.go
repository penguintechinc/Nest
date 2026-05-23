package auth

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redismock/v8"
	"github.com/penguintechinc/articdbm/proxy/internal/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestAuthenticateValidUser(t *testing.T) {
	db, mock := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		Users: map[string]*config.User{
			"testuser": {
				Username: "testuser",
				Enabled:  true,
			},
		},
		Permissions: map[string]*config.Permission{
			"testuser": {
				UserID:   "testuser",
				Database: "testdb",
				Actions:  []string{"read"},
			},
		},
	}

	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	mock.ExpectGet("articdbm:auth:mysql:testuser:testdb:").RedisNil()
	mock.ExpectSet("articdbm:auth:mysql:testuser:testdb:", "allowed", 5*time.Minute).SetVal("OK")

	result := manager.Authenticate(context.Background(), "testuser", "testdb", "mysql")

	assert.True(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthenticateDisabledUser(t *testing.T) {
	db, mock := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		Users: map[string]*config.User{
			"testuser": {
				Username: "testuser",
				Enabled:  false,
			},
		},
	}

	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	mock.ExpectGet("articdbm:auth:mysql:testuser:testdb:").RedisNil()
	mock.ExpectSet("articdbm:auth:mysql:testuser:testdb:", "denied", 5*time.Minute).SetVal("OK")

	result := manager.Authenticate(context.Background(), "testuser", "testdb", "mysql")

	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthenticateExpiredUser(t *testing.T) {
	db, mock := redismock.NewClientMock()
	defer db.Close()

	pastTime := time.Now().Add(-24 * time.Hour)
	cfg := &config.Config{
		Users: map[string]*config.User{
			"testuser": {
				Username:  "testuser",
				Enabled:   true,
				ExpiresAt: &pastTime,
			},
		},
	}

	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	mock.ExpectGet("articdbm:auth:mysql:testuser:testdb:").RedisNil()
	mock.ExpectSet("articdbm:auth:mysql:testuser:testdb:", "denied", 5*time.Minute).SetVal("OK")

	result := manager.Authenticate(context.Background(), "testuser", "testdb", "mysql")

	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthenticateWrongIP(t *testing.T) {
	db, mock := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		Users: map[string]*config.User{
			"testuser": {
				Username:   "testuser",
				Enabled:    true,
				AllowedIPs: []string{"192.168.1.1", "10.0.0.0/8"},
			},
		},
	}

	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	mock.ExpectGet("articdbm:auth:mysql:testuser:testdb:172.16.0.1").RedisNil()
	mock.ExpectSet("articdbm:auth:mysql:testuser:testdb:172.16.0.1", "denied", 5*time.Minute).SetVal("OK")

	result := manager.AuthenticateWithIP(context.Background(), "testuser", "testdb", "mysql", "172.16.0.1")

	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthenticateNoPermission(t *testing.T) {
	db, mock := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		Users: map[string]*config.User{
			"testuser": {
				Username: "testuser",
				Enabled:  true,
			},
		},
		Permissions: map[string]*config.Permission{},
	}

	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	mock.ExpectGet("articdbm:auth:mysql:testuser:testdb:").RedisNil()
	mock.ExpectSet("articdbm:auth:mysql:testuser:testdb:", "denied", 5*time.Minute).SetVal("OK")

	result := manager.Authenticate(context.Background(), "testuser", "testdb", "mysql")

	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthenticateCachedResult(t *testing.T) {
	db, mock := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{}
	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	mock.ExpectGet("articdbm:auth:mysql:testuser:testdb:").SetVal("allowed")

	result := manager.Authenticate(context.Background(), "testuser", "testdb", "mysql")

	assert.True(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthorizeReadAllowed(t *testing.T) {
	db, mock := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		Permissions: map[string]*config.Permission{
			"testuser": {
				UserID:   "testuser",
				Database: "testdb",
				Table:    "*",
				Actions:  []string{"read"},
			},
		},
	}

	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	mock.ExpectGet("articdbm:authz:testuser:testdb:testtable:false").RedisNil()
	mock.ExpectSet("articdbm:authz:testuser:testdb:testtable:false", "allowed", 5*time.Minute).SetVal("OK")

	result := manager.Authorize(context.Background(), "testuser", "testdb", "testtable", false)

	assert.True(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthorizeWriteDenied(t *testing.T) {
	db, mock := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		Permissions: map[string]*config.Permission{
			"testuser": {
				UserID:   "testuser",
				Database: "testdb",
				Table:    "*",
				Actions:  []string{"read"},
			},
		},
	}

	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	mock.ExpectGet("articdbm:authz:testuser:testdb:testtable:true").RedisNil()
	mock.ExpectSet("articdbm:authz:testuser:testdb:testtable:true", "denied", 5*time.Minute).SetVal("OK")

	result := manager.Authorize(context.Background(), "testuser", "testdb", "testtable", true)

	assert.False(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestIsIPAllowedCIDR(t *testing.T) {
	db, _ := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{
		Users: map[string]*config.User{
			"testuser": {
				Username:   "testuser",
				AllowedIPs: []string{"192.168.1.0/24"},
			},
		},
	}

	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	user, _ := cfg.GetUser("testuser")
	result := manager.isIPAllowed("192.168.1.5", user.AllowedIPs)

	assert.True(t, result)
}

func TestIsIPAllowedDirect(t *testing.T) {
	db, _ := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{}
	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	allowedIPs := []string{"192.168.1.1", "10.0.0.1"}
	result := manager.isIPAllowed("192.168.1.1", allowedIPs)

	assert.True(t, result)
}

func TestHashPassword(t *testing.T) {
	db, _ := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{}
	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	hash1 := manager.HashPassword("mypassword")
	hash2 := manager.HashPassword("mypassword")

	assert.NotEmpty(t, hash1)
	assert.Equal(t, hash1, hash2)
	assert.NotEqual(t, "mypassword", hash1)
}

func TestGenerateAPIKey(t *testing.T) {
	db, _ := redismock.NewClientMock()
	defer db.Close()

	cfg := &config.Config{}
	logger := zap.NewNop()
	manager := NewManager(cfg, db, logger)

	key1 := manager.GenerateAPIKey()
	key2 := manager.GenerateAPIKey()

	assert.NotEmpty(t, key1)
	assert.NotEmpty(t, key2)
	assert.NotEqual(t, key1, key2)
	assert.Greater(t, len(key1), 40)
}
