package tools

import (
	"log/slog"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/receipts"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
)

// Deps is the shared infrastructure every tool handler needs. Caller-
// identity (UserID, AgentID) lives on *auth.Principal which the
// AddInstrumentedTool wrapper resolves per-call via AuthChain and
// threads as an explicit handler argument (Decision principal-
// resolver-architecture). Per-call project scope (ProjectID,
// ProjectSlug, ProjectLocale, Role) lives on *auth.ProjectScope which
// handlers resolve from each tool input's project_slug field via
// auth.ResolveProject (Decision mcp-scope-account-level-industry-
// standard). Both abstractions keep handler bodies blind to auth_mode
// so adding BearerToken / OAuth resolvers later changes zero handler
// code.
//
// Keeping this struct small on purpose — anything added here shows up
// in every tool's signature and becomes an implicit dependency you
// cannot avoid paying for.
type Deps struct {
	DB      *db.Pool
	Logger  *slog.Logger
	Version string

	// AuthChain resolves the calling Principal for each tool invocation.
	// V1 wires a single TrustedLocalResolver; V1.5+ prepends
	// BearerTokenResolver / OAuthSessionResolver. Nil chains short-
	// circuit AddInstrumentedTool with ErrNoResolverMatched so partial
	// wiring fails loud at first request rather than silently
	// authenticating as nobody.
	AuthChain *auth.Chain

	// UserLanguage is the PINDOC.md / env fallback language the server uses
	// when selecting NOT_READY / suggested_action templates. Phase 5
	// replaces this with a per-project lookup.
	UserLanguage string

	// DefaultProjectSlug is the env-derived (PINDOC_PROJECT) project
	// the operator considers their primary one. Account-level scope
	// (Decision mcp-scope-account-level-industry-standard) means tools
	// take a project_slug input — this default is the fallback when the
	// caller passes an empty slug to project.current / user.update so
	// existing single-project setups keep working without harness
	// changes. Empty when the env isn't set; handlers must then surface
	// a stable not_ready code rather than guess.
	DefaultProjectSlug string

	// Transport identifies which MCP transport built this Server.
	// Carried purely so capability advertisement can echo it for
	// telemetry / debugging — account-level scope means the value no
	// longer drives scope_mode branching. One of "stdio" |
	// "streamable_http"; empty falls back to "stdio" inside
	// buildCapabilities.
	Transport string

	// Embedder generates vectors for chunking on write and for query-side
	// search / context.for_task. Phase 3+.
	Embedder embed.Provider

	// Receipts is the in-memory search-receipt store used to enforce
	// search-before-propose (Phase 11b). Nil-safe: every call site checks
	// before dereferencing, and nil disables the gate (useful for tests).
	Receipts *receipts.Store

	// Settings is the operator-editable config store (Phase 14a). Nil-
	// safe: capability reporting falls back to defaults, and human_url_abs
	// is simply omitted when PublicBaseURL is empty.
	Settings *settings.Store

	// RepoRoot is the absolute path to the working-tree root the agent is
	// pinning against. Optional; loaded from PINDOC_REPO_ROOT. When set,
	// artifact.propose statically verifies each kind="code" pin's path and
	// emits a PIN_PATH_NOT_FOUND warning on accepted responses if the file
	// is missing at HEAD. Empty = validation disabled (V1.5 git-pinner
	// takes over). Pure warning, never blocks.
	RepoRoot string

	// Telemetry is the async MCP tool-call logger (Phase J). Nil-safe
	// — Instrument() no-ops when absent, so tests that don't care
	// about observability can leave it unset.
	Telemetry *telemetry.Store
}

// AbsHumanURL builds an absolute share URL from the current settings. Empty
// when PublicBaseURL isn't configured — callers should treat absence as
// "operator hasn't set a base URL yet; fall back to human_url relative
// path".
func AbsHumanURL(s *settings.Store, projectSlug, projectLocale, artifactSlug string) string {
	if s == nil {
		return ""
	}
	base := s.Get().PublicBaseURL
	if base == "" {
		return ""
	}
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + HumanURL(projectSlug, projectLocale, artifactSlug)
}

// HumanURL returns the canonical /p/:project/:locale/wiki/:slug relative
// URL used in all agent-to-human share links (Task task-phase-18-project-
// locale-implementation adds the locale segment between slug and wiki).
// Agents paste this into chat so the user can click through to the
// reader. Relative on purpose — the hosting origin is the user's
// deployment (self-host first), the agent does not know the external
// base URL. Empty `projectLocale` falls back to "en" so pre-migration
// call sites still emit a valid-looking URL.
func HumanURL(projectSlug, projectLocale, artifactSlug string) string {
	locale := projectLocale
	if locale == "" {
		locale = "en"
	}
	return "/p/" + projectSlug + "/" + locale + "/wiki/" + artifactSlug
}
