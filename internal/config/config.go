package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL     string
	AnthropicAPIKey string // legacy fallback; per-tenant LLM config is preferred
	SecretKey       string // base64-encoded 32-byte key for secret encryption
	HTTPAddr        string
	LogLevel        string
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		SecretKey:       os.Getenv("OPENROW_SECRET_KEY"),
		HTTPAddr:        getOr("HTTP_ADDR", ":8080"),
		LogLevel:        getOr("LOG_LEVEL", "info"),
	}
	if c.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if c.SecretKey == "" {
		return nil, errors.New(
			"OPENROW_SECRET_KEY is required " +
				"(generate with `openssl rand -base64 32`; " +
				"needed to encrypt stored API keys and connector secrets)")
	}
	return c, nil
}

func getOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
