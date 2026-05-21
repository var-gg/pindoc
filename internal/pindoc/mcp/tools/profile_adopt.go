package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// Decision taxonomy-change-operation T13: profile.adopt is the change-set
// kind that lets an existing project take on a different taxonomy
// profile's top-level skeleton without losing identity. This file is the
// PLANNER — propose(kind=profile.adopt) computes the dry-run diff and
// records the change-set; it mutates nothing. T14 is the apply.

// relocationMoveInput is the agent-facing relocation map entry.
type relocationMoveInput struct {
	ToAreaSlug    string   `json:"to_area_slug" jsonschema:"target area slug an artifact group moves into"`
	ArtifactSlugs []string `json:"artifact_slugs" jsonschema:"artifact slugs or ids to move into to_area_slug"`
}

// profileAdoptTopLevelSpec is a top-level area apply will create.
type profileAdoptTopLevelSpec struct {
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Fileable       bool   `json:"fileable"`
	MaxDepth       int    `json:"max_depth"`
	IsCrossCutting bool   `json:"is_cross_cutting"`
}

// profileAdoptPlan is the plan_json a profile.adopt change-set stores —
// the exact instruction set T14's apply executes. Slices are sorted so
// plan_hash is deterministic.
type profileAdoptPlan struct {
	Kind              string                     `json:"kind"`
	ProjectID         string                     `json:"project_id"`
	SourceProfileSlug string                     `json:"source_profile_slug"`
	TargetProfileSlug string                     `json:"target_profile_slug"`
	TopLevelToCreate  []profileAdoptTopLevelSpec `json:"top_level_to_create"`
	TopLevelReused    []string                   `json:"top_level_reused"`
	TopLevelToRetire  []string                   `json:"top_level_to_retire"`
	Relocations       []artifactRelocationMove   `json:"relocations"`
}

// profileAdoptRetireInfo describes one legacy top-level the adoption
// retires: its subtree artifact count (the unmapped inventory left in
// legacy) and whether it is empty enough to archive right away.
type profileAdoptRetireInfo struct {
	Slug          string `json:"slug"`
	ArtifactCount int    `json:"artifact_count"`
	WouldArchive  bool   `json:"would_archive"`
}

// profileAdoptDiff is the review-facing dry-run summary returned by
// propose and stored as diff_json.
type profileAdoptDiff struct {
	SourceProfileSlug   string                   `json:"source_profile_slug"`
	TargetProfileSlug   string                   `json:"target_profile_slug"`
	TopLevelToCreate    []string                 `json:"top_level_to_create"`
	TopLevelReused      []string                 `json:"top_level_reused"`
	TopLevelToRetire    []profileAdoptRetireInfo `json:"top_level_to_retire"`
	RelocationMoves     int                      `json:"relocation_moves"`
	RelocationArtifacts int                      `json:"relocation_artifacts"`
}

type currentTopLevel struct {
	id        string
	lifecycle string
}

// proposeProfileAdopt plans a profile.adopt change-set: which target
// top-levels to create vs reuse, which current top-levels become
// retiring, the agent's relocation map, and the legacy inventory left
// behind. It records the change-set but mutates no area or artifact.
func proposeProfileAdopt(ctx context.Context, deps Deps, p *auth.Principal, projectID, projectSlug string, in taxonomyChangeProposeInput) (*sdk.CallToolResult, taxonomyChangeProposeOutput, error) {
	targetSlug := strings.TrimSpace(in.TargetProfileSlug)
	if targetSlug == "" {
		return nil, taxonomyChangeNotReady("TARGET_PROFILE_REQUIRED",
			"kind=profile.adopt requires target_profile_slug."), nil
	}
	targetProfile, ok := projects.TaxonomyProfileBySlug(targetSlug)
	if !ok {
		return nil, taxonomyChangeNotReady("TARGET_PROFILE_UNKNOWN",
			fmt.Sprintf("taxonomy profile %q is not registered.", targetSlug)), nil
	}

	var sourceProfileSlug, projectLang string
	if err := deps.DB.QueryRow(ctx, `
		SELECT taxonomy_profile_slug, primary_language FROM projects WHERE id = $1::uuid
	`, projectID).Scan(&sourceProfileSlug, &projectLang); err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("load project profile: %w", err)
	}
	if sourceProfileSlug == targetProfile.Slug {
		return nil, taxonomyChangeNotReady("PROFILE_UNCHANGED",
			fmt.Sprintf("project already uses the %q taxonomy profile.", targetProfile.Slug)), nil
	}

	current, err := loadCurrentTopLevels(ctx, deps, projectID)
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, err
	}

	targetSlugs := map[string]bool{}
	var toCreate []profileAdoptTopLevelSpec
	var reused []string
	for _, row := range targetProfile.TopLevel {
		targetSlugs[row.Slug] = true
		if _, exists := current[row.Slug]; exists {
			reused = append(reused, row.Slug)
			continue
		}
		toCreate = append(toCreate, profileAdoptTopLevelSpec{
			Slug:           row.Slug,
			Name:           row.Name,
			Description:    projects.LocalizedAreaDescription(row.DescriptionEN, row.DescriptionKO, projectLang),
			Fileable:       row.Fileable,
			MaxDepth:       row.MaxDepth,
			IsCrossCutting: row.IsCrossCutting,
		})
	}
	var toRetire []string
	for slug, cur := range current {
		if cur.lifecycle == "active" && !targetSlugs[slug] {
			toRetire = append(toRetire, slug)
		}
	}
	sort.Strings(reused)
	sort.Strings(toRetire)
	sort.Slice(toCreate, func(i, j int) bool { return toCreate[i].Slug < toCreate[j].Slug })

	relocations := normalizeRelocationMoves(in.RelocationMap)

	retireInfo := make([]profileAdoptRetireInfo, 0, len(toRetire))
	for _, slug := range toRetire {
		count, err := areaSubtreeArtifactCount(ctx, deps, current[slug].id)
		if err != nil {
			return nil, taxonomyChangeProposeOutput{}, err
		}
		retireInfo = append(retireInfo, profileAdoptRetireInfo{
			Slug: slug, ArtifactCount: count, WouldArchive: count == 0,
		})
	}

	relocArtifacts := 0
	for _, m := range relocations {
		relocArtifacts += len(m.ArtifactIDs)
	}

	plan := profileAdoptPlan{
		Kind:              taxonomyChangeKindProfileAdopt,
		ProjectID:         projectID,
		SourceProfileSlug: sourceProfileSlug,
		TargetProfileSlug: targetProfile.Slug,
		TopLevelToCreate:  toCreate,
		TopLevelReused:    reused,
		TopLevelToRetire:  toRetire,
		Relocations:       relocations,
	}
	diff := profileAdoptDiff{
		SourceProfileSlug:   sourceProfileSlug,
		TargetProfileSlug:   targetProfile.Slug,
		TopLevelToCreate:    topLevelSpecSlugs(toCreate),
		TopLevelReused:      reused,
		TopLevelToRetire:    retireInfo,
		RelocationMoves:     len(relocations),
		RelocationArtifacts: relocArtifacts,
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("marshal profile.adopt plan: %w", err)
	}
	planHash, err := computeTaxonomyPlanHash(plan)
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("hash profile.adopt plan: %w", err)
	}
	diffJSON, err := json.Marshal(diff)
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("marshal profile.adopt diff: %w", err)
	}

	actorID := strings.TrimSpace(p.AgentID)
	if actorID == "" {
		actorID = "unassigned"
	}
	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("begin profile.adopt propose tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	changeID, err := insertTaxonomyChange(ctx, tx, taxonomyChange{
		ProjectID:         projectID,
		Kind:              taxonomyChangeKindProfileAdopt,
		SourceProfileSlug: sourceProfileSlug,
		TargetProfileSlug: targetProfile.Slug,
		PlanJSON:          planJSON,
		DiffJSON:          diffJSON,
		PlanHash:          planHash,
		CreatedBy:         actorID,
	})
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("record profile.adopt change: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, payload)
		VALUES ($1::uuid, 'taxonomy.profile_adopt_proposed', jsonb_build_object(
			'change_id', $2::text, 'plan_hash', $3::text, 'kind', $4::text,
			'source_profile_slug', $5::text, 'target_profile_slug', $6::text,
			'proposed_by', $7::text
		))
	`, projectID, changeID, planHash, taxonomyChangeKindProfileAdopt,
		sourceProfileSlug, targetProfile.Slug, actorID); err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("record profile.adopt event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("commit profile.adopt propose: %w", err)
	}

	return nil, taxonomyChangeProposeOutput{
		Status:      "proposed",
		ProjectSlug: projectSlug,
		ChangeID:    changeID,
		PlanHash:    planHash,
		Diff:        &diff,
		Message: fmt.Sprintf(
			"Recorded change-set %s (profile.adopt %s -> %s): %d top-level(s) to create, %d reused, %d to retire, %d relocation move(s). NOT applied — owner approves, then pindoc.taxonomy.change.apply.",
			changeID, sourceProfileSlug, targetProfile.Slug,
			len(toCreate), len(reused), len(toRetire), len(relocations)),
	}, nil
}

// loadCurrentTopLevels returns the project's current top-level areas
// keyed by slug.
func loadCurrentTopLevels(ctx context.Context, deps Deps, projectID string) (map[string]currentTopLevel, error) {
	rows, err := deps.DB.Query(ctx, `
		SELECT id::text, slug, lifecycle FROM areas
		 WHERE project_id = $1::uuid AND parent_id IS NULL
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("load current top-level areas: %w", err)
	}
	defer rows.Close()
	out := map[string]currentTopLevel{}
	for rows.Next() {
		var id, slug, lifecycle string
		if err := rows.Scan(&id, &slug, &lifecycle); err != nil {
			return nil, fmt.Errorf("scan top-level area: %w", err)
		}
		out[slug] = currentTopLevel{id: id, lifecycle: lifecycle}
	}
	return out, rows.Err()
}

// areaSubtreeArtifactCount counts the artifacts an area's whole subtree
// (itself + every descendant) holds.
func areaSubtreeArtifactCount(ctx context.Context, deps Deps, areaID string) (int, error) {
	var count int
	if err := deps.DB.QueryRow(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM areas WHERE id = $1::uuid
			UNION ALL
			SELECT a.id FROM areas a JOIN subtree s ON a.parent_id = s.id
		)
		SELECT count(*) FROM artifacts WHERE area_id IN (SELECT id FROM subtree)
	`, areaID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count subtree artifacts for %s: %w", areaID, err)
	}
	return count, nil
}

// normalizeRelocationMoves converts the agent-facing relocation map into
// the internal move shape, dropping empty entries.
func normalizeRelocationMoves(in []relocationMoveInput) []artifactRelocationMove {
	out := []artifactRelocationMove{}
	for _, m := range in {
		slug := strings.ToLower(strings.TrimSpace(m.ToAreaSlug))
		if slug == "" {
			continue
		}
		ids := []string{}
		for _, a := range m.ArtifactSlugs {
			if a = strings.TrimSpace(a); a != "" {
				ids = append(ids, a)
			}
		}
		if len(ids) == 0 {
			continue
		}
		sort.Strings(ids)
		out = append(out, artifactRelocationMove{ToAreaSlug: slug, ArtifactIDs: ids})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ToAreaSlug < out[j].ToAreaSlug })
	return out
}

func topLevelSpecSlugs(specs []profileAdoptTopLevelSpec) []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.Slug)
	}
	return out
}
