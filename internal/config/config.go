package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HTTPConfig struct {
	Address         string
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type AuthConfig struct {
	JWTSecret  string
	TokenTTL   time.Duration
	RefreshTTL time.Duration
}

type BroadcastConfig struct {
	Workers        int
	QueueBatchSize int
	RatePerSecond  int
}

type RateLimitConfig struct {
	Backend               string
	MessagesPerMinute     int
	BroadcastPerHour      int
	WebhookCallsPerMinute int
}

type AIConfig struct {
	OpenAIAPIKey string
	BaseURL      string
	Model        string
	Timeout      time.Duration
	Workers      int
	MemoryLimit  int
}

type Config struct {
	AppEnv    string
	LogLevel  string
	HTTP      HTTPConfig
	Database  DatabaseConfig
	Auth      AuthConfig
	Broadcast BroadcastConfig
	RateLimit RateLimitConfig
	AI        AIConfig
}

var (
	loadOnce sync.Once
	cached   *Config
	loadErr  error
)

func Load() (*Config, error) {
	loadOnce.Do(func() {
		cfg := &Config{
			AppEnv:   getEnv("APP_ENV", "development"),
			LogLevel: strings.ToLower(strings.TrimSpace(getEnv("LOG_LEVEL", ""))),
			HTTP: HTTPConfig{
				Address:         strings.TrimSpace(getEnv("HTTP_ADDRESS", "")),
				Port:            strings.TrimSpace(getEnv("PORT", "")),
				ReadTimeout:     getDuration("HTTP_READ_TIMEOUT", 15*time.Second),
				WriteTimeout:    getDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
				ShutdownTimeout: getDuration("HTTP_SHUTDOWN_TIMEOUT", 20*time.Second),
			},
			Database: DatabaseConfig{
				URL:             getEnv("DATABASE_URL", ""),
				MaxOpenConns:    getInt("DATABASE_MAX_OPEN_CONNS", 20),
				MaxIdleConns:    getInt("DATABASE_MAX_IDLE_CONNS", 5),
				ConnMaxLifetime: getDuration("DATABASE_CONN_MAX_LIFETIME", 30*time.Minute),
			},
			Auth: AuthConfig{
				JWTSecret:  getEnv("JWT_SECRET", ""),
				TokenTTL:   getDuration("JWT_TTL", 24*time.Hour),
				RefreshTTL: getDuration("JWT_REFRESH_TTL", 168*time.Hour),
			},
			Broadcast: BroadcastConfig{
				Workers:        getInt("BROADCAST_WORKERS", 4),
				QueueBatchSize: getInt("BROADCAST_QUEUE_BATCH_SIZE", 8),
				RatePerSecond:  getInt("BROADCAST_RATE_PER_SECOND", 2),
			},
			RateLimit: RateLimitConfig{
				Backend:               getEnv("RATE_LIMIT_BACKEND", "memory"),
				MessagesPerMinute:     getInt("RATE_LIMIT_MESSAGES_PER_MINUTE", 60),
				BroadcastPerHour:      getInt("RATE_LIMIT_BROADCAST_PER_HOUR", 120),
				WebhookCallsPerMinute: getInt("RATE_LIMIT_WEBHOOK_CALLS_PER_MINUTE", 120),
			},
			AI: AIConfig{
				OpenAIAPIKey: getEnv("OPENAI_API_KEY", ""),
				BaseURL:      getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
				Model:        getEnv("OPENAI_MODEL", "gpt-4o-mini"),
				Timeout:      getDuration("AI_TIMEOUT", 15*time.Second),
				Workers:      getInt("AI_WORKERS", 2),
				MemoryLimit:  getInt("AI_MEMORY_LIMIT", 12),
			},
		}

		if cfg.LogLevel == "" {
			if cfg.AppEnv == "development" {
				cfg.LogLevel = "debug"
			} else {
				cfg.LogLevel = "info"
			}
		}
		if cfg.HTTP.Port != "" {
			cfg.HTTP.Address = ":" + cfg.HTTP.Port
		}
		if cfg.HTTP.Address == "" && cfg.AppEnv == "development" {
			cfg.HTTP.Address = ":8080"
		}

		if err := cfg.Validate(); err != nil {
			loadErr = err
			return
		}

		cached = cfg
	})

	return cached, loadErr
}

func (c *Config) Validate() error {
	var missing []string
	if strings.TrimSpace(c.Database.URL) == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if strings.TrimSpace(c.Auth.JWTSecret) == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if strings.TrimSpace(c.HTTP.Port) == "" && strings.TrimSpace(c.HTTP.Address) == "" {
		missing = append(missing, "PORT or HTTP_ADDRESS")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	if strings.TrimSpace(c.HTTP.Port) != "" {
		port, err := strconv.Atoi(c.HTTP.Port)
		if err != nil || port <= 0 || port > 65535 {
			return fmt.Errorf("PORT must be an integer between 1 and 65535")
		}
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error")
	}

	if strings.EqualFold(strings.TrimSpace(c.AppEnv), "production") && len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters when APP_ENV=production")
	}

	return nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}
