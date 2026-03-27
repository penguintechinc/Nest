package config

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
)

type Config struct {
	mu sync.RWMutex

	Version    string
	ProxyPort  int
	TLSEnabled bool
	TLSCert    string
	TLSKey     string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	MySQLEnabled   bool
	MySQLPort      int
	MySQLBackends  []Backend

	PostgreSQLEnabled bool
	PostgreSQLPort    int
	PostgreSQLBackends []Backend

	MSSQLEnabled   bool
	MSSQLPort      int
	MSSQLBackends  []Backend

	MongoDBEnabled   bool
	MongoDBPort      int
	MongoDBBackends  []Backend

	RedisProxyEnabled bool
	RedisProxyPort    int
	RedisBackends     []Backend

	MetricsPort int

	SQLInjectionDetection bool
	MaxConnections        int
	ConnectionTimeout     time.Duration
	QueryTimeout          time.Duration

	// Database/Account blocking configuration
	DefaultDatabaseBlocking bool
	CustomBlockingEnabled   bool
	BlockingEnabled         bool
	SeedDefaultBlocked      bool

	ClusterMode      bool
	ClusterRedisAddr string

	// XDP/AF_XDP Configuration
	XDPEnabled         bool
	XDPInterface       string
	XDPRateLimitPPS    uint64
	XDPBurstLimit      uint32
	AFXDPEnabled       bool
	AFXDPBatchSize     int
	XDPCacheSize       uint32
	XDPCacheTTL        uint32

	Users        map[string]*User
	Permissions  map[string]*Permission
}

type Backend struct {
	Host     string
	Port     int
	Type     string // "read" or "write"
	Weight   int
	TLS      bool
	User     string
	Password string
	Database string
}

type User struct {
	Username     string
	PasswordHash string
	APIKey       string    // API key for programmatic access
	Enabled      bool
	RequireTLS   bool      // Force TLS for this user
	AllowedIPs   []string  // IP whitelist for additional security
	RateLimit    int       // Requests per second limit
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ExpiresAt    *time.Time // Optional account expiration
}

type Permission struct {
	UserID     string
	Database   string
	Table      string
	Actions    []string // "read", "write", "admin"
	TimeLimit  *time.Time // Optional access expiration per database
	MaxQueries int      // Query limit per hour for this database
}

func LoadConfig() *Config {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/articdbm/")
	viper.AddConfigPath(".")

	viper.AutomaticEnv()

	cfg := &Config{
		Version:               "1.0.0",
		ProxyPort:             getEnvAsInt("PROXY_PORT", 8080),
		TLSEnabled:            getEnvAsBool("TLS_ENABLED", false),
		TLSCert:               getEnv("TLS_CERT", ""),
		TLSKey:                getEnv("TLS_KEY", ""),
		RedisAddr:             getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:         getEnv("REDIS_PASSWORD", ""),
		RedisDB:               getEnvAsInt("REDIS_DB", 0),
		MySQLEnabled:          getEnvAsBool("MYSQL_ENABLED", true),
		MySQLPort:             getEnvAsInt("MYSQL_PORT", 3306),
		PostgreSQLEnabled:     getEnvAsBool("POSTGRESQL_ENABLED", true),
		PostgreSQLPort:        getEnvAsInt("POSTGRESQL_PORT", 5432),
		MSSQLEnabled:          getEnvAsBool("MSSQL_ENABLED", true),
		MSSQLPort:             getEnvAsInt("MSSQL_PORT", 1433),
		MongoDBEnabled:        getEnvAsBool("MONGODB_ENABLED", true),
		MongoDBPort:           getEnvAsInt("MONGODB_PORT", 27017),
		RedisProxyEnabled:     getEnvAsBool("REDIS_PROXY_ENABLED", true),
		RedisProxyPort:        getEnvAsInt("REDIS_PROXY_PORT", 6379),
		MetricsPort:           getEnvAsInt("METRICS_PORT", 9090),
		SQLInjectionDetection: getEnvAsBool("SQL_INJECTION_DETECTION", true),
		MaxConnections:        getEnvAsInt("MAX_CONNECTIONS", 1000),
		ConnectionTimeout:     time.Duration(getEnvAsInt("CONNECTION_TIMEOUT", 30)) * time.Second,
		QueryTimeout:          time.Duration(getEnvAsInt("QUERY_TIMEOUT", 60)) * time.Second,
		
		// Database/Account blocking configuration
		DefaultDatabaseBlocking: getEnvAsBool("DEFAULT_DATABASE_BLOCKING", true),
		CustomBlockingEnabled:   getEnvAsBool("CUSTOM_BLOCKING_ENABLED", true),
		BlockingEnabled:         getEnvAsBool("BLOCKING_ENABLED", true),
		SeedDefaultBlocked:      getEnvAsBool("SEED_DEFAULT_BLOCKED", true),
		
		ClusterMode:           getEnvAsBool("CLUSTER_MODE", false),
		ClusterRedisAddr:      getEnv("CLUSTER_REDIS_ADDR", ""),

		// XDP/AF_XDP Configuration
		XDPEnabled:         getEnvAsBool("XDP_ENABLED", true),
		XDPInterface:       getEnv("XDP_INTERFACE", "eth0"),
		XDPRateLimitPPS:    uint64(getEnvAsInt("XDP_RATE_LIMIT_PPS", 100000000)),
		XDPBurstLimit:      uint32(getEnvAsInt("XDP_BURST_LIMIT", 10000)),
		AFXDPEnabled:       getEnvAsBool("AFXDP_ENABLED", true),
		AFXDPBatchSize:     getEnvAsInt("AFXDP_BATCH_SIZE", 64),
		XDPCacheSize:       uint32(getEnvAsInt("XDP_CACHE_SIZE", 1048576)),
		XDPCacheTTL:        uint32(getEnvAsInt("XDP_CACHE_TTL", 300)),

		Users:                 make(map[string]*User),
		Permissions:           make(map[string]*Permission),
	}

	if err := viper.ReadInConfig(); err == nil {
		viper.Unmarshal(cfg)
	}

	cfg.loadBackends()

	return cfg
}

func (c *Config) loadBackends() {
	if mysqlBackends := getEnv("MYSQL_BACKENDS", ""); mysqlBackends != "" {
		c.MySQLBackends = parseBackends(mysqlBackends)
	}
	if postgresqlBackends := getEnv("POSTGRESQL_BACKENDS", ""); postgresqlBackends != "" {
		c.PostgreSQLBackends = parseBackends(postgresqlBackends)
	}
	if mssqlBackends := getEnv("MSSQL_BACKENDS", ""); mssqlBackends != "" {
		c.MSSQLBackends = parseBackends(mssqlBackends)
	}
	if mongodbBackends := getEnv("MONGODB_BACKENDS", ""); mongodbBackends != "" {
		c.MongoDBBackends = parseBackends(mongodbBackends)
	}
	if redisBackends := getEnv("REDIS_BACKENDS", ""); redisBackends != "" {
		c.RedisBackends = parseBackends(redisBackends)
	}
}

func parseBackends(backendsStr string) []Backend {
	var backends []Backend
	if err := json.Unmarshal([]byte(backendsStr), &backends); err != nil {
		return []Backend{}
	}
	return backends
}

func (c *Config) RefreshFromRedis(ctx context.Context, client *redis.Client) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	usersData, err := client.Get(ctx, "articdbm:users").Result()
	if err == nil {
		var users map[string]*User
		if err := json.Unmarshal([]byte(usersData), &users); err == nil {
			c.Users = users
		}
	}

	permsData, err := client.Get(ctx, "articdbm:permissions").Result()
	if err == nil {
		var perms map[string]*Permission
		if err := json.Unmarshal([]byte(permsData), &perms); err == nil {
			c.Permissions = perms
		}
	}

	backendsData, err := client.Get(ctx, "articdbm:backends").Result()
	if err == nil {
		var backends map[string][]Backend
		if err := json.Unmarshal([]byte(backendsData), &backends); err == nil {
			if b, ok := backends["mysql"]; ok {
				c.MySQLBackends = b
			}
			if b, ok := backends["postgresql"]; ok {
				c.PostgreSQLBackends = b
			}
			if b, ok := backends["mssql"]; ok {
				c.MSSQLBackends = b
			}
			if b, ok := backends["mongodb"]; ok {
				c.MongoDBBackends = b
			}
			if b, ok := backends["redis"]; ok {
				c.RedisBackends = b
			}
		}
	}

	return nil
}

func (c *Config) GetUser(username string) (*User, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	user, ok := c.Users[username]
	return user, ok
}

func (c *Config) GetPermission(userID string) (*Permission, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	perm, ok := c.Permissions[userID]
	return perm, ok
}

func (c *Config) GetReadBackends(dbType string) []Backend {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var allBackends []Backend
	switch dbType {
	case "mysql":
		allBackends = c.MySQLBackends
	case "postgresql":
		allBackends = c.PostgreSQLBackends
	case "mssql":
		allBackends = c.MSSQLBackends
	case "mongodb":
		allBackends = c.MongoDBBackends
	case "redis":
		allBackends = c.RedisBackends
	default:
		return []Backend{}
	}

	var readBackends []Backend
	for _, b := range allBackends {
		if b.Type == "read" || b.Type == "" {
			readBackends = append(readBackends, b)
		}
	}

	if len(readBackends) == 0 {
		return allBackends
	}

	return readBackends
}

func (c *Config) GetWriteBackends(dbType string) []Backend {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var allBackends []Backend
	switch dbType {
	case "mysql":
		allBackends = c.MySQLBackends
	case "postgresql":
		allBackends = c.PostgreSQLBackends
	case "mssql":
		allBackends = c.MSSQLBackends
	case "mongodb":
		allBackends = c.MongoDBBackends
	case "redis":
		allBackends = c.RedisBackends
	default:
		return []Backend{}
	}

	var writeBackends []Backend
	for _, b := range allBackends {
		if b.Type == "write" || b.Type == "" {
			writeBackends = append(writeBackends, b)
		}
	}

	if len(writeBackends) == 0 {
		return allBackends
	}

	return writeBackends
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if value, err := strconv.ParseBool(valueStr); err == nil {
		return value
	}
	return defaultValue
}