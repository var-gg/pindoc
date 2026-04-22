package tools

import (
	"log/slog"

	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/receipts"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
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
}

// AbsHumanURL builds an absolute share URL from the current settings. Empty
// when PublicBaseURL isn't configured — callers should treat absence as
// "operator hasn't set a base URL yet; fall back to human_url relative
// path".
func AbsHumanURL(s *settings.Store, projectSlug, artifactSlug string) string {
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
	return base + HumanURL(projectSlug, artifactSlug)
}

// HumanURL returns the canonical /p/:project/wiki/:slug relative URL used
// in all agent-to-human share links. Agents paste this into chat so the
// user can click through to the reader. Relative on purpose — the hosting
// origin is the user's deployment (self-host first), the agent does not
// know the external base URL.
func HumanURL(projectSlug, artifactSlug string) string {
	return "/p/" + projectSlug + "/wiki/" + artifactSlug
}
