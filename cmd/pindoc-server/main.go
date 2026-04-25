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
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/httpapi"
	pmcp "github.com/var-gg/pindoc/internal/pindoc/mcp"
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

	logger.Info("pindoc-server starting",
		"version", version,
		"commit", commit,
		"transport", transportName,
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
		logger.Warn("using stub embedder — retrieval quality is hash-based, not semantic. Set PINDOC_EMBED_PROVIDER=http + PINDOC_EMBED_ENDPOINT=... to enable real embeddings.")
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

		apiHandler := httpapi.New(cfg, httpapi.Deps{
			DB:                   pool,
			Logger:               logger,
			DefaultProjectSlug:   cfg.ProjectSlug,
			DefaultProjectLocale: defaultLocale,
			Embedder:             embedder,
			Settings:             ssStore,
			Telemetry:            tele,
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
			Settings:  ssStore,
			Telemetry: tele,
			Transport: "streamable_http",
		}, apiHandler)
		return
	}

	server := pmcp.NewServer(pmcp.Options{
		Name:      "pindoc",
		Version:   version,
		Logger:    logger,
		Config:    cfg,
		DB:        pool,
		Embedder:  embedder,
		AgentID:   agentID,
		Settings:  ssStore,
		Telemetry: tele,
		Transport: "stdio",
	})

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
func runHTTPDaemon(ctx context.Context, logger *slog.Logger, addr string, baseOpts pmcp.Options, apiHandler http.Handler) {
	mcpServer := pmcp.NewServer(baseOpts).SDK()
	getServer := func(_ *http.Request) *sdk.Server {
		return mcpServer
	}
	streamHandler := sdk.NewStreamableHTTPHandler(getServer, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", streamHandler)
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
