package projects

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// MaxActiveTopLevelAreas caps how many active top-level areas a project
// may hold. Decision area-taxonomy-profiled-skeleton: profiles curate
// 8-11 top-level areas and controlled extensions stay within ~12 so the
// navigation shelf does not sprawl (Larson/Czerwinski depth-breadth).
// Decision taxonomy-change-operation T8: only active top-levels count —
// retiring/archived legacy areas do not consume the budget.
const MaxActiveTopLevelAreas = 12

// topLevelAreaSlugRe is the URL-safe area slug shape (lowercase letter +
// kebab tail, 2-40 chars), identical to the project slug shape.
var topLevelAreaSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)

// Sentinel errors so callers map to stable error codes without parsing
// strings (Decision taxonomy-change-operation T9).
var (
	ErrTopLevelAreaSlugInvalid = errors.New("TOP_LEVEL_SLUG_INVALID")
	ErrTopLevelAreaNameInvalid = errors.New("TOP_LEVEL_NAME_INVALID")
	ErrTopLevelAreaCapExceeded = errors.New("TOP_LEVEL_CAP_EXCEEDED")
	ErrTopLevelAreaSlugTaken   = errors.New("TOP_LEVEL_SLUG_TAKEN")
)

// TopLevelAreaSpec is the shape CreateTopLevelArea inserts.
type TopLevelAreaSpec struct {
	Slug           string
	Name           string
	Description    string
	IsCrossCutting bool
	Fileable       bool
	MaxDepth       int
}

// topLevelAreaQuerier is satisfied by pgx.Tx — CreateTopLevelArea always
// runs inside the caller's transaction (project-create seed, or a
// taxonomy-change apply).
type topLevelAreaQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CreateTopLevelArea inserts one active top-level area (parent_id NULL)
// and returns its id. It is the single creation primitive shared by
// project-create seeding (seedAreas) and the runtime taxonomy-change
// apply path (Decision taxonomy-change-operation T9; consumed by T10's
// top_level.add and T14's profile.adopt).
//
// originProfileSlug records which profile the spec came from;
// originChangeID ties a runtime-added area to its taxonomy_changes row.
// Both are empty for project-create seeding.
//
// Facet / one-off slug rejection is intentionally NOT done here — that is
// proposal-gate policy (pindoc.taxonomy.change.propose). This primitive
// validates only what the creation itself requires: slug shape, name, the
// active top-level cap, and slug uniqueness.
func CreateTopLevelArea(ctx context.Context, q topLevelAreaQuerier, projectID string, spec TopLevelAreaSpec, originProfileSlug, originChangeID string) (string, error) {
	slug := strings.ToLower(strings.TrimSpace(spec.Slug))
	name := strings.TrimSpace(spec.Name)
	// `_unsorted` is the reserved quarantine area every profile seeds — the
	// one area slug with a leading underscore. Every other slug, including
	// any runtime-proposed top-level, must match the normal kebab shape so
	// agents cannot smuggle underscore-prefixed system-looking slugs onto
	// the shelf.
	if slug != "_unsorted" && !topLevelAreaSlugRe.MatchString(slug) {
		return "", fmt.Errorf("%w: %q", ErrTopLevelAreaSlugInvalid, spec.Slug)
	}
	if n := len([]rune(name)); n < 2 || n > 60 {
		return "", fmt.Errorf("%w: %q", ErrTopLevelAreaNameInvalid, spec.Name)
	}
	maxDepth := spec.MaxDepth
	if maxDepth < 1 {
		maxDepth = 1
	}

	var activeCount int
	if err := q.QueryRow(ctx, `
		SELECT count(*) FROM areas
		 WHERE project_id = $1::uuid AND parent_id IS NULL AND lifecycle = 'active'
	`, projectID).Scan(&activeCount); err != nil {
		return "", fmt.Errorf("count active top-level areas: %w", err)
	}
	if activeCount+1 > MaxActiveTopLevelAreas {
		return "", fmt.Errorf("%w: %d active top-level areas, cap %d",
			ErrTopLevelAreaCapExceeded, activeCount, MaxActiveTopLevelAreas)
	}

	var descPtr *string
	if d := strings.TrimSpace(spec.Description); d != "" {
		descPtr = &d
	}
	var id string
	err := q.QueryRow(ctx, `
		INSERT INTO areas (
			project_id, parent_id, slug, name, description,
			is_cross_cutting, fileable, max_depth,
			origin_profile_slug, origin_change_id
		) VALUES (
			$1::uuid, NULL, $2, $3, $4,
			$5, $6, $7,
			NULLIF($8, ''), NULLIF($9, '')::uuid
		)
		RETURNING id::text
	`, projectID, slug, name, descPtr, spec.IsCrossCutting, spec.Fileable, maxDepth,
		strings.TrimSpace(originProfileSlug), strings.TrimSpace(originChangeID)).Scan(&id)
	if err != nil {
		if isAreaSlugUniqueViolation(err) {
			return "", fmt.Errorf("%w: %q", ErrTopLevelAreaSlugTaken, slug)
		}
		return "", fmt.Errorf("insert top-level area: %w", err)
	}
	return id, nil
}

// isAreaSlugUniqueViolation reports whether err is the areas (project_id,
// slug) unique-constraint violation — a slug already taken by any area in
// the project, top-level or sub-area.
func isAreaSlugUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "areas_project_id_slug")
}
