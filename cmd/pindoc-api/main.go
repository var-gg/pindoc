// pindoc-api runs the read-only HTTP API the web UI talks to.
//
// Separate binary from pindoc-server (stdio MCP) on purpose: Claude Code
// launches pindoc-server as a subprocess per session, so binding a shared
// HTTP port from there would conflict across sessions. The HTTP API is a
// long-running daemon a user starts once (`go run ./cmd/pindoc-api`) and
// leaves open while they dev.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/httpapi"
)

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
		logger.Error("config load", "err", err)
		os.Exit(1)
	}

	addr := os.Getenv("PINDOC_HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:5831"
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db open", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	embedder, err := embed.Build(cfg.Embed)
	if err != nil {
		logger.Error("embed build", "err", err)
		os.Exit(1)
	}

	handler := httpapi.New(cfg, httpapi.Deps{
		DB:          pool,
		Logger:      logger,
		ProjectSlug: cfg.ProjectSlug,
		Embedder:    embedder,
		Version:     version,
		BuildCommit: commit,
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("pindoc-api listening", "addr", addr, "project", cfg.ProjectSlug)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown requested")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("pindoc-api stopped")
}
