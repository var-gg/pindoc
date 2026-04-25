package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// ErrProjectSlugRequired is returned by ResolveProject when the caller
// passed an empty project slug. Account-level connection means every
// project-scoped tool MUST receive a project_slug in its input — empty
// is a programmer / harness bug, not a runtime auth failure, so the
// sentinel lets handlers map it to the PROJECT_SLUG_REQUIRED not_ready
// code without sniffing error strings.
var ErrProjectSlugRequired = errors.New("auth: project_slug is required for this tool")

// ErrProjectNotFound is returned when the slug doesn't resolve to any
// row in the projects table. Distinguishing this from "auth refused"
// matters: the caller may have mistyped the slug (404-equivalent) or
// genuinely lack access (403-equivalent). V1 trusted_local can only see
// the former because every project is visible.
var ErrProjectNotFound = errors.New("auth: project not found for the given slug")

// ProjectScope is the per-call answer to "which project, with what
// role". Returned by ResolveProject after the handler reads the
// project_slug input field. Handlers downstream pull ProjectID /
// ProjectSlug / ProjectLocale / Role off this struct rather than off
// the Principal — Principal is account-level (Decision
// mcp-scope-account-level-industry-standard).
type ProjectScope struct {
	// ProjectID is the projects.id (uuid) row this call is scoped to.
	// Always populated on a successful Resolve — handlers writing to
	// foreign-key columns (events.project_id, etc.) use this directly.
	ProjectID string

	// ProjectSlug is the projects.slug for the active scope. Carried so
	// handlers building HumanURL / share links / log lines don't have
	// to re-query the row they just resolved.
	ProjectSlug string

	// ProjectLocale is the project's canonical language metadata. It is
	// loaded from projects.primary_language; the old projects.locale
	// identity column was dropped by task-canonical-locale-migration.
	// Kept as a compatibility field for handlers that still need to show
	// the canonical language in responses. HumanURL ignores it because
	// share paths are now /p/{slug}/wiki/...
	ProjectLocale string

	// Role is the caller's permission tier within this project. V1
	// trusted_local always emits "owner" — handlers should already
	// route through Can(action) rather than role-string equality so
	// V1.5 ACL (project_members table with editor / viewer rows) is a
	// data change, not a handler edit.
	Role string
}

// roleActions enumerates which Role values are permitted to invoke
// each named action. V1 ships with one role ("owner") that satisfies
// every action — adding "editor" / "viewer" later is a map edit rather
// than a handler audit. Can() returns false for unknown actions on
// purpose so a typo at the call site fails closed.
var roleActions = map[string]map[string]bool{
	// Read actions: any authenticated principal can pull artifact /
	// project metadata. V1.5+ "viewer" role still satisfies these.
	"read.project":  {"owner": true, "editor": true, "viewer": true},
	"read.artifact": {"owner": true, "editor": true, "viewer": true},
	"read.area":     {"owner": true, "editor": true, "viewer": true},

	// Write actions: artifact.propose, area.create, project.create,
	// task.assign — owner + editor only. Viewer is read-only.
	"write.artifact": {"owner": true, "editor": true},
	"write.area":     {"owner": true, "editor": true},
	"write.task":     {"owner": true, "editor": true},
	"write.project":  {"owner": true},

	// Telemetry / capability surfaces — open to anyone authenticated.
	"read.capabilities": {"owner": true, "editor": true, "viewer": true},
}

// Can reports whether this ProjectScope's role permits the named
// action. Returns false on unknown action names so a typo fails closed
// instead of silently allowing the call. Nil receiver also returns
// false (handlers must check err from ResolveProject before relying on
// scope).
func (s *ProjectScope) Can(action string) bool {
	if s == nil {
		return false
	}
	roles, ok := roleActions[action]
	if !ok {
		return false
	}
	return roles[s.Role]
}

// ResolveProject looks up the project row and returns the scope this
// caller has within it. Three failure modes:
//
//   - empty slug → ErrProjectSlugRequired (handler bug)
//   - slug not in projects table → ErrProjectNotFound (caller mistyped)
//   - DB error → wrapped with %w
//
// V1 trusted_local: every Principal sees every project as owner — the
// project_members table doesn't ship until V1.5 OAuth lands, so we
// short-circuit the membership lookup and just stamp owner. V1.5+
// (oauth_github mode) replaces the body with a project_members JOIN
// keyed on Principal.UserID.
func ResolveProject(ctx context.Context, pool *db.Pool, p *Principal, slug string) (*ProjectScope, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, ErrProjectSlugRequired
	}
	if pool == nil {
		return nil, errors.New("auth: ResolveProject called with nil DB pool")
	}

	var (
		projectID       string
		primaryLanguage string
	)
	err := pool.QueryRow(ctx,
		`SELECT id::text, primary_language FROM projects WHERE slug = $1 LIMIT 1`,
		slug,
	).Scan(&projectID, &primaryLanguage)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", ErrProjectNotFound, slug)
	}
	if err != nil {
		return nil, fmt.Errorf("auth: lookup project %q: %w", slug, err)
	}

	role := resolveRole(p)
	return &ProjectScope{
		ProjectID:     projectID,
		ProjectSlug:   slug,
		ProjectLocale: primaryLanguage,
		Role:          role,
	}, nil
}

// resolveRole picks the Role for the (Principal, project) pair. V1
// trusted_local stamps owner unconditionally — single-user self-host
// has no meaningful role distinction yet. V1.5+ branches on
// p.AuthMode: oauth_github queries project_members(user_id, role) and
// returns the looked-up tier; trusted_local stays owner so OSS
// self-host upgrades don't change behaviour.
func resolveRole(p *Principal) string {
	if p == nil {
		return ""
	}
	// Future: switch on p.AuthMode once project_members lands.
	return RoleOwner
}
