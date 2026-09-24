package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Database DatabaseConfig
	AppEnv   string
	Server   ServerConfig
	Bank     BankConfig
}

type ServerConfig struct {
	Port           string
	AllowedOrigins []string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string

	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

type BankConfig struct {
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
}

func Load() (*Config, error) {
	readTimeout, err := getEnvAsDuration("SERVER_READ_TIMEOUT", 15*time.Second)
	if err != nil {
		return nil, err
	}
	writeTimeout, err := getEnvAsDuration("SERVER_WRITE_TIMEOUT", 15*time.Second)
	if err != nil {
		return nil, err
	}
	idleTimeout, err := getEnvAsDuration("SERVER_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return nil, err
	}

	maxConns, err := getEnvAsInt32("DB_MAX_OPEN_CONNS", 25)
	if err != nil {
		return nil, err
	}
	minConns, err := getEnvAsInt32("DB_MIN_OPEN_CONNS", 5)
	if err != nil {
		return nil, err
	}
	maxConnLifetime, err := getEnvAsDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute)
	if err != nil {
		return nil, err
	}
	maxConnIdleTime, err := getEnvAsDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute)
	if err != nil {
		return nil, err
	}
	bankTimeout, err := getEnvAsDuration("BANK_TIMEOUT", 10*time.Second) // comfortably above the mock's 2000ms max injected latency
	if err != nil {
		return nil, err
	}
	bankMaxRetries, err := getEnvAsInt32("BANK_MAX_RETRIES", 3)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: ServerConfig{
			Port:           getEnv("APP_PORT", "8080"),
			ReadTimeout:    readTimeout,
			WriteTimeout:   writeTimeout,
			IdleTimeout:    idleTimeout,
			AllowedOrigins: getEnvAsSlice("CORS_ALLOWED_ORIGINS", nil),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "5432"),
			User:            getEnv("DB_USER", "postgres"),
			Password:        os.Getenv("DB_PASSWORD"),
			DBName:          getEnv("DB_NAME", "mockbank"),
			SSLMode:         getEnv("DB_SSLMODE", "disable"),
			MaxConns:        maxConns,
			MinConns:        minConns,
			MaxConnLifetime: maxConnLifetime,
			MaxConnIdleTime: maxConnIdleTime,
		},
		Bank: BankConfig{
			BaseURL:    getEnv("BANK_BASE_URL", "http://localhost:8787"),
			Timeout:    bankTimeout,
			MaxRetries: int(bankMaxRetries),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}

	if c.Database.Host == "" {
		return fmt.Errorf("database host cannot be empty")
	}
	if c.Database.User == "" {
		return fmt.Errorf("database user cannot be empty")
	}
	if c.Database.Password == "" {
		return fmt.Errorf("database password cannot be empty (set DB_PASSWORD)")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database name cannot be empty")
	}
	if c.Database.MaxConns <= 0 {
		return fmt.Errorf("DB_MAX_OPEN_CONNS must be > 0, got %d", c.Database.MaxConns)
	}
	if c.Database.MinConns < 0 {
		return fmt.Errorf("DB_MIN_OPEN_CONNS cannot be negative, got %d", c.Database.MinConns)
	}
	if c.Database.MinConns > c.Database.MaxConns {
		return fmt.Errorf("DB_MIN_OPEN_CONNS (%d) cannot exceed DB_MAX_OPEN_CONNS (%d)",
			c.Database.MinConns, c.Database.MaxConns)
	}
	if c.AppEnv == "production" && len(c.Server.AllowedOrigins) == 0 {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS must be set in production")
	}

	return nil
}

func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// URL returns a URL-style DSN for tools that require it (golang-migrate CLI, psql, etc.)
func (c *DatabaseConfig) URL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt32(key string, defaultValue int32) (int32, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	value, err := strconv.ParseInt(valueStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q: %w", key, valueStr, err)
	}
	return int32(value), nil
}

// getEnvAsDuration takes the default as a real time.Duration, not a string,
// so a typo in a hardcoded default can never silently produce 0.
func getEnvAsDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	d, err := time.ParseDuration(valueStr)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q: %w", key, valueStr, err)
	}
	return d, nil
}

func getEnvAsSlice(key string, defaultValue []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	parts := strings.Split(v, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
