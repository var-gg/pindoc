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

	server := pmcp.NewServer(pmcp.Options{
		Name:    "pindoc",
		Version: version,
		Logger:  logger,
		Config:  cfg,
	})

	err = server.Run(ctx, &sdk.StdioTransport{})
	switch {
	case err == nil,
		errors.Is(err, context.Canceled),
		errors.Is(err, io.EOF),
		// The SDK wraps its close signal as a non-typed error on Windows;
		// fall back to a substring check so clean disconnects don't log
		// at ERROR and trip on-disconnect alerting when we wire that up.
		err != nil && strings.Contains(err.Error(), "server is closing"):
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
