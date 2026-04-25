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

	// Embed controls which embedding provider is built at startup.
	Embed embed.Config

	// RepoRoot is the absolute filesystem path of the working tree the
	// agent pins against. Optional; set via PINDOC_REPO_ROOT. When
	// populated, artifact.propose statically verifies each kind="code"
	// pin's path against the tree and returns a PIN_PATH_NOT_FOUND
	// warning on any missing files. Empty disables the check — the V1.5
	// git-pinner will replace this with a real repo-aware validator.
	RepoRoot string

	// Author identity dual (Decision `decision-author-identity-dual`,
	// migration 0014). V1 single-user self-host reads these env vars at
	// startup and upserts a row into the `users` table so every write
	// from this MCP session can be tagged with the human author_user_id
	// alongside the agent's author_id label. Empty UserName skips the
	// upsert and artifact.propose leaves author_user_id NULL — Reader
	// falls back to "(unknown) via {agent_id}" byline. V1.5 GitHub
	// OAuth replaces these env vars with session-resolved principals.
	UserName  string
	UserEmail string
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
		RepoRoot:     env("PINDOC_REPO_ROOT", ""),
		UserName:     strings.TrimSpace(env("PINDOC_USER_NAME", "")),
		UserEmail:    strings.TrimSpace(env("PINDOC_USER_EMAIL", "")),
		Embed: embed.Config{
			// Empty default → gemma (bundled on-device embeddinggemma-300m).
			// Set explicitly to "stub" for offline unit tests, "http" for
			// external TEI / OpenAI / bge-m3, or "gemma" for clarity.
			Provider:       env("PINDOC_EMBED_PROVIDER", ""),
			GemmaVariant:   env("PINDOC_EMBED_GEMMA_VARIANT", ""),
			ModelDir:       env("PINDOC_EMBED_MODEL_DIR", ""),
			RuntimeDir:     env("PINDOC_EMBED_RUNTIME_DIR", ""),
			RuntimeLib:     env("PINDOC_ONNX_RUNTIME_LIB", ""),
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
