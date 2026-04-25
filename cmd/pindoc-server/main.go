// Pindoc MCP server entry point.
//
// Default transport is stdio — the channel every MCP-capable coding agent
// (Claude Code, Cursor, Codex, Cline) already speaks; the binary is
// launched as a subprocess by the agent and the project is pinned at
// startup via PINDOC_PROJECT.
//
// `-http <addr>` (or PINDOC_HTTP_MCP_ADDR env) flips the binary into
// long-running daemon mode. A single daemon serves multiple Claude Code
// sessions over streamable-HTTP, with each session pinned to its project
// via the URL path /mcp/p/{project}. The getServer callback resolves
// that slug per-connection so writes always land in the URL's project
// — see docs/03-architecture.md and Decision
// pindoc-mcp-transport-streamable-http-per-connection-scope for the
// rationale.
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
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
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
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// `-http <addr>` flips the binary into streamable-HTTP daemon mode. A
	// single daemon serves many Claude Code sessions, each pinned to a
	// project via /mcp/p/{project}. Empty (default) keeps the legacy
	// subprocess-per-session stdio path. PINDOC_HTTP_MCP_ADDR env is the
	// equivalent for setups (Docker, systemd) that prefer envs over args.
	httpAddrFlag := flag.String("http", "", "When set, run as a streamable-HTTP MCP daemon binding this address (e.g. 127.0.0.1:5832). Empty = stdio mode.")
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
		})
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

// runHTTPDaemon serves the streamable-HTTP MCP transport on addr. Multiple
// Claude Code sessions can attach to a single daemon, each pinned to one
// project via the /mcp/p/{project} URL path. The getServer callback is
// invoked once per connection: it parses the slug, validates against the
// projects table, and builds a fresh project-scoped Server for the rest
// of that session. baseOpts carries the long-lived dependencies (DB pool,
// embedder, settings, telemetry, agent identity) shared across every
// connection — only ProjectSlug and Transport vary per request.
func runHTTPDaemon(ctx context.Context, logger *slog.Logger, addr string, baseOpts pmcp.Options) {
	pool := baseOpts.DB

	getServer := func(req *http.Request) *sdk.Server {
		project := strings.TrimSpace(req.PathValue("project"))
		if project == "" {
			// Should never happen because the mux pattern requires the
			// segment, but guard anyway so a malformed mount doesn't
			// crash the daemon.
			logger.Warn("getServer called with empty project slug")
			return nil
		}
		opts := baseOpts
		opts.ProjectSlug = project
		opts.Transport = "streamable_http"
		return pmcp.NewServer(opts).SDK()
	}

	streamHandler := sdk.NewStreamableHTTPHandler(getServer, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp/p/{project}", func(w http.ResponseWriter, r *http.Request) {
		project := strings.TrimSpace(r.PathValue("project"))
		if project == "" {
			http.Error(w, "project slug required", http.StatusBadRequest)
			return
		}
		// Validate project exists before delegating to the SDK so the
		// failure mode is a plain HTTP 404 the operator can grep for in
		// access logs, not an opaque transport error inside the MCP
		// session.
		var exists bool
		if err := pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM projects WHERE slug = $1)`,
			project,
		).Scan(&exists); err != nil {
			logger.Error("project existence check failed",
				"project_slug", project, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !exists {
			logger.Warn("rejected MCP connection — unknown project slug",
				"project_slug", project, "remote", r.RemoteAddr)
			http.Error(w, fmt.Sprintf("project %q not found", project), http.StatusNotFound)
			return
		}
		streamHandler.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("pindoc-server listening (streamable_http)",
			"addr", addr,
			"path_pattern", "/mcp/p/{project}",
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http listen failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("pindoc-server shutting down (streamable_http)")
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
