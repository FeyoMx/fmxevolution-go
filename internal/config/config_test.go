package config

import (
	"strings"
	"testing"
)

func TestConfigValidateRequiresDeploymentEnv(t *testing.T) {
	cfg := &Config{}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	message := err.Error()
	for _, key := range []string{"DATABASE_URL", "JWT_SECRET", "PORT or HTTP_ADDRESS"} {
		if !strings.Contains(message, key) {
			t.Fatalf("expected missing %s in error, got %q", key, message)
		}
	}
}

func TestConfigValidateRejectsInvalidPortAndLogLevel(t *testing.T) {
	cfg := validConfig()
	cfg.HTTP.Port = "abc"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "PORT") {
		t.Fatalf("expected PORT validation error, got %v", err)
	}

	cfg = validConfig()
	cfg.LogLevel = "verbose"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("expected LOG_LEVEL validation error, got %v", err)
	}
}

func TestConfigValidateAcceptsLegacyHTTPAddress(t *testing.T) {
	cfg := validConfig()
	cfg.HTTP.Port = ""
	cfg.HTTP.Address = ":8085"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected HTTP_ADDRESS fallback to validate, got %v", err)
	}
}

func TestConfigValidateRequiresLongProductionJWTSecret(t *testing.T) {
	cfg := validConfig()
	cfg.AppEnv = "production"
	cfg.Auth.JWTSecret = "short-secret"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("expected JWT_SECRET length validation error, got %v", err)
	}
}

func validConfig() *Config {
	return &Config{
		AppEnv:   "development",
		LogLevel: "info",
		HTTP: HTTPConfig{
			Port: "8080",
		},
		Database: DatabaseConfig{
			URL: "postgres://user:pass@localhost:5432/db?sslmode=disable",
		},
		Auth: AuthConfig{
			JWTSecret: "development-secret",
		},
	}
}
