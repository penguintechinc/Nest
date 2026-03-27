package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/penguintechinc/articdbm/proxy/internal/config"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type Manager struct {
	cfg    *config.Config
	redis  *redis.Client
	logger *zap.Logger
}

func NewManager(cfg *config.Config, redis *redis.Client, logger *zap.Logger) *Manager {
	return &Manager{
		cfg:    cfg,
		redis:  redis,
		logger: logger,
	}
}

func (m *Manager) Authenticate(ctx context.Context, username, database, dbType string) bool {
	return m.AuthenticateWithIP(ctx, username, database, dbType, "")
}

func (m *Manager) AuthenticateWithIP(ctx context.Context, username, database, dbType, clientIP string) bool {
	cacheKey := fmt.Sprintf("articdbm:auth:%s:%s:%s:%s", dbType, username, database, clientIP)
	
	cached, err := m.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		return cached == "allowed"
	}

	user, ok := m.cfg.GetUser(username)
	if !ok || !user.Enabled {
		m.logger.Warn("Authentication failed: user not found or disabled", 
			zap.String("username", username))
		m.cacheAuthResult(ctx, cacheKey, false)
		return false
	}

	// Check account expiration
	if user.ExpiresAt != nil && time.Now().After(*user.ExpiresAt) {
		m.logger.Warn("Authentication failed: user account expired", 
			zap.String("username", username))
		m.cacheAuthResult(ctx, cacheKey, false)
		return false
	}

	// Check IP whitelist if configured
	if len(user.AllowedIPs) > 0 && clientIP != "" {
		if !m.isIPAllowed(clientIP, user.AllowedIPs) {
			m.logger.Warn("Authentication failed: IP not in whitelist", 
				zap.String("username", username), zap.String("ip", clientIP))
			m.cacheAuthResult(ctx, cacheKey, false)
			return false
		}
	}

	perm, ok := m.cfg.GetPermission(username)
	if !ok {
		m.logger.Warn("Authentication failed: no permissions found", 
			zap.String("username", username))
		m.cacheAuthResult(ctx, cacheKey, false)
		return false
	}

	// Check database access
	if perm.Database != "*" && perm.Database != database {
		m.logger.Warn("Authentication failed: database access denied", 
			zap.String("username", username), zap.String("database", database))
		m.cacheAuthResult(ctx, cacheKey, false)
		return false
	}

	// Check permission expiration
	if perm.TimeLimit != nil && time.Now().After(*perm.TimeLimit) {
		m.logger.Warn("Authentication failed: database access expired", 
			zap.String("username", username), zap.String("database", database))
		m.cacheAuthResult(ctx, cacheKey, false)
		return false
	}

	m.cacheAuthResult(ctx, cacheKey, true)
	return true
}

func (m *Manager) Authorize(ctx context.Context, username, database, table string, isWrite bool) bool {
	cacheKey := fmt.Sprintf("articdbm:authz:%s:%s:%s:%t", username, database, table, isWrite)
	
	cached, err := m.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		return cached == "allowed"
	}

	perm, ok := m.cfg.GetPermission(username)
	if !ok {
		m.cacheAuthResult(ctx, cacheKey, false)
		return false
	}

	if perm.Database != "*" && perm.Database != database {
		m.cacheAuthResult(ctx, cacheKey, false)
		return false
	}

	if table != "" && perm.Table != "*" && perm.Table != table {
		m.cacheAuthResult(ctx, cacheKey, false)
		return false
	}

	action := "read"
	if isWrite {
		action = "write"
	}

	for _, allowedAction := range perm.Actions {
		if allowedAction == action || allowedAction == "*" {
			m.cacheAuthResult(ctx, cacheKey, true)
			return true
		}
	}

	m.cacheAuthResult(ctx, cacheKey, false)
	return false
}

func (m *Manager) cacheAuthResult(ctx context.Context, key string, allowed bool) {
	value := "denied"
	if allowed {
		value = "allowed"
	}
	
	m.redis.Set(ctx, key, value, 5*time.Minute)
}

func (m *Manager) HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

func (m *Manager) ValidatePassword(username, password string) bool {
	user, ok := m.cfg.GetUser(username)
	if !ok {
		return false
	}

	return user.PasswordHash == m.HashPassword(password)
}

func (m *Manager) SyncUsersFromManager(ctx context.Context) error {
	usersData, err := m.redis.Get(ctx, "articdbm:manager:users").Result()
	if err != nil {
		return err
	}

	var users map[string]*config.User
	if err := json.Unmarshal([]byte(usersData), &users); err != nil {
		return err
	}

	m.cfg.Users = users
	m.logger.Info("Synced users from manager", zap.Int("count", len(users)))
	
	return nil
}

func (m *Manager) SyncPermissionsFromManager(ctx context.Context) error {
	permsData, err := m.redis.Get(ctx, "articdbm:manager:permissions").Result()
	if err != nil {
		return err
	}

	var perms map[string]*config.Permission
	if err := json.Unmarshal([]byte(permsData), &perms); err != nil {
		return err
	}

	m.cfg.Permissions = perms
	m.logger.Info("Synced permissions from manager", zap.Int("count", len(perms)))
	
	return nil
}

// API Key Authentication Methods

func (m *Manager) ValidateAPIKey(ctx context.Context, apiKey, database, dbType string) (string, bool) {
	if apiKey == "" {
		return "", false
	}

	// Check all users for matching API key
	for username, user := range m.cfg.Users {
		if user.APIKey == apiKey && user.Enabled {
			// Check if user can access this database
			if m.AuthenticateWithIP(ctx, username, database, dbType, "") {
				m.logger.Info("API key authentication successful", 
					zap.String("username", username))
				return username, true
			}
		}
	}

	m.logger.Warn("API key authentication failed", zap.String("key_prefix", apiKey[:8]+"..."))
	return "", false
}

func (m *Manager) GenerateAPIKey() string {
	// Generate 32 random bytes and base64 encode
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}

// Security Helper Methods

func (m *Manager) isIPAllowed(clientIP string, allowedIPs []string) bool {
	for _, allowedIP := range allowedIPs {
		if strings.Contains(allowedIP, "/") {
			// CIDR notation
			_, network, err := net.ParseCIDR(allowedIP)
			if err != nil {
				continue
			}
			ip := net.ParseIP(clientIP)
			if ip != nil && network.Contains(ip) {
				return true
			}
		} else {
			// Direct IP match
			if clientIP == allowedIP {
				return true
			}
		}
	}
	return false
}

func (m *Manager) CheckTLSRequired(username string) bool {
	user, ok := m.cfg.GetUser(username)
	if !ok {
		return false
	}
	return user.RequireTLS
}

func (m *Manager) GetUserRateLimit(username string) int {
	user, ok := m.cfg.GetUser(username)
	if !ok {
		return 0 // No limit
	}
	return user.RateLimit
}

// Rate Limiting

func (m *Manager) CheckRateLimit(ctx context.Context, username string) bool {
	user, ok := m.cfg.GetUser(username)
	if !ok || user.RateLimit <= 0 {
		return true // No rate limit
	}

	key := fmt.Sprintf("articdbm:rate:%s", username)
	current, err := m.redis.Get(ctx, key).Int()
	if err != nil {
		current = 0
	}

	if current >= user.RateLimit {
		m.logger.Warn("Rate limit exceeded",
			zap.String("username", username),
			zap.Int("limit", user.RateLimit),
			zap.Int("current", current))
		return false
	}

	// Increment counter with 1-second TTL
	pipe := m.redis.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Second)
	pipe.Exec(ctx)

	return true
}

// GetUser retrieves a user by username from the configuration
func (m *Manager) GetUser(ctx context.Context, username string) (*config.User, error) {
	user, ok := m.cfg.GetUser(username)
	if !ok {
		return nil, fmt.Errorf("user %s not found", username)
	}
	return user, nil
}

// HasDatabaseAccess checks if a user has access to a specific database
func (m *Manager) HasDatabaseAccess(ctx context.Context, user *config.User, database string) bool {
	if user == nil || !user.Enabled {
		return false
	}
	perm, ok := m.cfg.GetPermission(user.Username)
	if !ok {
		return false
	}
	return perm.Database == "*" || perm.Database == database
}

// HasTableAccess checks if a user has access to a specific table within a database
func (m *Manager) HasTableAccess(ctx context.Context, user *config.User, database, table string) bool {
	if user == nil || !user.Enabled {
		return false
	}
	perm, ok := m.cfg.GetPermission(user.Username)
	if !ok {
		return false
	}
	if perm.Database != "*" && perm.Database != database {
		return false
	}
	return perm.Table == "*" || perm.Table == table
}

// IsIPAllowed checks if the given IP address is allowed for the user
func (m *Manager) IsIPAllowed(ctx context.Context, user *config.User, ipAddress string) bool {
	if user == nil {
		return false
	}
	if len(user.AllowedIPs) == 0 {
		return true // No IP restrictions
	}
	return m.isIPAllowed(ipAddress, user.AllowedIPs)
}