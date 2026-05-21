package tools

import (
	"context"
	"encoding/json"
	"errors"
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
	// StructuralConflicts blocks apply; SemanticConflictCandidates is
	// advisory (Decision taxonomy-change-operation T15).
	StructuralConflicts        []string `json:"structural_conflicts,omitempty"`
	SemanticConflictCandidates []string `json:"semantic_conflict_candidates,omitempty"`
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
	structuralConflicts, semanticCandidates, err := detectProfileAdoptConflicts(ctx, deps, projectID, plan)
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, err
	}
	diff := profileAdoptDiff{
		SourceProfileSlug:          sourceProfileSlug,
		TargetProfileSlug:          targetProfile.Slug,
		TopLevelToCreate:           topLevelSpecSlugs(toCreate),
		TopLevelReused:             reused,
		TopLevelToRetire:           retireInfo,
		RelocationMoves:            len(relocations),
		RelocationArtifacts:        relocArtifacts,
		StructuralConflicts:        structuralConflicts,
		SemanticConflictCandidates: semanticCandidates,
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

// applyProfileAdopt executes an approved profile.adopt change-set in one
// transaction (Decision taxonomy-change-operation T14). Order matters:
// legacy top-levels retire FIRST so the active top-level cap is not
// exhausted when the new ones are created. The project profile pin is
// updated LAST, recording the adoption as structurally complete.
func applyProfileAdopt(ctx context.Context, deps Deps, p *auth.Principal, projectID string, change taxonomyChange) (*sdk.CallToolResult, taxonomyChangeApplyOutput, error) {
	var plan profileAdoptPlan
	if err := json.Unmarshal(change.PlanJSON, &plan); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("parse profile.adopt plan: %w", err)
	}

	// Decision taxonomy-change-operation T15: re-run the structural
	// conflict detector at apply. A conflict that surfaced since approval
	// (a colliding area appeared, the cap filled) makes the plan unsafe —
	// refuse before mutating anything.
	structuralConflicts, _, err := detectProfileAdoptConflicts(ctx, deps, projectID, plan)
	if err != nil {
		return nil, taxonomyChangeApplyOutput{}, err
	}
	if len(structuralConflicts) > 0 {
		return nil, taxonomyChangeApplyStale(deps, ctx, change, "structural conflict — "+structuralConflicts[0]), nil
	}

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("begin profile.adopt apply tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Project-scoped advisory lock: a profile.adopt apply must not
	// interleave with another taxonomy change-set on the same project.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "taxonomy:"+projectID); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("acquire project lock: %w", err)
	}

	// Step 1: retire the legacy top-levels first — freeing active
	// top-level budget before the new ones are created.
	retiredAreaIDs := []string{}
	if len(plan.TopLevelToRetire) > 0 {
		rows, err := tx.Query(ctx, `
			UPDATE areas
			   SET lifecycle = 'retiring',
			       retired_by_change_id = $2::uuid
			 WHERE project_id = $1::uuid
			   AND parent_id IS NULL
			   AND lifecycle = 'active'
			   AND slug = ANY($3)
			RETURNING id::text
		`, projectID, change.ID, plan.TopLevelToRetire)
		if err != nil {
			return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("retire legacy top-levels: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("scan retired area: %w", err)
			}
			retiredAreaIDs = append(retiredAreaIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("retired area rows: %w", err)
		}
	}

	// Step 2: create the missing target top-levels. A slug collision or
	// cap overflow here means the world drifted since approval.
	createdCount := 0
	for _, spec := range plan.TopLevelToCreate {
		if _, createErr := projects.CreateTopLevelArea(ctx, tx, projectID, projects.TopLevelAreaSpec{
			Slug:           spec.Slug,
			Name:           spec.Name,
			Description:    spec.Description,
			IsCrossCutting: spec.IsCrossCutting,
			Fileable:       spec.Fileable,
			MaxDepth:       spec.MaxDepth,
		}, plan.TargetProfileSlug, change.ID); createErr != nil {
			if errors.Is(createErr, projects.ErrTopLevelAreaSlugTaken) || errors.Is(createErr, projects.ErrTopLevelAreaCapExceeded) {
				return nil, taxonomyChangeApplyStale(deps, ctx, change, createErr.Error()), nil
			}
			return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("create target top-level %q: %w", spec.Slug, createErr)
		}
		createdCount++
	}

	// Step 3: apply the approved relocation map. A failed move is drift.
	relocated, relErr := executeArtifactRelocation(ctx, tx, p, projectID, change.ID,
		"profile.adopt: "+plan.SourceProfileSlug+" -> "+plan.TargetProfileSlug, plan.Relocations)
	if relErr != nil {
		return nil, taxonomyChangeApplyStale(deps, ctx, change, relErr.Error()), nil
	}

	// Step 4: archive any retiring legacy top-level the relocations
	// emptied. Non-empty ones stay retiring as legacy inventory.
	archived, _, archErr := archiveEmptyAreas(ctx, tx, projectID, retiredAreaIDs, change.ID)
	if archErr != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("archive emptied legacy areas: %w", archErr)
	}

	// Step 5: update the project profile pin — the last structural step.
	if _, err := tx.Exec(ctx, `
		UPDATE projects SET taxonomy_profile_slug = $2 WHERE id = $1::uuid
	`, projectID, plan.TargetProfileSlug); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("update project profile pin: %w", err)
	}

	actor := taxonomyChangeActor(p)
	if err := markTaxonomyChangeApplied(ctx, tx, change.ID, actor); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("mark applied: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, payload)
		VALUES ($1::uuid, 'taxonomy.change_applied', jsonb_build_object(
			'change_id', $2::text, 'plan_hash', $3::text, 'kind', $4::text,
			'applied_by', $5::text,
			'source_profile_slug', $6::text, 'target_profile_slug', $7::text,
			'top_level_created', $8::int, 'top_level_retired', $9::int,
			'artifacts_relocated', $10::int, 'areas_archived', $11::int
		))
	`, projectID, change.ID, change.PlanHash, change.Kind, actor,
		plan.SourceProfileSlug, plan.TargetProfileSlug,
		createdCount, len(retiredAreaIDs), relocated, len(archived)); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("record change_applied event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("commit profile.adopt apply: %w", err)
	}
	return nil, taxonomyChangeApplyOutput{
		Status:        "applied",
		ChangeID:      change.ID,
		ChangeStatus:  taxonomyChangeStatusApplied,
		Kind:          change.Kind,
		ArchivedCount: len(archived),
		Message: fmt.Sprintf(
			"Applied change-set %s (profile.adopt %s -> %s): %d top-level created, %d retired, %d artifact(s) relocated, %d legacy area(s) archived.",
			change.ID, plan.SourceProfileSlug, plan.TargetProfileSlug,
			createdCount, len(retiredAreaIDs), relocated, len(archived)),
	}, nil
}
