// Package config loads process configuration from the environment, 12-factor style.
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Port            string
	DatabaseURL     string
	ShutdownTimeout time.Duration
	DBPingTimeout   time.Duration
	// OTLPEndpoint is the standard OTel env var name. Empty means "export
	// traces to stdout" - see internal/tracing.
	OTLPEndpoint string
}

func Load() (Config, error) {
	cfg := Config{
		Port:            getEnv("PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		ShutdownTimeout: 15 * time.Second,
		DBPingTimeout:   2 * time.Second,
		OTLPEndpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
