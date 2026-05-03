package projects

import (
	"context"
	"fmt"
	"strings"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// Visibility tiers for projects (and artifacts). Mirrors the CHECK
// constraint on projects.visibility / artifacts.visibility (migration
// 0050). Public is unauthenticated-readable; Org is members-only;
// Private is creator+ACL only. Default 'org' is the safe-by-default
// choice — nothing leaks publicly without an explicit opt-in.
const (
	VisibilityPublic  = "public"
	VisibilityOrg     = "org"
	VisibilityPrivate = "private"
)

// ViewerScope describes who is asking for the visibility-filtered list,
// so CountVisible / ListVisible can pick the right WHERE clause without
// each call site re-implementing the rule. Anonymous viewers see
// 'public' only; Org members see 'public'+'org' within their Orgs;
// the same user across different Orgs is handled by the OrgIDs slice
// (the union of every Org they have a membership row in).
type ViewerScope struct {
	// TrustedLocal preserves the single-user self-host behavior for
	// loopback/process-trusted callers. It is intentionally explicit so
	// the zero value fails closed to public-only instead of silently
	// counting every project.
	TrustedLocal bool
	// AnonymousOnly forces the query to behave as if no user is logged in,
	// even when UserID/OrgIDs are populated. Used by /pindoc.org/{org}/...
	// public profile rendering: the same Reader code path serves both
	// authenticated and anonymous visitors, and the route layer flips
	// this flag based on the URL, not on auth state.
	AnonymousOnly bool
	// UserID is the caller's users.id. Empty string means anonymous.
	UserID string
	// OrgIDs is the set of organizations.id values the caller is a
	// member of. May be empty (anonymous, or a logged-in user who
	// hasn't joined any team Org yet).
	OrgIDs []string
}

// CountVisible returns the number of projects visible to the given
// caller. The query joins on visibility + Org membership:
//
//   - 'public' rows: visible to everyone unconditionally.
//   - 'org' rows:    visible only when the caller's OrgIDs include
//     the project's organization_id.
//   - 'private' rows: not visible at this layer; per-artifact ACL
//     handles within-Org owner-only items, not whole projects. (A
//     private *project* would mean "even Org members can't see this
//     project exists" which is the future Phase 3 enterprise tier;
//     for now no flow sets projects.visibility='private'.)
//
// Backwards compat: legacy string user IDs normalize to TrustedLocal,
// but new call sites should pass ViewerScope explicitly. The ViewerScope
// zero value is public-only so accidental empty scopes fail closed.
func CountVisible(ctx context.Context, pool *db.Pool, scopeOrUserID any) (int, error) {
	scope := normalizeScope(scopeOrUserID)
	q, args := buildVisibilitySelect(scope, "SELECT count(*) FROM projects")
	var n int
	err := pool.QueryRow(ctx, q, args...).Scan(&n)
	return n, err
}

// IsMultiProject is the derivation rule the MCP `pindoc.project.current`
// capabilities block and the HTTP `/api/config` payload share. Two or
// more visible projects flips Reader's project switcher on; a single
// project keeps the chrome-less single-tenant look. Pure function so
// the rule is testable without a DB.
func IsMultiProject(visibleCount int) bool {
	return visibleCount > 1
}

// MultiProjectCaps is the Reader-facing split of the legacy
// `multi_project` flag. Switching answers "should project-switcher chrome
// exist"; creation answers "may the Reader expose the web project-create
// entrypoint". Creation is intentionally permissive today because role /
// plan policy is not yet modeled at this layer.
type MultiProjectCaps struct {
	MultiProjectSwitching bool
	ProjectCreateAllowed  bool
}

func CapabilitiesForVisibleCount(visibleCount int) MultiProjectCaps {
	return MultiProjectCaps{
		MultiProjectSwitching: IsMultiProject(visibleCount),
		ProjectCreateAllowed:  true,
	}
}

// normalizeScope accepts either a string (legacy userID call site, kept
// for the trusted_local migration path) or a ViewerScope (new shape).
// The any-typed parameter avoids a breaking change to existing callers
// while the rollout progresses; once every call site passes ViewerScope
// the signature can tighten.
func normalizeScope(in any) ViewerScope {
	switch v := in.(type) {
	case ViewerScope:
		return v
	case string:
		userID := strings.TrimSpace(v)
		if userID == "" {
			return ViewerScope{AnonymousOnly: true}
		}
		// Legacy non-empty userID calls are trusted_local for backwards
		// compatibility. New call sites pass ViewerScope explicitly so the
		// trust decision stays visible at the edge.
		return ViewerScope{UserID: userID, TrustedLocal: true}
	default:
		return ViewerScope{}
	}
}

// buildVisibilitySelect appends the WHERE clause that enforces the
// visibility tiers. Splits into three branches:
//   - TrustedLocal: no WHERE (loopback/process trust)
//   - AnonymousOnly: WHERE visibility = 'public'
//   - authenticated: public, org memberships, direct project memberships,
//     and private projects only when directly joined
func buildVisibilitySelect(scope ViewerScope, base string) (string, []any) {
	if scope.TrustedLocal {
		return base, nil
	}
	if scope.AnonymousOnly {
		return base + " WHERE visibility = $1", []any{VisibilityPublic}
	}
	userID := strings.TrimSpace(scope.UserID)
	if userID == "" && len(scope.OrgIDs) == 0 {
		return base + " WHERE visibility = $1", []any{VisibilityPublic}
	}
	clauses := []string{"visibility = $1"}
	args := []any{VisibilityPublic}
	if len(scope.OrgIDs) > 0 {
		n := len(args) + 1
		clauses = append(clauses, fmt.Sprintf("(visibility = $%d AND organization_id::text = ANY($%d))", n, n+1))
		args = append(args, VisibilityOrg, scope.OrgIDs)
	}
	if userID != "" {
		n := len(args) + 1
		projectMember := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM project_members pm
			 WHERE pm.project_id = projects.id
			   AND pm.user_id::text = $%d
		)`, n+2)
		orgMember := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM organization_members om
			 WHERE om.organization_id = projects.organization_id
			   AND om.user_id::text = $%d
		)`, n+2)
		membership := "(" + projectMember + " OR " + orgMember + ")"
		clauses = append(clauses,
			fmt.Sprintf("(visibility = $%d AND %s)", n, membership),
			fmt.Sprintf("(visibility = $%d AND %s)", n+1, projectMember),
		)
		args = append(args, VisibilityOrg, VisibilityPrivate, userID)
	}
	return base + " WHERE " + strings.Join(clauses, " OR "), args
}
