// Package mcp wires the official Go MCP SDK to Pindoc's tool surface.
//
// Tools live in sub-packages under ./tools. This package owns the server
// lifecycle: it constructs the sdk.Server, registers every tool, and
// exposes a single Run entry point the main() binary calls.
package mcp

import (
	"context"
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

	// UserID is the resolved users.id row this MCP session binds to.
	// Empty means the operator hasn't set PINDOC_USER_NAME and the
	// server runs without a user binding. Stamped onto loopback
	// Principals via TrustedLocalResolver. Pre-resolved by main() so
	// the HTTP daemon and MCP layers share one upsert.
	UserID string

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
	sdk                 *sdk.Server
	logger              *slog.Logger
	telemetry           *telemetry.Store
	toolsetListChanged  *toolsetListChangedNotifier
	toolsetChangeNotice toolsetChangeNotice
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
		AssetRoot:             opts.Config.AssetRoot,
		Telemetry:             opts.Telemetry,
		DefaultProjectSlug:    opts.Config.ProjectSlug,
		Transport:             transport,
		AuthProviders:         opts.Config.AuthProviders,
		BindAddr:              opts.Config.BindAddr,
	}
	userID := strings.TrimSpace(opts.UserID)
	if userID == "" {
		userID = upsertStartupUserID(context.Background(), opts.Logger, deps, opts.Config)
	}
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

	deps.AuthChain = authChainForConfig(opts.Config, userID, opts.AgentID)

	// Phase 1 handshake — same registration path as every other tool so
	// the auth chain runs and telemetry records the call.
	tools.RegisterPing(s, deps)
	tools.RegisterRuntimeStatus(s, deps)

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
	tools.RegisterArtifactWordingFix(s, deps)
	tools.RegisterArtifactAddPin(s, deps)
	tools.RegisterArtifactSetVisibility(s, deps)
	tools.RegisterHarnessInstall(s, deps)
	tools.RegisterArtifactAudit(s, deps)
	tools.RegisterArtifactSearch(s, deps)
	tools.RegisterContextForTask(s, deps)

	// Asset v1 — project-scoped LocalFS blobs plus revision-scoped
	// artifact attachments. The blob itself is never exposed through the
	// MCP output; agents get stable pindoc-asset:// refs and metadata.
	tools.RegisterAssetUpload(s, deps)
	tools.RegisterAssetRead(s, deps)
	tools.RegisterAssetAttach(s, deps)

	// Phase 7 revision history.
	tools.RegisterArtifactRevisions(s, deps)
	tools.RegisterArtifactDiff(s, deps)
	tools.RegisterArtifactSummary(s, deps)

	// Layer 2 — read state (migration 0040). Bridge to Layer 4 verification:
	// agents query this to confirm a human has actually read AI revisions
	// before promoting them into the verification candidate lane.
	tools.RegisterArtifactReadState(s, deps)

	// Task operation tools — Decision task-operation-tools-task-assign-
	// 단건-task-bulk-assign-배치-reas. Semantic shortcuts over
	// artifact.propose(shape="meta_patch", task_meta={assignee}) that
	// bypass the search_receipt gate (operational-metadata lane).
	// task.queue is the Reader-parity read model agents should call before
	// claiming the pending Task queue is empty.
	tools.RegisterTaskQueue(s, deps)
	tools.RegisterTaskFlow(s, deps)
	tools.RegisterTaskNext(s, deps)
	tools.RegisterTaskAcceptanceTransition(s, deps)
	tools.RegisterTaskAssign(s, deps)
	tools.RegisterTaskBulkAssign(s, deps)
	tools.RegisterTaskClaimDone(s, deps)
	tools.RegisterTaskDoneCheck(s, deps)

	// Author identity dual (migration 0014) — user row read/update.
	tools.RegisterUserCurrent(s, deps)
	tools.RegisterUserUpdate(s, deps)

	// Phase F revision-shapes — queryable scope graph.
	tools.RegisterScopeInFlight(s, deps)

	toolsetNotice, err := recordToolsetVersion(context.Background(), opts.DB, tools.ToolsetVersion())
	if err != nil {
		opts.Logger.Warn("toolset version state update failed; list_changed notification disabled",
			"toolset_version", tools.ToolsetVersion(),
			"error", err,
		)
	} else if toolsetNotice.Changed {
		opts.Logger.Info("toolset version changed; scheduling tools/list_changed notification",
			"previous_toolset_version", toolsetNotice.Previous,
			"current_toolset_version", toolsetNotice.Current,
		)
	}

	return &Server{
		sdk:                 s,
		logger:              opts.Logger,
		telemetry:           opts.Telemetry,
		toolsetListChanged:  &toolsetListChangedNotifier{notice: toolsetNotice},
		toolsetChangeNotice: toolsetNotice,
	}, nil
}

// authChainForConfig builds the resolver chain from config axes
// (Decision `decision-auth-model-loopback-and-providers`):
//
//   - When AuthProviders includes a Pindoc-AS-backed IdP (`github`),
//     a BearerTokenResolver runs first so requests carrying a valid
//     Bearer JWT (validated by the OAuth middleware before the chain)
//     produce Source=oauth principals.
//   - TrustedLocalResolver runs last as the loopback fastpath. Stdio
//     transports always land here (process trust); HTTP requests land
//     here only when no Bearer is present, which the OAuth middleware
//     allows for loopback addresses (and for AllowPublicUnauthenticated
//     deployments per § 3 of the Decision).
//
// Result: handlers no longer branch on auth_mode strings; Source on
// the produced Principal carries everything they need.
func authChainForConfig(cfg *config.Config, userID, agentID string) *auth.Chain {
	resolvers := []auth.Resolver{}
	if cfg != nil && len(cfg.AuthProviders) > 0 {
		resolvers = append(resolvers, auth.NewBearerTokenResolver(agentID))
	}
	resolvers = append(resolvers, auth.NewTrustedLocalResolver(userID, agentID))
	return auth.NewChain(resolvers...)
}

// Run blocks until the transport returns (client disconnected, ctx cancelled,
// or fatal error). Graceful shutdown on ctx cancel is handled by the SDK.
func (s *Server) Run(ctx context.Context, transport sdk.Transport) error {
	s.logger.Info("mcp server ready",
		"tools", tools.RegisteredTools,
		"toolset_version", tools.ToolsetVersion(),
	)
	s.StartToolsetListChangedNotifier(ctx)
	return s.sdk.Run(ctx, transport)
}

// StartToolsetListChangedNotifier schedules one standards-compliant
// notifications/tools/list_changed push when this process observes that the
// persisted toolset_version changed since the previous server version. It is
// transport-neutral: stdio uses it from Run, while streamable_http starts it
// after wiring the shared SDK server into the HTTP handler.
func (s *Server) StartToolsetListChangedNotifier(ctx context.Context) {
	if s == nil || s.toolsetListChanged == nil {
		return
	}
	s.toolsetListChanged.start(ctx, s.sdk, s.logger)
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
