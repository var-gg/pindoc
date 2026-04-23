package tools

import (
	"log/slog"

	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/receipts"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
)

// Deps is the shared context every tool handler needs. Keeping this tiny on
// purpose — anything added here shows up in every tool's signature and
// becomes an implicit dependency you cannot avoid paying for.
type Deps struct {
	DB      *db.Pool
	Logger  *slog.Logger
	Version string

	// ProjectSlug is resolved on server startup from PINDOC_PROJECT.
	// For Phase 2 the MCP server treats it as "the" project.
	ProjectSlug string

	// ProjectLocale is the `projects.locale` column value for the active
	// ProjectSlug (Task task-phase-18-project-locale-implementation,
	// migration 0015). Same slug can live under multiple locales, so the
	// canonical identity is (slug, locale). Empty until server boot
	// resolves it; HumanURL / AbsHumanURL embed it in the URL path.
	ProjectLocale string

	// UserLanguage is the PINDOC.md / env fallback language the server uses
	// when selecting NOT_READY / suggested_action templates. Phase 5
	// replaces this with a per-project lookup.
	UserLanguage string

	// Embedder generates vectors for chunking on write and for query-side
	// search / context.for_task. Phase 3+.
	Embedder embed.Provider

	// MultiProject mirrors config.MultiProject so tools can advertise
	// whether the instance expects a Project Switcher UI. Independent of
	// the URL scope model.
	MultiProject bool

	// Receipts is the in-memory search-receipt store used to enforce
	// search-before-propose (Phase 11b). Nil-safe: every call site checks
	// before dereferencing, and nil disables the gate (useful for tests).
	Receipts *receipts.Store

	// AgentID is the server-issued identity for this MCP subprocess
	// (Phase 12c). Set once at startup from PINDOC_AGENT_ID env, or
	// generated fresh if unset. Persisted on every artifact_revisions
	// row via source_session_ref so provenance is server-trusted rather
	// than agent-asserted. `author_id` in propose input remains a
	// client-reported display label.
	AgentID string

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

	// UserID is the uuid of the `users` row bound to this MCP session
	// (Decision `decision-author-identity-dual`). Populated at server
	// startup by upserting on PINDOC_USER_NAME; empty when the operator
	// skipped identity setup (artifact.propose then leaves
	// author_user_id NULL). V1.5 OAuth replaces this with a session-
	// resolved principal rather than an env-anchored single user.
	UserID string

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
