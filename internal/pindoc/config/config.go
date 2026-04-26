// Package config loads Pindoc server configuration from env vars.
//
// For Phase 1 we only need the bare minimum: a DB connection string (unused
// until Phase 2) and a log level. A PINDOC.md-driven config file layer
// lands in Phase 5 — until then, everything is env-driven because the
// server is typically launched by Claude Code as a stdio subprocess and
// env is the simplest shared channel.
package config

import (
	"fmt"
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

	// AuthMode selects the resolver family the server boots with.
	// V1 supports trusted_local only; the other enum values are accepted
	// at config-parse time so operators get a stable error when they try
	// to enable a future mode before its implementation lands.
	AuthMode AuthMode

	// UserLanguage hints NOT_READY template selection until Phase 5 loads
	// the real value from PINDOC.md. Default "en".
	UserLanguage string

	// ReceiptExemptionLimit is the create-path search_receipt bootstrap
	// allowance per area/author before artifact.propose requires an
	// explicit search/context receipt again. Default 5. Set to 0 to
	// disable the exemption while keeping normal search_receipt gating.
	ReceiptExemptionLimit int

	// ProjectSlug is the MCP server's active scope and the HTTP API's default
	// project. URL shares without /p/{project}/ prefix redirect here; MCP
	// write tools operate on this project unless a future session overrides
	// via PINDOC_PROJECT.
	ProjectSlug string

	// Embed controls which embedding provider is built at startup.
	Embed embed.Config

	// Summary controls the optional source-bound LLM used by the Today
	// briefing. Empty Endpoint means the Reader always uses the deterministic
	// rule-based template and still writes/reads the summary cache.
	Summary SummaryConfig

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

type SummaryConfig struct {
	Endpoint      string
	APIKey        string
	Model         string
	DailyTokenCap int
	GroupCap      int
	Timeout       time.Duration
}

type AuthMode string

const (
	AuthModeTrustedLocal   AuthMode = "trusted_local"
	AuthModePublicReadonly AuthMode = "public_readonly"
	AuthModeSingleUser     AuthMode = "single_user"
	AuthModeOAuthGitHub    AuthMode = "oauth_github"
)

func (m AuthMode) Valid() bool {
	switch m {
	case AuthModeTrustedLocal, AuthModePublicReadonly, AuthModeSingleUser, AuthModeOAuthGitHub:
		return true
	default:
		return false
	}
}

func ValidAuthModesString() string {
	return strings.Join(validAuthModeStrings(), "|")
}

func parseAuthMode(raw string) (AuthMode, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return AuthModeTrustedLocal, nil
	}
	mode := AuthMode(strings.ToLower(trimmed))
	if !mode.Valid() {
		return "", fmt.Errorf("invalid PINDOC_AUTH_MODE: '%s'. valid: %s", trimmed, ValidAuthModesString())
	}
	return mode, nil
}

func validAuthModeStrings() []string {
	return []string{
		string(AuthModeTrustedLocal),
		string(AuthModePublicReadonly),
		string(AuthModeSingleUser),
		string(AuthModeOAuthGitHub),
	}
}

// Load builds a Config from process env vars and fails fast on invalid enum
// values so a misspelled security mode never silently boots the wrong model.
func Load() (*Config, error) {
	authMode, err := parseAuthMode(env("PINDOC_AUTH_MODE", string(AuthModeTrustedLocal)))
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		DatabaseURL:           env("PINDOC_DATABASE_URL", "postgres://pindoc:pindoc_dev@localhost:5432/pindoc?sslmode=disable"),
		LogLevel:              env("PINDOC_LOG_LEVEL", "info"),
		AuthMode:              authMode,
		UserLanguage:          strings.ToLower(env("PINDOC_USER_LANGUAGE", "en")),
		ReceiptExemptionLimit: envInt("PINDOC_RECEIPT_EXEMPTION_LIMIT", 5),
		ProjectSlug:           env("PINDOC_PROJECT", "pindoc"),
		RepoRoot:              env("PINDOC_REPO_ROOT", ""),
		UserName:              strings.TrimSpace(env("PINDOC_USER_NAME", "")),
		UserEmail:             strings.TrimSpace(env("PINDOC_USER_EMAIL", "")),
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
		Summary: SummaryConfig{
			Endpoint:      env("PINDOC_SUMMARY_LLM_ENDPOINT", ""),
			APIKey:        env("PINDOC_SUMMARY_LLM_API_KEY", ""),
			Model:         env("PINDOC_SUMMARY_LLM_MODEL", ""),
			DailyTokenCap: envInt("PINDOC_SUMMARY_DAILY_TOKEN_CAP", 20000),
			GroupCap:      envInt("PINDOC_SUMMARY_GROUP_CAP", 20),
			Timeout:       envDuration("PINDOC_SUMMARY_LLM_TIMEOUT", 15*time.Second),
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
