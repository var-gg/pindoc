// Pindoc MCP server entry point.
//
// Runs over stdio — the transport every MCP-capable coding agent (Claude
// Code, Cursor, Codex, Cline) already speaks. An HTTP read API and a
// streamable-HTTP MCP transport land in later phases; for M1 Phase 1 we
// only need stdio + a handshake tool so Claude Code can attach.
package main

import (
	"context"
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

	server := pmcp.NewServer(pmcp.Options{
		Name:     "pindoc",
		Version:  version,
		Logger:   logger,
		Config:   cfg,
		DB:       pool,
		Embedder: embedder,
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
