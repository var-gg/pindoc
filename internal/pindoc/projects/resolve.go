package projects

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrProjectNotFound is returned by ResolveByOrgAndSlug when no
// (organization_slug, project_slug) pair matches an active row. Distinct
// from a generic "lookup failed" so the HTTP layer can map to 404
// without parsing strings.
var ErrProjectNotFound = errors.New("PROJECT_NOT_FOUND")

// ResolveResult is the minimum projection the URL routing layer needs to
// branch on after resolving /{org}/p/{slug}: project id (for downstream
// joins), the canonical slugs (for redirect-on-rename), and the project
// visibility tier (for the anonymous-vs-member access check).
type ResolveResult struct {
	ProjectID         string
	ProjectSlug       string
	OrgID             string
	OrgSlug           string
	ProjectVisibility string
}

// ResolveByOrgAndSlug looks up the project owned by the given Org slug
// with the given project slug. Used by the future /pindoc.org/{org}/p/{slug}
// route family (and any current handler that wants to be Org-aware).
//
// Joins on organizations.slug through projects.organization_id because
// organization_id is the authoritative FK. The
// query filters on deleted_at IS NULL on both sides so soft-deleted
// orgs/projects don't surface.
func ResolveByOrgAndSlug(ctx context.Context, q queryer, orgSlug, projectSlug string) (*ResolveResult, error) {
	if orgSlug == "" || projectSlug == "" {
		return nil, fmt.Errorf("%w: org_slug and project_slug are required", ErrProjectNotFound)
	}
	var out ResolveResult
	err := q.QueryRow(ctx, `
		SELECT p.id::text, p.slug, o.id::text, o.slug, p.visibility
		  FROM projects p
		  JOIN organizations o ON o.id = p.organization_id
		 WHERE o.slug = $1 AND p.slug = $2 AND o.deleted_at IS NULL
		 LIMIT 1
	`, orgSlug, projectSlug).Scan(&out.ProjectID, &out.ProjectSlug, &out.OrgID, &out.OrgSlug, &out.ProjectVisibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: org=%q project=%q", ErrProjectNotFound, orgSlug, projectSlug)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve project by org+slug: %w", err)
	}
	return &out, nil
}

// IsAccessibleAnonymously is the access-control rule for /pindoc.org/{org}/p/{slug}
// when the requester is unauthenticated. Public projects pass; anything
// else (including the safe-default 'org' tier) returns false so the
// route layer can render a 404 — distinct from "this Org doesn't exist"
// only on the inside but indistinguishable on the wire so existence
// itself isn't leaked.
//
// 'private' projects are reserved for the future enterprise tier where
// even Org members can't see the project unless ACLed in; for OSS Day-1
// only 'public' returns true here.
func (r *ResolveResult) IsAccessibleAnonymously() bool {
	return r != nil && r.ProjectVisibility == VisibilityPublic
}
