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
	"github.com/var-gg/pindoc/internal/pindoc/settings"
	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
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

	// Apply migrations — keeps pindoc-api startable even if pindoc-server
	// hasn't run yet (operator-first self-host deploys often hit the HTTP
	// path before any agent session). Migrations are idempotent per
	// schema_migrations row.
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		logger.Error("db migrate", "err", err)
		os.Exit(1)
	}

	ssStore, err := settings.New(ctx, pool)
	if err != nil {
		logger.Error("settings load", "err", err)
		os.Exit(1)
	}
	if seeded, err := ssStore.SeedFromEnv(ctx, "public_base_url", os.Getenv("PINDOC_PUBLIC_BASE_URL")); err != nil {
		logger.Warn("settings seed from env failed", "err", err)
	} else if seeded {
		logger.Info("settings seeded from env", "key", "public_base_url")
	}

	embedder, err := embed.Build(cfg.Embed)
	if err != nil {
		logger.Error("embed build", "err", err)
		os.Exit(1)
	}
	telemetryStore := telemetry.New(ctx, pool.Pool, logger, telemetry.Options{})
	defer telemetryStore.Close()

	// Resolve the default project's locale at startup so LegacyRedirect
	// in the web UI can rebuild pre-Phase-18 URLs into /p/:slug/:locale/
	// shape without an extra round-trip (Task task-phase-18-project-
	// locale-implementation). Missing project row (fresh install before
	// pindoc.project.create runs) is expected — leave empty and the
	// client-side fallback kicks in.
	var defaultLocale string
	if err := pool.QueryRow(ctx,
		`SELECT locale FROM projects WHERE slug = $1 LIMIT 1`, cfg.ProjectSlug,
	).Scan(&defaultLocale); err != nil {
		logger.Info("default project locale lookup skipped",
			"project_slug", cfg.ProjectSlug, "err", err)
	}

	handler := httpapi.New(cfg, httpapi.Deps{
		DB:                   pool,
		Logger:               logger,
		DefaultProjectSlug:   cfg.ProjectSlug,
		DefaultProjectLocale: defaultLocale,
		MultiProject:         cfg.MultiProject,
		Embedder:             embedder,
		Settings:             ssStore,
		Telemetry:            telemetryStore,
		Version:              version,
		BuildCommit:          commit,
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
