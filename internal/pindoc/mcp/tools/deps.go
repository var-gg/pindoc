package tools

import (
	"log/slog"

	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
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
}
