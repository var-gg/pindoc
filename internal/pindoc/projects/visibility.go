package projects

import (
	"context"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// CountVisible returns the number of projects visible to the given caller.
// V1 trusted_local: returns total row count from the projects table — the
// local subprocess sees every project. userID is accepted but ignored at
// V1; V1.5+ replaces the body with an ACL filter on owner_id / team
// membership using that argument, so the signature stays stable across
// the auth rollout.
//
// The projects table currently has no `status` column (no soft-delete /
// archived flag in the V1 schema). When that column lands the WHERE
// clause grows here — call sites stay untouched.
func CountVisible(ctx context.Context, pool *db.Pool, userID string) (int, error) {
	_ = userID
	var n int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM projects`).Scan(&n)
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
