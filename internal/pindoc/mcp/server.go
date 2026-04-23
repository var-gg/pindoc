// Package mcp wires the official Go MCP SDK to Pindoc's tool surface.
//
// Tools live in sub-packages under ./tools. This package owns the server
// lifecycle: it constructs the sdk.Server, registers every tool, and
// exposes a single Run entry point the main() binary calls.
package mcp

import (
	"context"
	"log/slog"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/mcp/tools"
	"github.com/var-gg/pindoc/internal/pindoc/receipts"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
)

// resolveStartupProjectLocale reads projects.locale for the active
// PINDOC_PROJECT slug and returns it so HumanURL / AbsHumanURL can embed
// the locale segment in response URLs (Task task-phase-18-project-locale-
// implementation). Falls back to empty string when the project row is
// missing or DB unreachable; HumanURL then falls back to its own "en"
// default so share links still render without blocking the server boot.
func resolveStartupProjectLocale(ctx context.Context, logger *slog.Logger, deps tools.Deps) string {
	if deps.DB == nil {
		return ""
	}
	var locale string
	err := deps.DB.QueryRow(ctx,
		`SELECT locale FROM projects WHERE slug = $1 LIMIT 1`,
		deps.ProjectSlug,
	).Scan(&locale)
	if err != nil {
		logger.Warn("project locale lookup failed; HumanURL uses 'en' fallback",
			"project_slug", deps.ProjectSlug, "err", err)
		return ""
	}
	return locale
}

// upsertStartupUserID resolves the users.id row this MCP session should
// bind to (Decision `decision-author-identity-dual`, migration 0014). Any
// failure to reach the DB or upsert is logged and treated as "no user
// bound" — the server still boots and artifact.propose then leaves
// author_user_id NULL. Returning empty on empty env is intentional.
func upsertStartupUserID(ctx context.Context, logger *slog.Logger, deps tools.Deps, cfg *config.Config) string {
	id, err := tools.UpsertUserFromEnv(ctx, deps, cfg.UserName, cfg.UserEmail)
	if err != nil {
		logger.Warn("user upsert from env failed; MCP session runs without user binding",
			"error", err,
			"user_name", cfg.UserName,
		)
		return ""
	}
	if id != "" {
		logger.Info("user binding resolved for MCP session",
			"user_id", id,
			"display_name", cfg.UserName,
		)
	}
	return id
}

type Options struct {
	Name     string
	Version  string
	Logger   *slog.Logger
	Config   *config.Config
	DB       *db.Pool
	Embedder embed.Provider

	// AgentID is the server-issued identity for this subprocess (Phase
	// 12c). Set by the binary entrypoint at startup; empty falls back to
	// "unassigned" which still lets writes proceed but flags the gap in
	// audit logs.
	AgentID string

	// Settings is the operator-editable config store (Phase 14a).
	Settings *settings.Store
}

type Server struct {
	sdk    *sdk.Server
	logger *slog.Logger
}

func NewServer(opts Options) *Server {
	impl := &sdk.Implementation{
		Name:    opts.Name,
		Version: opts.Version,
	}
	s := sdk.NewServer(impl, nil)

	// Phase 1: handshake.
	tools.RegisterPing(s, tools.PingDeps{
		Version:      opts.Version,
		UserLanguage: opts.Config.UserLanguage,
	})

	// Phase 2 read-side: project context + scope enumeration + artifact fetch.
	deps := tools.Deps{
		DB:           opts.DB,
		Logger:       opts.Logger,
		Version:      opts.Version,
		ProjectSlug:  opts.Config.ProjectSlug,
		UserLanguage: opts.Config.UserLanguage,
		Embedder:     opts.Embedder,
		MultiProject: opts.Config.MultiProject,
		Receipts:     receipts.New(0), // DefaultTTL applies
		AgentID:      opts.AgentID,
		Settings:     opts.Settings,
		RepoRoot:     opts.Config.RepoRoot,
	}
	deps.UserID = upsertStartupUserID(context.Background(), opts.Logger, deps, opts.Config)
	deps.ProjectLocale = resolveStartupProjectLocale(context.Background(), opts.Logger, deps)

	tools.RegisterProjectCurrent(s, deps)
	tools.RegisterProjectCreate(s, deps)
	tools.RegisterAreaList(s, deps)
	tools.RegisterArtifactRead(s, deps)

	// Phase 2.3 write-side + Phase 3 retrieval.
	tools.RegisterArtifactPropose(s, deps)
	tools.RegisterHarnessInstall(s, deps)
	tools.RegisterArtifactSearch(s, deps)
	tools.RegisterContextForTask(s, deps)

	// Phase 7 revision history.
	tools.RegisterArtifactRevisions(s, deps)
	tools.RegisterArtifactDiff(s, deps)
	tools.RegisterArtifactSummary(s, deps)

	// Task status v2 (migration 0013) — agent-to-agent verification.
	tools.RegisterArtifactVerify(s, deps)

	// Author identity dual (migration 0014) — user row read/update.
	tools.RegisterUserCurrent(s, deps)
	tools.RegisterUserUpdate(s, deps)

	// Phase F revision-shapes — queryable scope graph.
	tools.RegisterScopeInFlight(s, deps)

	return &Server{
		sdk:    s,
		logger: opts.Logger,
	}
}

// Run blocks until the transport returns (client disconnected, ctx cancelled,
// or fatal error). Graceful shutdown on ctx cancel is handled by the SDK.
func (s *Server) Run(ctx context.Context, transport sdk.Transport) error {
	s.logger.Info("mcp server ready",
		"tools", []string{
			"pindoc.ping",
			"pindoc.project.current",
			"pindoc.project.create",
			"pindoc.area.list",
			"pindoc.artifact.read",
			"pindoc.artifact.propose",
			"pindoc.harness.install",
			"pindoc.artifact.search",
			"pindoc.context.for_task",
			"pindoc.artifact.revisions",
			"pindoc.artifact.diff",
			"pindoc.artifact.summary_since",
			"pindoc.artifact.verify",
			"pindoc.user.current",
			"pindoc.user.update",
			"pindoc.scope.in_flight",
		})
	return s.sdk.Run(ctx, transport)
}
