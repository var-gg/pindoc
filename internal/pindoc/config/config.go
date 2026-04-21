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
	"strconv"
	"strings"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

type Config struct {
	// DatabaseURL is a Postgres connection string.
	DatabaseURL string

	// LogLevel is "debug" | "info" | "warn" | "error".
	LogLevel string

	// UserLanguage hints NOT_READY template selection until Phase 5 loads
	// the real value from PINDOC.md. Default "en".
	UserLanguage string

	// ProjectSlug is the MCP server's active scope and the HTTP API's default
	// project. URL shares without /p/{project}/ prefix redirect here; MCP
	// write tools operate on this project unless a future session overrides
	// via PINDOC_PROJECT.
	ProjectSlug string

	// MultiProject toggles the Web UI's project switcher. Default false so
	// single-project installs stay chromeless; flip to true once pindoc.project.create
	// is run to introduce a second project.
	MultiProject bool

	// Embed controls which embedding provider is built at startup.
	Embed embed.Config
}

// Load builds a Config from process env vars. It never fails for Phase 1
// usage (there are no required fields); the error return is reserved for
// when real validation lands.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:  env("PINDOC_DATABASE_URL", "postgres://pindoc:pindoc_dev@localhost:5432/pindoc?sslmode=disable"),
		LogLevel:     env("PINDOC_LOG_LEVEL", "info"),
		UserLanguage: strings.ToLower(env("PINDOC_USER_LANGUAGE", "en")),
		ProjectSlug:  env("PINDOC_PROJECT", "pindoc"),
		MultiProject: envBool("PINDOC_MULTI_PROJECT", false),
		Embed: embed.Config{
			Provider:       env("PINDOC_EMBED_PROVIDER", "stub"),
			Endpoint:       env("PINDOC_EMBED_ENDPOINT", ""),
			APIKey:         env("PINDOC_EMBED_API_KEY", ""),
			Model:          env("PINDOC_EMBED_MODEL", ""),
			Dimension:      envInt("PINDOC_EMBED_DIM", 0),
			MaxTokens:      envInt("PINDOC_EMBED_MAX_TOKENS", 0),
			Multilingual:   envBool("PINDOC_EMBED_MULTILINGUAL", true),
			Timeout:        envDuration("PINDOC_EMBED_TIMEOUT", 0),
			PrefixQuery:    env("PINDOC_EMBED_PREFIX_QUERY", ""),
			PrefixDocument: env("PINDOC_EMBED_PREFIX_DOCUMENT", ""),
		},
	}
	return cfg, nil
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
