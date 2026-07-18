// Package config provides centralized application configuration.
// All values are read from environment variables with sane defaults.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the application-wide configuration.
type Config struct {
	// Server configuration
	ServerPort   string
	AdminPort    string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// PostgreSQL configuration
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string

	// Redis configuration
	RedisAddr     string
	RedisPassword string

	// Rate limiter defaults
	DefaultRateLimit    int
	DefaultWindowSecs   int
	CircuitBreakerLimit int32

	// Logger pipeline tuning
	LogBufferSize    int
	LogFlushInterval time.Duration
	LogBatchSize     int
	LogWorkerCount   int

	// Mock API configuration
	MockAPIURL string
}

// Load reads environment variables and returns a validated Config.
func Load() *Config {
	return &Config{
		ServerPort:   envStr("SERVER_PORT", "8080"),
		AdminPort:    envStr("ADMIN_PORT", "8081"),
		ReadTimeout:  envDuration("READ_TIMEOUT_MS", 5000),
		WriteTimeout: envDuration("WRITE_TIMEOUT_MS", 10000),

		PostgresHost:     envStr("POSTGRES_HOST", "localhost"),
		PostgresPort:     envStr("POSTGRES_PORT", "5432"),
		PostgresUser:     envStr("POSTGRES_USER", "postgres"),
		PostgresPassword: envStr("POSTGRES_PASSWORD", "password"),
		PostgresDB:       envStr("POSTGRES_DB", "ratelimiter"),

		RedisAddr:     envStr("REDIS_ADDR", "localhost:6379"),
		RedisPassword: envStr("REDIS_PASSWORD", ""),

		DefaultRateLimit:    envInt("DEFAULT_RATE_LIMIT", 60),
		DefaultWindowSecs:   envInt("DEFAULT_WINDOW_SECS", 60),
		CircuitBreakerLimit: int32(envInt("CIRCUIT_BREAKER_LIMIT", 3)),

		LogBufferSize:    envInt("LOG_BUFFER_SIZE", 10000),
		LogFlushInterval: envDuration("LOG_FLUSH_INTERVAL_MS", 2000),
		LogBatchSize:     envInt("LOG_BATCH_SIZE", 500),
		LogWorkerCount:   envInt("LOG_WORKER_COUNT", 4),

		MockAPIURL: envStr("MOCK_API_URL", "http://localhost:9090"),
	}
}

// DSN returns the PostgreSQL data source connection string.
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		c.PostgresHost, c.PostgresPort, c.PostgresUser, c.PostgresPassword, c.PostgresDB,
	)
}

// String provides a redacted summary suitable for startup logs.
func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{ServerPort: %s, AdminPort: %s, Redis: %s, Postgres: %s@%s:%s/%s, LogBuffer: %d, LogWorkers: %d}",
		c.ServerPort, c.AdminPort, c.RedisAddr,
		c.PostgresUser, c.PostgresHost, c.PostgresPort, c.PostgresDB,
		c.LogBufferSize, c.LogWorkerCount,
	)
}

// GetEnv reads a string environment variable with a fallback default.
// Exported so other packages (e.g. tests) can reuse it.
func GetEnv(key, fallback string) string {
	return envStr(key, fallback)
}

func envStr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallbackMs int) time.Duration {
	return time.Duration(envInt(key, fallbackMs)) * time.Millisecond
}
