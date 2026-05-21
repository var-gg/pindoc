package tools

import (
	"context"
	"fmt"
	"sort"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// Decision taxonomy-change-operation T15: detectProfileAdoptConflicts is
// the structural conflict detector for a profile.adopt change-set. It runs
// at propose time (so the owner sees conflicts in the dry-run diff before
// approving) and again at apply time (so an unsafe change-set is refused
// before it mutates anything).
//
// structural conflicts BLOCK apply — they describe a plan that cannot
// land cleanly: the active top-level cap would overflow, or a target
// top-level slug already collides with an existing area. The lower
// primitives (CreateTopLevelArea cap/collision checks, executeArtifact-
// Relocation target validation) still backstop apply; this detector
// surfaces those failures up front.
//
// semantic conflict candidates are advisory only — reused top-levels
// beyond the universal misc/_unsorted/cross-cutting overlap. Whether two
// profiles' same-slug areas mean the same concept is a profile-governance
// question deferred to a follow-up Decision (T16); here we only surface
// the candidates and never auto-merge.
func detectProfileAdoptConflicts(ctx context.Context, deps Deps, projectID string, plan profileAdoptPlan) (structural, semantic []string, err error) {
	// Active top-level cap: reused top-levels stay active, and every
	// to_create one is new and active.
	activeAfter := len(plan.TopLevelReused) + len(plan.TopLevelToCreate)
	if activeAfter > projects.MaxActiveTopLevelAreas {
		structural = append(structural, fmt.Sprintf(
			"active top-level cap exceeded: adoption would leave %d active top-levels (cap %d)",
			activeAfter, projects.MaxActiveTopLevelAreas))
	}

	// A to_create slug must not already exist as any area in the project
	// — a top-level slug is unique per project across every depth.
	for _, spec := range plan.TopLevelToCreate {
		var exists bool
		if qErr := deps.DB.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM areas WHERE project_id = $1::uuid AND slug = $2)
		`, projectID, spec.Slug).Scan(&exists); qErr != nil {
			return nil, nil, fmt.Errorf("check slug collision for %q: %w", spec.Slug, qErr)
		}
		if exists {
			structural = append(structural, fmt.Sprintf(
				"slug %q already exists as an area in this project — the new top-level would collide", spec.Slug))
		}
	}

	universal := map[string]bool{"misc": true, "_unsorted": true, "cross-cutting": true}
	for _, slug := range plan.TopLevelReused {
		if !universal[slug] {
			semantic = append(semantic, slug)
		}
	}

	sort.Strings(structural)
	sort.Strings(semantic)
	return structural, semantic, nil
}
