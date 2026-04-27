// Package config loads Pindoc server configuration from env vars.
//
// Decision `decision-auth-model-loopback-and-providers` retired the
// 4-mode `PINDOC_AUTH_MODE` enum in favour of three orthogonal env
// axes: BindAddr (where to listen), AuthProviders (which IdPs are
// active), and AllowPublicUnauthenticated (explicit opt-in for
// external exposure without IdP). Loopback bind + empty providers
// matches the historical "single-user self-host" mental model with
// zero config.
package config

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

// ErrPublicWithoutAuth is the boot-time refusal Decision
// `decision-auth-model-loopback-and-providers` § 4 enforces: a
// non-loopback `PINDOC_BIND_ADDR` with empty `PINDOC_AUTH_PROVIDERS`
// and `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED=false` would expose the
// daemon to the network with no authenticated identity at all.
// Treating that as a configuration error and refusing to start makes
// the operator's intent explicit — they must either add an IdP or
// opt in to the public-without-auth network model.
var ErrPublicWithoutAuth = errors.New("config: PINDOC_BIND_ADDR is non-loopback but PINDOC_AUTH_PROVIDERS is empty and PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED is false — set one of them to opt in")

const (
	// DefaultBindAddr matches the "loopback only, no IdP" single-user
	// path. Operators flipping to `0.0.0.0:5830` (or any non-loopback
	// host) must also set AuthProviders or AllowPublicUnauthenticated
	// (Public-Without-Auth Refusal).
	DefaultBindAddr = "127.0.0.1:5830"

	// AuthProviderGitHub is the only provider Pindoc ships with
	// today. Future providers (`google`, `local-password`, `passkey`)
	// extend this list — the AuthProviders CSV is the wire surface.
	AuthProviderGitHub = "github"
)

type Config struct {
	// DatabaseURL is a Postgres connection string.
	DatabaseURL string

	// LogLevel is "debug" | "info" | "warn" | "error".
	LogLevel string

	// AuthProviders is the CSV-decoded list of identity providers the
	// daemon exposes to external requests. Empty (default) means no
	// IdP is wired — combined with a loopback BindAddr that yields the
	// "loopback only, single user" model. The first non-empty value
	// drives MCP `/.well-known/oauth-authorization-server` metadata
	// and the OAuth bootstrap path; loopback requests bypass it.
	AuthProviders []string

	// BindAddr is the host:port the daemon listens on. Loopback
	// (127.0.0.1 / ::1 / localhost) is the "self-host" baseline where
	// every request is auto-trusted; non-loopback values trip the
	// Public-Without-Auth Refusal unless an IdP or the explicit
	// AllowPublicUnauthenticated opt-in is also set.
	BindAddr string

	// AllowPublicUnauthenticated is the explicit opt-in for "external
	// exposure with no IdP". Default false — an operator who wants
	// their daemon reachable on a private LAN behind a trusted reverse
	// proxy must set this to acknowledge the trust assumption.
	AllowPublicUnauthenticated bool

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
	// falls back to "(unknown) via {agent_id}" byline.
	UserName  string
	UserEmail string

	// OAuth 2.1 authorization-server settings used when AuthProviders
	// includes an IdP that needs upstream OAuth (today: github). Pindoc
	// acts as the MCP-facing OAuth authorization server while GitHub is
	// the upstream identity provider used during signup/login.
	OAuthSigningKeyPath  string
	OAuthClientID        string
	OAuthClientSecret    string
	OAuthRedirectURIs    []string
	OAuthRedirectBaseURL string
	GitHubClientID       string
	GitHubClientSecret   string
}

type SummaryConfig struct {
	Endpoint      string
	APIKey        string
	Model         string
	DailyTokenCap int
	GroupCap      int
	Timeout       time.Duration
}

// HasAuthProvider reports whether the given provider name is active.
// Comparison is case-insensitive — operators who set
// `PINDOC_AUTH_PROVIDERS=GitHub` get the same wiring as `github`.
func (c *Config) HasAuthProvider(name string) bool {
	if c == nil {
		return false
	}
	target := strings.ToLower(strings.TrimSpace(name))
	for _, p := range c.AuthProviders {
		if strings.ToLower(strings.TrimSpace(p)) == target {
			return true
		}
	}
	return false
}

// IsLoopbackBind reports whether BindAddr resolves to a loopback host
// (127.0.0.1, ::1, or `localhost`). Empty BindAddr is treated as
// loopback because the daemon defaults to `127.0.0.1:5830`. Used by
// the boot-time Public-Without-Auth Refusal and by the streamable_http
// transport to decide whether the OAuth middleware is mandatory.
func (c *Config) IsLoopbackBind() bool {
	if c == nil {
		return true
	}
	return isLoopbackHostPort(c.BindAddr)
}

// Validate runs the cross-field invariants Load() would otherwise have
// to repeat. Today the only invariant is the Public-Without-Auth
// Refusal — exported so tests and `cmd/pindoc-server` can call it on
// configs they assemble manually.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config: nil")
	}
	if !c.IsLoopbackBind() && len(c.AuthProviders) == 0 && !c.AllowPublicUnauthenticated {
		return ErrPublicWithoutAuth
	}
	return nil
}

// Load builds a Config from process env vars and fails fast on
// configurations that would expose the daemon to the network without
// authentication so a misconfigured daemon never silently boots in
// the wrong security model.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:                env("PINDOC_DATABASE_URL", "postgres://pindoc:pindoc_dev@localhost:5432/pindoc?sslmode=disable"),
		LogLevel:                   env("PINDOC_LOG_LEVEL", "info"),
		AuthProviders:              normalizeProviders(envList("PINDOC_AUTH_PROVIDERS", nil)),
		BindAddr:                   strings.TrimSpace(env("PINDOC_BIND_ADDR", DefaultBindAddr)),
		AllowPublicUnauthenticated: envBool("PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED", false),
		UserLanguage:               strings.ToLower(env("PINDOC_USER_LANGUAGE", "en")),
		ReceiptExemptionLimit:      envInt("PINDOC_RECEIPT_EXEMPTION_LIMIT", 5),
		ProjectSlug:                env("PINDOC_PROJECT", "pindoc"),
		RepoRoot:                   env("PINDOC_REPO_ROOT", ""),
		UserName:                   strings.TrimSpace(env("PINDOC_USER_NAME", "")),
		UserEmail:                  strings.TrimSpace(env("PINDOC_USER_EMAIL", "")),
		OAuthSigningKeyPath:        env("PINDOC_OAUTH_SIGNING_KEY_PATH", "./data/oauth-signing.pem"),
		OAuthClientID:              strings.TrimSpace(env("PINDOC_OAUTH_CLIENT_ID", "claude-desktop")),
		OAuthClientSecret:          strings.TrimSpace(env("PINDOC_OAUTH_CLIENT_SECRET", "")),
		OAuthRedirectBaseURL:       strings.TrimSpace(env("PINDOC_OAUTH_REDIRECT_BASE_URL", "")),
		GitHubClientID:             strings.TrimSpace(env("PINDOC_GITHUB_CLIENT_ID", "")),
		GitHubClientSecret:         strings.TrimSpace(env("PINDOC_GITHUB_CLIENT_SECRET", "")),
		OAuthRedirectURIs: envList("PINDOC_OAUTH_REDIRECT_URIS", []string{
			"http://127.0.0.1:3846/callback",
			"http://localhost:3846/callback",
		}),
		Embed: embed.Config{
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
	if cfg.BindAddr == "" {
		cfg.BindAddr = DefaultBindAddr
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// normalizeProviders trims and lowercases each entry, dropping
// duplicates and empty fragments so `PINDOC_AUTH_PROVIDERS=GitHub , `
// boots the same chain as `github`.
func normalizeProviders(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		v := strings.ToLower(strings.TrimSpace(item))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// isLoopbackHostPort reports whether `addr` (host:port or just host)
// names a loopback address. Treats empty / unparseable as loopback so
// the default boot path stays loopback-only.
func isLoopbackHostPort(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return true
	}
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
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

func envList(key string, fallback []string) []string {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return append([]string(nil), fallback...)
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// FormatProvidersForLog renders an authentication providers list for
// log lines. Empty list returns "(none)" so log scrapers can grep it
// without parsing CSV.
func FormatProvidersForLog(providers []string) string {
	if len(providers) == 0 {
		return "(none)"
	}
	return strings.Join(providers, ",")
}
