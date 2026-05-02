package projects

import (
	"context"
	"fmt"

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
// Backwards compat: if scope.UserID is empty AND OrgIDs is empty AND
// AnonymousOnly is false, the function falls back to "trusted_local"
// behavior (return total row count) so V1 self-host single-user
// callers that haven't migrated to the scoped API still work.
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
		// Legacy call: just a userID. Treat as trusted_local — return
		// every row regardless of visibility. Phase D ACL work replaces
		// this branch with a real membership lookup.
		return ViewerScope{UserID: v}
	default:
		return ViewerScope{}
	}
}

// buildVisibilitySelect appends the WHERE clause that enforces the
// visibility tiers. Splits into three branches:
//   - AnonymousOnly: WHERE visibility = 'public'
//   - has Org memberships: WHERE visibility = 'public'
//                            OR (visibility = 'org' AND organization_id = ANY($N))
//   - logged-in but no Org memberships, AnonymousOnly false:
//                          legacy trusted_local — return everything
//                          (single-user self-host preserves V1 behavior)
func buildVisibilitySelect(scope ViewerScope, base string) (string, []any) {
	if scope.AnonymousOnly {
		return base + " WHERE visibility = $1", []any{VisibilityPublic}
	}
	if len(scope.OrgIDs) > 0 {
		return base + fmt.Sprintf(
				" WHERE visibility = $1 OR (visibility = $2 AND organization_id::text = ANY($3))"),
			[]any{VisibilityPublic, VisibilityOrg, scope.OrgIDs}
	}
	// Trusted_local fallback: no scoping, full row count. Preserves V1
	// self-host behavior while the membership-aware path rolls out.
	return base, nil
}
