// Pindoc MCP server entry point.
//
// Default transport is stdio — the channel every MCP-capable coding agent
// (Claude Code, Cursor, Codex, Cline) already speaks; the binary is
// launched as a subprocess by the agent and the operator's default
// project is read from PINDOC_PROJECT (per-call inputs may override).
//
// `-http <addr>` (or PINDOC_HTTP_MCP_ADDR env) flips the binary into
// long-running daemon mode. A single daemon serves multiple agent
// sessions over streamable-HTTP at one account-level URL: /mcp.
// Connections are not scoped to a project; each tool input carries
// project_slug and the handler resolves it per call. See
// docs/03-architecture.md.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/httpapi"
	pmcp "github.com/var-gg/pindoc/internal/pindoc/mcp"
	pmcptools "github.com/var-gg/pindoc/internal/pindoc/mcp/tools"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
	"github.com/var-gg/pindoc/internal/pindoc/providers"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
)

// Build-time variables. Set via -ldflags in release builds.
var (
	version = "0.0.1-dev"
	commit  = "unknown"
)

func main() {
	startTime := time.Now()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// `-http <addr>` flips the binary into HTTP daemon mode. The same
	// address serves the streamable-HTTP MCP transport (/mcp — single
	// account-level endpoint), the Reader's read-only API (/api/...),
	// and a liveness probe (/health) on a single mux — all loopback-
	// only, all gated by auth_mode=trusted_local for V1. Empty
	// (default) keeps the legacy subprocess-per-session stdio path.
	// PINDOC_HTTP_MCP_ADDR env is the equivalent for setups (Docker,
	// systemd) that prefer envs over args.
	httpAddrFlag := flag.String("http", "", "When set, run as an HTTP daemon binding this address (e.g. 127.0.0.1:5830) — serves /mcp, /api/..., /health. Empty = stdio mode.")
	flag.Parse()
	httpAddr := strings.TrimSpace(*httpAddrFlag)
	if httpAddr == "" {
		httpAddr = strings.TrimSpace(os.Getenv("PINDOC_HTTP_MCP_ADDR"))
	}
	transportName := "stdio"
	if httpAddr != "" {
		transportName = "streamable_http"
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}
	if err := validateServerConfig(cfg); err != nil {
		logger.Error("server config rejected", "err", err)
		os.Exit(1)
	}

	logger.Info("pindoc-server starting",
		"version", version,
		"commit", commit,
		"transport", transportName,
		"providers", config.FormatProvidersForLog(cfg.AuthProviders),
		"bind_addr", cfg.BindAddr,
		"loopback_only", cfg.IsLoopbackBind(),
		"db_configured", cfg.DatabaseURL != "",
	)

	// Signal-driven shutdown. Claude Code closes stdin on disconnect, so the
	// stdio transport will also return on its own; the signal handler is for
	// manual ctrl-C during local dev.
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db open failed", "err", err, "dsn_hint", "is docker compose up -d db running?")
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool.Pool); err != nil {
		logger.Error("db migrate failed", "err", err)
		os.Exit(1)
	}
	logger.Info("db ready", "migrations", "applied")
	if normalized, err := projects.BootstrapDefaultProjectRepoFromWorkdir(ctx, pool, cfg.ProjectSlug, ""); err != nil {
		logger.Info("default project repo bootstrap skipped", "project_slug", cfg.ProjectSlug, "err", err)
	} else if normalized != "" {
		logger.Info("default project repo bootstrap checked", "project_slug", cfg.ProjectSlug, "git_remote_url", normalized)
	}

	embedder, err := embed.Build(cfg.Embed)
	if err != nil {
		logger.Error("embed provider build failed", "err", err)
		os.Exit(1)
	}
	info := embedder.Info()
	logger.Info("embedder ready",
		"name", info.Name, "model", info.ModelID,
		"dim", info.Dimension, "max_tokens", info.MaxTokens,
		"multilingual", info.Multilingual,
	)
	if info.Name == "stub" {
		logger.Warn(stubEmbedderWarning())
	}

	// Phase 14a: operator-editable settings, loaded from DB with one-time
	// env seed for first-boot convenience. docker-compose can pass
	// PINDOC_PUBLIC_BASE_URL; after first successful write the DB value
	// is authoritative and env changes are ignored (Ghost / Plausible
	// pattern — avoids the "UI change silently overridden by env" trap).
	ssStore, err := settings.New(ctx, pool)
	if err != nil {
		logger.Error("settings load failed", "err", err)
		os.Exit(1)
	}
	if seeded, err := ssStore.SeedFromEnv(ctx, "public_base_url", os.Getenv("PINDOC_PUBLIC_BASE_URL")); err != nil {
		logger.Warn("settings seed from env failed", "err", err)
	} else if seeded {
		logger.Info("settings seeded from env", "key", "public_base_url")
	}
	logger.Info("settings ready",
		"public_base_url", ssStore.Get().PublicBaseURL,
	)

	// Phase 12c: server-issued agent identity for this stdio subprocess.
	// Takes the env value when set (so a wrapper script can pin an agent
	// to a stable id across restarts), otherwise mints a fresh random one.
	// Persisted on every artifact_revisions row via source_session_ref so
	// audit trails don't depend on the agent's self-reported author_id.
	agentID := os.Getenv("PINDOC_AGENT_ID")
	if strings.TrimSpace(agentID) == "" {
		buf := make([]byte, 12)
		_, _ = rand.Read(buf)
		agentID = "ag_" + hex.EncodeToString(buf)
	}
	logger.Info("agent identity", "agent_id", agentID, "source", agentIDSource())

	// Phase J — async MCP tool-call telemetry. ctx governs the flusher
	// goroutine; on shutdown we close it so any buffered entries flush
	// before the process exits.
	tele := telemetry.New(ctx, pool.Pool, logger, telemetry.Options{})
	defer tele.Close()

	// Build the providers cipher + store once so the HTTP daemon, the
	// admin API, and the OAuthService share one decryption path. Empty
	// PINDOC_INSTANCE_KEY is OK on a fresh install — the EnsureKey
	// Available call below refuses to start only when the DB already
	// holds an encrypted credential and the key is missing.
	providerCipher, err := providers.NewCipherFromBase64(cfg.InstanceKeyB64)
	if err != nil {
		logger.Error("instance key invalid", "err", err)
		os.Exit(1)
	}
	providerStore := providers.New(pool, providerCipher)
	if err := providerStore.EnsureKeyAvailable(ctx); err != nil {
		logger.Error("provider store gate", "err", err,
			"hint", "PINDOC_INSTANCE_KEY is required because instance_providers contains encrypted credentials")
		os.Exit(1)
	}
	logger.Info("provider store ready",
		"instance_key_configured", providerCipher.Configured(),
	)

	// Resolve the bootstrap users.id once so the HTTP daemon and MCP
	// layers stamp the same row on Loopback principals (Decision
	// `decision-author-identity-dual`). Empty UserID is the "operator
	// skipped PINDOC_USER_NAME" path — handlers fall back to
	// anonymous attribution.
	defaultUserID := ""
	if uid, err := pmcptools.UpsertUserFromEnv(ctx, pmcptools.Deps{DB: pool}, cfg.UserName, cfg.UserEmail); err != nil {
		logger.Warn("user upsert from env failed; loopback principals run without user binding",
			"error", err,
			"user_name", cfg.UserName,
		)
	} else {
		defaultUserID = uid
		if uid != "" {
			logger.Info("user binding resolved",
				"user_id", uid,
				"display_name", cfg.UserName,
			)
		}
	}

	if httpAddr != "" {
		// Resolve the default project's canonical language once for the
		// compatibility `default_project_locale` API field. Reader URLs no
		// longer carry locale after task-canonical-locale-migration.
		var defaultLocale string
		if err := pool.QueryRow(ctx,
			`SELECT primary_language FROM projects WHERE slug = $1 LIMIT 1`, cfg.ProjectSlug,
		).Scan(&defaultLocale); err != nil {
			logger.Info("default project language lookup skipped",
				"project_slug", cfg.ProjectSlug, "err", err)
		}

		// Reader SPA dist dir. PINDOC_SPA_DIST overrides; otherwise we
		// look for `<cwd>/web/dist`, which is where `pnpm --dir web build`
		// drops it and what the NSSM service's AppDirectory points at.
		// Empty when neither resolves — daemon stays API-only and the
		// operator is expected to front it with a Vite dev server.
		spaDist := strings.TrimSpace(os.Getenv("PINDOC_SPA_DIST"))
		if spaDist == "" {
			if cwd, err := os.Getwd(); err == nil {
				candidate := filepath.Join(cwd, "web", "dist")
				if info, err := os.Stat(candidate); err == nil && info.IsDir() {
					spaDist = candidate
				}
			}
		}
		if spaDist != "" {
			logger.Info("Reader SPA enabled", "dist_dir", spaDist)
		} else {
			logger.Info("Reader SPA disabled — set PINDOC_SPA_DIST or `pnpm --dir web build` to enable")
		}

		// GitHub IdP activation: env CSV is the boot-time seed, DB is
		// the runtime source of truth. The admin UI mutates the DB
		// row, OAuthService.SetGitHubCredentials swaps creds without
		// a restart. Either source enabling github is enough to wire
		// the AS now.
		dbProviderRows, err := providerStore.Active(ctx)
		if err != nil {
			logger.Error("provider store active query failed", "err", err)
			os.Exit(1)
		}
		dbGithub, dbHasGithub := findGithubProvider(dbProviderRows)
		githubActive := cfg.HasAuthProvider(config.AuthProviderGitHub) || dbHasGithub

		var oauthSvc *pauth.OAuthService
		if githubActive {
			oauthUserID, err := pauth.EnsureBootstrapUser(ctx, pool, cfg.UserName, cfg.UserEmail)
			if err != nil {
				logger.Error("oauth bootstrap user failed", "err", err)
				os.Exit(1)
			}
			if err := projects.EnsureDefaultProjectOwnerMembership(ctx, pool, cfg.ProjectSlug, oauthUserID); err != nil {
				logger.Error("oauth default project membership bootstrap failed", "err", err)
				os.Exit(1)
			}
			publicBaseURL := daemonPublicBaseURL(ssStore.Get().PublicBaseURL, httpAddr)
			redirectBaseURL := daemonPublicBaseURL(firstNonEmpty(cfg.OAuthRedirectBaseURL, ssStore.Get().PublicBaseURL), httpAddr)
			ghClientID := cfg.GitHubClientID
			ghClientSecret := cfg.GitHubClientSecret
			if dbHasGithub {
				ghClientID = dbGithub.ClientID
				ghClientSecret = dbGithub.ClientSecret
			}
			oauthSvc, err = pauth.NewOAuthService(ctx, pool, pauth.OAuthConfig{
				Issuer:             publicBaseURL,
				PublicBaseURL:      publicBaseURL,
				RedirectBaseURL:    redirectBaseURL,
				SigningKeyPath:     cfg.OAuthSigningKeyPath,
				ClientID:           cfg.OAuthClientID,
				ClientSecret:       cfg.OAuthClientSecret,
				RedirectURIs:       cfg.OAuthRedirectURIs,
				BootstrapUserID:    oauthUserID,
				GitHubClientID:     ghClientID,
				GitHubClientSecret: ghClientSecret,
			})
			if err != nil {
				logger.Error("oauth service init failed", "err", err)
				os.Exit(1)
			}
			logger.Info("oauth service ready",
				"issuer", publicBaseURL,
				"client_id", cfg.OAuthClientID,
				"bootstrap_user_id", oauthUserID,
				"github_credentials_source", credentialsSource(dbHasGithub),
				"github_wired", oauthSvc.HasGitHub(),
			)
		}

		apiHandler := httpapi.New(cfg, httpapi.Deps{
			DB:                   pool,
			Logger:               logger,
			DefaultProjectSlug:   cfg.ProjectSlug,
			DefaultProjectLocale: defaultLocale,
			Embedder:             embedder,
			Settings:             ssStore,
			Telemetry:            tele,
			OAuth:                oauthSvc,
			Providers:            providerStore,
			AuthProviders:        cfg.AuthProviders,
			BindAddr:             cfg.BindAddr,
			DefaultUserID:        defaultUserID,
			DefaultAgentID:       agentID,
			Version:              version,
			BuildCommit:          commit,
			StartTime:            startTime,
			SPADistDir:           spaDist,
		})

		runHTTPDaemon(ctx, logger, httpAddr, pmcp.Options{
			Name:      "pindoc",
			Version:   version,
			Logger:    logger,
			Config:    cfg,
			DB:        pool,
			Embedder:  embedder,
			AgentID:   agentID,
			UserID:    defaultUserID,
			Settings:  ssStore,
			Telemetry: tele,
			Transport: "streamable_http",
		}, apiHandler, oauthSvc, cfg)
		return
	}

	server, err := pmcp.NewServer(pmcp.Options{
		Name:      "pindoc",
		Version:   version,
		Logger:    logger,
		Config:    cfg,
		DB:        pool,
		Embedder:  embedder,
		AgentID:   agentID,
		UserID:    defaultUserID,
		Settings:  ssStore,
		Telemetry: tele,
		Transport: "stdio",
	})
	if err != nil {
		logger.Error("mcp server init failed", "err", err)
		os.Exit(1)
	}

	err = server.Run(ctx, &sdk.StdioTransport{})
	switch {
	case err == nil,
		errors.Is(err, context.Canceled),
		errors.Is(err, io.EOF),
		// The SDK wraps its close signal as a non-typed error on Windows;
		// fall back to a substring check so clean disconnects don't log
		// at ERROR and trip on-disconnect alerting when we wire that up.
		err != nil && strings.Contains(err.Error(), "server is closing"),
		err != nil && strings.Contains(err.Error(), "file already closed"):
		logger.Info("pindoc-server stopped cleanly", "reason", errReason(err))
		return
	default:
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

// runHTTPDaemon serves the streamable-HTTP MCP transport, the Reader
// read-only HTTP API, and the /health liveness probe on a single addr.
// Account-level scope (Decision mcp-scope-account-level-industry-
// standard) means every MCP session attaches to the same /mcp endpoint
// and each tool call carries a project_slug input that the handler
// resolves per call. The MCP server is built once at boot and the
// getServer callback returns the same *sdk.Server for every request —
// no per-connection rebuild. baseOpts carries the long-lived MCP
// dependencies; apiHandler carries the Reader API mux (httpapi.New) —
// Go 1.22's ServeMux picks /mcp over the catch-all `/` so the routing
// is unambiguous.
func runHTTPDaemon(ctx context.Context, logger *slog.Logger, addr string, baseOpts pmcp.Options, apiHandler http.Handler, oauthSvc *pauth.OAuthService, cfg *config.Config) {
	mcp, err := pmcp.NewServer(baseOpts)
	if err != nil {
		logger.Error("mcp server init failed", "err", err)
		os.Exit(1)
	}
	mcpServer := mcp.SDK()
	getServer := func(_ *http.Request) *sdk.Server {
		return mcpServer
	}
	streamHandler := sdk.NewStreamableHTTPHandler(getServer, nil)
	var mcpHandler http.Handler = streamHandler
	if cfg != nil && cfg.HasAuthProvider(config.AuthProviderGitHub) {
		if oauthSvc == nil {
			logger.Error("oauth provider configured but oauth service is nil")
			os.Exit(1)
		}
		bearer := mcpauth.RequireBearerToken(oauthSvc.TokenVerifier, &mcpauth.RequireBearerTokenOptions{
			ResourceMetadataURL: oauthSvc.ResourceMetadataURL(),
			Scopes:              []string{pauth.ScopePindoc},
		})(mcpHandler)
		// Loopback Trust (Decision § 2): same-host calls bypass the
		// bearer middleware so stdio-loopback parity holds for the
		// HTTP transport too. Non-loopback callers still must
		// present a Pindoc AS JWT.
		mcpHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pauth.IsLoopbackRequest(r) {
				streamHandler.ServeHTTP(w, r)
				return
			}
			bearer.ServeHTTP(w, r)
		})
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	// Catch-all delegates everything else (/, /health, /api/...) to the
	// Reader API mux. ServeMux picks /mcp over `/` because it is the
	// more specific pattern.
	mux.Handle("/", apiHandler)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("pindoc-server listening",
			"addr", addr,
			"mcp_path", "/mcp",
			"api_path", "/api/...",
			"health_path", "/health",
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http listen failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("pindoc-server shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http shutdown returned error", "err", err)
	}
	logger.Info("pindoc-server stopped cleanly", "reason", "context done")
}

func errReason(err error) string {
	if err == nil {
		return "context done"
	}
	return err.Error()
}

func agentIDSource() string {
	if strings.TrimSpace(os.Getenv("PINDOC_AGENT_ID")) != "" {
		return "env"
	}
	return "generated"
}

func validateServerConfig(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	// PINDOC_GITHUB_CLIENT_ID / SECRET are no longer hard-required
	// here — the admin UI can supply them via instance_providers
	// (Decision decision-auth-model-loopback-and-providers § 3,
	// task-providers-admin-ui). Boot still fails loud later if env
	// CSV says github but neither env nor DB carries credentials.
	return nil
}

// findGithubProvider scans an Active() result for the github IdP.
// Returns the row + true when present; ok=false signals "no github
// row in DB" so boot falls back to env credentials.
func findGithubProvider(active []providers.Record) (providers.Record, bool) {
	for _, r := range active {
		if r.ProviderName == providers.ProviderGitHub {
			return r, true
		}
	}
	return providers.Record{}, false
}

// credentialsSource is a tiny helper to keep the boot log line readable.
func credentialsSource(fromDB bool) string {
	if fromDB {
		return "instance_providers"
	}
	return "env"
}

func daemonPublicBaseURL(publicBaseURL, addr string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if publicBaseURL != "" {
		if strings.HasPrefix(publicBaseURL, "http://") || strings.HasPrefix(publicBaseURL, "https://") {
			return publicBaseURL
		}
		return "http://" + publicBaseURL
	}
	host := strings.TrimSpace(addr)
	if h, p, err := net.SplitHostPort(addr); err == nil {
		h = strings.Trim(h, "[]")
		switch h {
		case "", "0.0.0.0", "::":
			h = "127.0.0.1"
		}
		host = net.JoinHostPort(h, p)
	}
	return "http://" + host
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stubEmbedderWarning() string {
	return strings.TrimSpace(`
+------------------------------------------------------------+
| EMBEDDER WARNING                                           |
| PINDOC_EMBED_PROVIDER=stub is active.                      |
| Search quality is hash-based, not semantic.                |
| For normal Docker boot, unset PINDOC_COMPOSE_EMBED_PROVIDER |
| so the default Gemma embedder starts.                      |
| Re-embed affected artifacts after returning to real search. |
+------------------------------------------------------------+`)
}
