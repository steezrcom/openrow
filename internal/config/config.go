package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL      string
	AnthropicAPIKey  string
	HTTPAddr         string
	LogLevel         string
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		HTTPAddr:        getOr("HTTP_ADDR", ":8080"),
		LogLevel:        getOr("LOG_LEVEL", "info"),
	}
	if c.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	return c, nil
}

func getOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
