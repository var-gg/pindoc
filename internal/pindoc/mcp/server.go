// Package mcp wires the official Go MCP SDK to Pindoc's tool surface.
//
// Tools live in sub-packages under ./tools. This package owns the server
// lifecycle: it constructs the sdk.Server, registers every tool, and
// exposes a single Run entry point the main() binary calls.
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/mcp/tools"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
	"github.com/var-gg/pindoc/internal/pindoc/receipts"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
)

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

	// Transport identifies which transport built this Server. Carried
	// purely for telemetry / capability advertisement; account-level
	// scope (Decision mcp-scope-account-level-industry-standard) means
	// scope_mode no longer branches on it. One of "stdio" |
	// "streamable_http"; empty falls back to "stdio".
	Transport string
}

type Server struct {
	sdk       *sdk.Server
	logger    *slog.Logger
	telemetry *telemetry.Store
}

func NewServer(opts Options) (*Server, error) {
	impl := &sdk.Implementation{
		Name:    opts.Name,
		Version: opts.Version,
	}
	s := sdk.NewServer(impl, nil)

	transport := opts.Transport
	if transport == "" {
		transport = "stdio"
	}

	// Deps carries pure infrastructure — caller-identity lives on the
	// Principal that the AuthChain produces per call (Decision
	// principal-resolver-architecture); per-call project scope lives
	// on auth.ProjectScope which handlers resolve from each tool
	// input's project_slug field (Decision mcp-scope-account-level-
	// industry-standard). Build deps first so upsertStartupUserID can
	// use it for the env-anchored user upsert, then layer the chain on
	// top once we know UserID.
	deps := tools.Deps{
		DB:                    opts.DB,
		Logger:                opts.Logger,
		Version:               opts.Version,
		UserLanguage:          opts.Config.UserLanguage,
		ReceiptExemptionLimit: opts.Config.ReceiptExemptionLimit,
		Embedder:              opts.Embedder,
		Receipts:              receipts.New(0), // DefaultTTL applies
		Settings:              opts.Settings,
		RepoRoot:              opts.Config.RepoRoot,
		Telemetry:             opts.Telemetry,
		DefaultProjectSlug:    opts.Config.ProjectSlug,
		Transport:             transport,
		AuthMode:              opts.Config.AuthMode,
	}
	userID := upsertStartupUserID(context.Background(), opts.Logger, deps, opts.Config)
	if err := projects.EnsureDefaultProjectOwnerMembership(context.Background(), opts.DB, opts.Config.ProjectSlug, userID); err != nil {
		opts.Logger.Warn("default project owner membership bootstrap failed",
			"project_slug", opts.Config.ProjectSlug,
			"user_id", userID,
			"error", err,
		)
	} else if strings.TrimSpace(userID) != "" {
		opts.Logger.Info("default project owner membership bootstrap complete",
			"project_slug", opts.Config.ProjectSlug,
			"user_id", userID,
		)
	}

	authChain, err := authChainForMode(opts.Config.AuthMode, userID, opts.AgentID)
	if err != nil {
		return nil, err
	}
	deps.AuthChain = authChain

	// Phase 1 handshake — same registration path as every other tool so
	// the auth chain runs and telemetry records the call.
	tools.RegisterPing(s, deps)

	tools.RegisterProjectCurrent(s, deps)
	tools.RegisterProjectCreate(s, deps)
	tools.RegisterProjectExport(s, deps)
	tools.RegisterWorkspaceDetect(s, deps)
	tools.RegisterAreaList(s, deps)
	tools.RegisterAreaCreate(s, deps)
	tools.RegisterArtifactRead(s, deps)
	tools.RegisterArtifactTranslate(s, deps)

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
	tools.RegisterTaskAcceptanceTransition(s, deps)
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
	}, nil
}

func authChainForMode(mode config.AuthMode, userID, agentID string) (*auth.Chain, error) {
	if mode == "" {
		mode = config.AuthModeTrustedLocal
	}
	switch mode {
	case config.AuthModeTrustedLocal:
		return auth.NewChain(auth.NewTrustedLocalResolver(userID, agentID)), nil
	case config.AuthModeOAuthGitHub:
		return auth.NewChain(auth.NewBearerTokenResolver(agentID)), nil
	case config.AuthModePublicReadonly, config.AuthModeSingleUser:
		return nil, fmt.Errorf("PINDOC_AUTH_MODE=%s is not supported yet in V1; use trusted_local", mode)
	default:
		return nil, fmt.Errorf("invalid PINDOC_AUTH_MODE: '%s'. valid: %s", mode, config.ValidAuthModesString())
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
// streamable-HTTP `getServer(req) *sdk.Server` callback in daemon mode.
// Account-level scope (Decision mcp-scope-account-level-industry-
// standard) means one Server instance handles every connection — the
// callback returns the same *sdk.Server for every request rather than
// rebuilding per-project.
func (s *Server) SDK() *sdk.Server {
	return s.sdk
}
