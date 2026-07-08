package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

// Config holds the controller configuration
type Config struct {
	// Database configuration
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSL      string

	// Kubernetes configuration
	KubeConfig         string
	InCluster          bool
	WatchAllNamespaces bool
	NamespacePrefix    string

	// Controller configuration
	ReconcileInterval time.Duration
	WorkerCount       int
	MaxRetries        int
	BackoffBase       time.Duration
	BackoffMax        time.Duration

	// Logging configuration
	LogLevel  string
	LogFormat string

	// Feature flags
	EnableMetrics     bool
	MetricsPort       int
	EnableHealthCheck bool
	HealthCheckPort   int
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{
		// Database defaults
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnvInt("DB_PORT", 5432),
		DBUser:     getEnv("DB_USER", "nest"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "nest"),
		DBSSL:      getEnv("DB_SSL_MODE", "disable"),

		// Kubernetes defaults
		KubeConfig:         getEnv("KUBECONFIG", ""),
		InCluster:          getEnvBool("IN_CLUSTER", true),
		WatchAllNamespaces: getEnvBool("WATCH_ALL_NAMESPACES", false),
		NamespacePrefix:    getEnv("NAMESPACE_PREFIX", "nest-team-"),

		// Controller defaults
		ReconcileInterval: getEnvDuration("RECONCILE_INTERVAL", 30*time.Second),
		WorkerCount:       getEnvInt("WORKER_COUNT", 5),
		MaxRetries:        getEnvInt("MAX_RETRIES", 3),
		BackoffBase:       getEnvDuration("BACKOFF_BASE", 5*time.Second),
		BackoffMax:        getEnvDuration("BACKOFF_MAX", 5*time.Minute),

		// Logging defaults
		LogLevel:  getEnv("LOG_LEVEL", "info"),
		LogFormat: getEnv("LOG_FORMAT", "json"),

		// Feature flags
		EnableMetrics:     getEnvBool("ENABLE_METRICS", true),
		MetricsPort:       getEnvInt("METRICS_PORT", 9090),
		EnableHealthCheck: getEnvBool("ENABLE_HEALTH_CHECK", true),
		HealthCheckPort:   getEnvInt("HEALTH_CHECK_PORT", 8080),
	}

	// Validate required fields
	if config.DBPassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD is required")
	}

	return config, nil
}

// GetDSN returns the database connection string
func (c *Config) GetDSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSL)
}

// SetupLogging configures the logging system
func (c *Config) SetupLogging() error {
	level, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	logrus.SetLevel(level)

	if c.LogFormat == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
	} else {
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	}

	return nil
}

// Helper functions for environment variables

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
