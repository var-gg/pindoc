// Package config loads Pindoc server configuration from env vars.
//
// For Phase 1 we only need the bare minimum: a DB connection string (unused
// until Phase 2) and a log level. A PINDOC.md-driven config file layer
// lands in Phase 5 — until then, everything is env-driven because the
// server is typically launched by Claude Code as a stdio subprocess and
// env is the simplest shared channel.
package config

import (
	"os"
	"strings"
)

type Config struct {
	// DatabaseURL is a Postgres connection string. Unused in Phase 1;
	// Phase 2 brings the schema online.
	DatabaseURL string

	// LogLevel is "debug" | "info" | "warn" | "error".
	LogLevel string

	// UserLanguage hints NOT_READY template selection until Phase 5 loads
	// the real value from PINDOC.md. Default "en".
	UserLanguage string
}

// Load builds a Config from process env vars. It never fails for Phase 1
// usage (there are no required fields); the error return is reserved for
// when real validation lands.
func Load() (*Config, error) {
	return &Config{
		DatabaseURL:  env("PINDOC_DATABASE_URL", "postgres://pindoc:pindoc_dev@localhost:5432/pindoc?sslmode=disable"),
		LogLevel:     env("PINDOC_LOG_LEVEL", "info"),
		UserLanguage: strings.ToLower(env("PINDOC_USER_LANGUAGE", "en")),
	}, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
