// Pindoc MCP server entry point.
//
// Runs over stdio — the transport every MCP-capable coding agent (Claude
// Code, Cursor, Codex, Cline) already speaks. An HTTP read API and a
// streamable-HTTP MCP transport land in later phases; for M1 Phase 1 we
// only need stdio + a handshake tool so Claude Code can attach.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	pmcp "github.com/var-gg/pindoc/internal/pindoc/mcp"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
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

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}

	logger.Info("pindoc-server starting",
		"version", version,
		"commit", commit,
		"transport", "stdio",
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

	server := pmcp.NewServer(pmcp.Options{
		Name:     "pindoc",
		Version:  version,
		Logger:   logger,
		Config:   cfg,
		DB:       pool,
		Embedder: embedder,
		AgentID:  agentID,
		Settings: ssStore,
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
