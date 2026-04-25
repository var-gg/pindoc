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
	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
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

	// Telemetry is the async MCP tool-call logger (Phase J). When set,
	// every Register* call wraps its handler with Instrument() so raw
	// (tool, duration, bytes, tokens) lands in mcp_tool_calls without
	// impacting response latency.
	Telemetry *telemetry.Store

	// ProjectSlug overrides Config.ProjectSlug for this Server instance.
	// Streamable-HTTP daemons populate this per-connection from the
	// /mcp/p/{project} URL so each MCP session lands in its own project.
	// Empty falls back to Config.ProjectSlug — the stdio path. Caller is
	// responsible for validating the slug (e.g. exists in `projects`)
	// before calling NewServer.
	ProjectSlug string

	// Transport identifies which transport built this Server, propagated
	// into Deps and surfaced in pindoc.project.current capabilities. One
	// of "stdio" | "streamable_http". Empty falls back to "stdio" so the
	// existing subprocess path keeps advertising fixed_session.
	Transport string
}

type Server struct {
	sdk       *sdk.Server
	logger    *slog.Logger
	telemetry *telemetry.Store
}

func NewServer(opts Options) *Server {
	impl := &sdk.Implementation{
		Name:    opts.Name,
		Version: opts.Version,
	}
	s := sdk.NewServer(impl, nil)

	// Resolve the active project slug for this Server. ProjectSlug from
	// Options wins so streamable-HTTP daemons can pin per-connection;
	// stdio callers leave it empty and inherit Config.ProjectSlug.
	projectSlug := opts.ProjectSlug
	if projectSlug == "" {
		projectSlug = opts.Config.ProjectSlug
	}
	transport := opts.Transport
	if transport == "" {
		transport = "stdio"
	}

	// Phase 2 read-side: project context + scope enumeration + artifact fetch.
	deps := tools.Deps{
		DB:           opts.DB,
		Logger:       opts.Logger,
		Version:      opts.Version,
		ProjectSlug:  projectSlug,
		UserLanguage: opts.Config.UserLanguage,
		Embedder:     opts.Embedder,
		MultiProject: opts.Config.MultiProject,
		Receipts:     receipts.New(0), // DefaultTTL applies
		AgentID:      opts.AgentID,
		Settings:     opts.Settings,
		RepoRoot:     opts.Config.RepoRoot,
		Telemetry:    opts.Telemetry,
		Transport:    transport,
	}
	deps.UserID = upsertStartupUserID(context.Background(), opts.Logger, deps, opts.Config)
	deps.ProjectLocale = resolveStartupProjectLocale(context.Background(), opts.Logger, deps)

	// Phase 1 handshake (Ping has its own Deps subset — still
	// instrumented via the shared Telemetry store passed in opts).
	tools.RegisterPing(s, tools.PingDeps{
		Version:      opts.Version,
		UserLanguage: opts.Config.UserLanguage,
		Telemetry:    opts.Telemetry,
		AgentID:      opts.AgentID,
		ProjectSlug:  projectSlug,
	})

	tools.RegisterProjectCurrent(s, deps)
	tools.RegisterProjectCreate(s, deps)
	tools.RegisterAreaList(s, deps)
	tools.RegisterAreaCreate(s, deps)
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

	// Task operation tools — Decision task-operation-tools-task-assign-
	// 단건-task-bulk-assign-배치-reas. Semantic shortcuts over
	// artifact.propose(shape="meta_patch", task_meta={assignee}) that
	// bypass the search_receipt gate (operational-metadata lane).
	// task.queue is the Reader-parity read model agents should call before
	// claiming the pending Task queue is empty.
	tools.RegisterTaskQueue(s, deps)
	tools.RegisterTaskAssign(s, deps)
	tools.RegisterTaskBulkAssign(s, deps)

	// Author identity dual (migration 0014) — user row read/update.
	tools.RegisterUserCurrent(s, deps)
	tools.RegisterUserUpdate(s, deps)

	// Phase F revision-shapes — queryable scope graph.
	tools.RegisterScopeInFlight(s, deps)

	return &Server{
		sdk:       s,
		logger:    opts.Logger,
		telemetry: opts.Telemetry,
	}
}

// Run blocks until the transport returns (client disconnected, ctx cancelled,
// or fatal error). Graceful shutdown on ctx cancel is handled by the SDK.
func (s *Server) Run(ctx context.Context, transport sdk.Transport) error {
	s.logger.Info("mcp server ready",
		"tools", tools.RegisteredTools,
		"toolset_version", tools.ToolsetVersion(),
	)
	return s.sdk.Run(ctx, transport)
}

// SDK returns the underlying go-sdk Server for callers that need to plug
// it into a transport other than (*Server).Run — most notably the
// streamable-HTTP `getServer(req) *sdk.Server` callback in daemon mode,
// which builds a fresh project-scoped Server per HTTP connection. The
// stdio path keeps using Run() and never needs this.
func (s *Server) SDK() *sdk.Server {
	return s.sdk
}
