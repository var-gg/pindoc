package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

// Decision taxonomy-change-operation T11: archiveEmptyAreas is the
// area-retirement primitive. It transitions empty areas to
// lifecycle='archived' — a row update, never a hard delete, so the
// areas.parent_id ON DELETE CASCADE can never fire and wipe a subtree.
//
// An area is archived only when its whole subtree (itself + every
// descendant) holds zero artifacts and it has no active/retiring child
// area. Archival is effectively post-order: a parent stays blocked until
// its children archive, so the multi-pass loop drains leaves first.
//
// It returns the area IDs actually archived and the IDs left blocked
// (still holding artifacts, or with a non-archivable child area).
func archiveEmptyAreas(ctx context.Context, tx pgx.Tx, projectID string, areaIDs []string, changeID string) (archived, blocked []string, err error) {
	remaining := map[string]bool{}
	for _, id := range areaIDs {
		if id != "" {
			remaining[id] = true
		}
	}
	for {
		progressed := false
		for id := range remaining {
			empty, emptyErr := areaSubtreeEmpty(ctx, tx, id)
			if emptyErr != nil {
				return archived, blocked, emptyErr
			}
			if !empty {
				continue
			}
			ct, exErr := tx.Exec(ctx, `
				UPDATE areas
				   SET lifecycle = 'archived',
				       archived_at = now(),
				       retired_by_change_id = COALESCE(retired_by_change_id, NULLIF($2, '')::uuid)
				 WHERE id = $1::uuid
				   AND project_id = $3::uuid
				   AND lifecycle <> 'archived'
			`, id, changeID, projectID)
			if exErr != nil {
				return archived, blocked, fmt.Errorf("archive area %s: %w", id, exErr)
			}
			delete(remaining, id)
			if ct.RowsAffected() == 0 {
				// Already archived, or not in this project — drop it
				// from the working set without reporting it.
				continue
			}
			archived = append(archived, id)
			progressed = true
		}
		if !progressed {
			break
		}
	}
	for id := range remaining {
		blocked = append(blocked, id)
	}
	sort.Strings(archived)
	sort.Strings(blocked)
	return archived, blocked, nil
}

// areaSubtreeEmpty reports whether an area can be archived: its whole
// subtree (itself + every descendant) holds no artifacts, and it has no
// active or retiring child area. An archived child does not block the
// parent — it has already been retired.
func areaSubtreeEmpty(ctx context.Context, tx pgx.Tx, areaID string) (bool, error) {
	var subtreeArtifacts int
	if err := tx.QueryRow(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM areas WHERE id = $1::uuid
			UNION ALL
			SELECT a.id FROM areas a JOIN subtree s ON a.parent_id = s.id
		)
		SELECT count(*) FROM artifacts WHERE area_id IN (SELECT id FROM subtree)
	`, areaID).Scan(&subtreeArtifacts); err != nil {
		return false, fmt.Errorf("count subtree artifacts for %s: %w", areaID, err)
	}
	if subtreeArtifacts > 0 {
		return false, nil
	}
	var liveChildren int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM areas
		 WHERE parent_id = $1::uuid AND lifecycle IN ('active', 'retiring')
	`, areaID).Scan(&liveChildren); err != nil {
		return false, fmt.Errorf("count live children for %s: %w", areaID, err)
	}
	return liveChildren == 0, nil
}

// areaRetirePlan is the plan_json shape for a kind=area.retire_empty
// change-set. AreaIDs/AreaSlugs are sorted so plan_hash is deterministic.
type areaRetirePlan struct {
	Kind      string   `json:"kind"`
	ProjectID string   `json:"project_id"`
	AreaIDs   []string `json:"area_ids"`
	AreaSlugs []string `json:"area_slugs"`
}

// proposeAreaRetireEmpty records a kind=area.retire_empty change-set. It
// resolves the requested area slugs to ids and persists the plan; it does
// NOT archive anything — apply does, re-checking emptiness then.
func proposeAreaRetireEmpty(ctx context.Context, deps Deps, p *auth.Principal, projectID, projectSlug string, in taxonomyChangeProposeInput) (*sdk.CallToolResult, taxonomyChangeProposeOutput, error) {
	slugs := []string{}
	seen := map[string]bool{}
	for _, s := range in.AreaSlugs {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && !seen[s] {
			seen[s] = true
			slugs = append(slugs, s)
		}
	}
	if len(slugs) == 0 {
		return nil, taxonomyChangeNotReady("AREA_SLUGS_REQUIRED",
			"kind=area.retire_empty requires a non-empty area_slugs list."), nil
	}
	sort.Strings(slugs)

	rows, err := deps.DB.Query(ctx, `
		SELECT id::text, slug FROM areas
		 WHERE project_id = $1::uuid AND slug = ANY($2)
	`, projectID, slugs)
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("resolve retire areas: %w", err)
	}
	resolved := map[string]string{}
	for rows.Next() {
		var id, slug string
		if err := rows.Scan(&id, &slug); err != nil {
			rows.Close()
			return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("scan retire area: %w", err)
		}
		resolved[slug] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("retire area rows: %w", err)
	}

	areaIDs := make([]string, 0, len(slugs))
	for _, s := range slugs {
		id, ok := resolved[s]
		if !ok {
			return nil, taxonomyChangeNotReady("AREA_NOT_FOUND",
				fmt.Sprintf("area %q is not an area in this project.", s)), nil
		}
		areaIDs = append(areaIDs, id)
	}
	sort.Strings(areaIDs)

	plan := areaRetirePlan{
		Kind:      taxonomyChangeKindAreaRetire,
		ProjectID: projectID,
		AreaIDs:   areaIDs,
		AreaSlugs: slugs,
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("marshal area.retire_empty plan: %w", err)
	}
	planHash, err := computeTaxonomyPlanHash(plan)
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("hash area.retire_empty plan: %w", err)
	}
	diffJSON, err := json.Marshal(map[string]any{"to_retire": slugs})
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("marshal area.retire_empty diff: %w", err)
	}

	actorID := strings.TrimSpace(p.AgentID)
	if actorID == "" {
		actorID = "unassigned"
	}
	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("begin retire propose tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	changeID, err := insertTaxonomyChange(ctx, tx, taxonomyChange{
		ProjectID: projectID,
		Kind:      taxonomyChangeKindAreaRetire,
		PlanJSON:  planJSON,
		DiffJSON:  diffJSON,
		PlanHash:  planHash,
		CreatedBy: actorID,
	})
	if err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("record area.retire_empty change: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, payload)
		VALUES ($1::uuid, 'taxonomy.area_retire_proposed', jsonb_build_object(
			'change_id', $2::text, 'plan_hash', $3::text, 'kind', $4::text,
			'area_slugs', $5::jsonb, 'proposed_by', $6::text
		))
	`, projectID, changeID, planHash, taxonomyChangeKindAreaRetire, string(diffSlugsJSON(slugs)), actorID); err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("record area.retire_empty event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, taxonomyChangeProposeOutput{}, fmt.Errorf("commit area.retire_empty propose: %w", err)
	}
	return nil, taxonomyChangeProposeOutput{
		Status:      "proposed",
		ProjectSlug: projectSlug,
		ChangeID:    changeID,
		PlanHash:    planHash,
		AreaSlugs:   slugs,
		Message: fmt.Sprintf(
			"Recorded change-set %s (area.retire_empty, %d area(s)). NOT applied — owner approves, then pindoc.taxonomy.change.apply archives the empty ones.",
			changeID, len(slugs)),
	}, nil
}

// applyAreaRetireEmpty executes a kind=area.retire_empty change-set: it
// archives every empty area in the plan and reports the ones left blocked
// (still holding artifacts or a live child) — all in one transaction.
func applyAreaRetireEmpty(ctx context.Context, deps Deps, p *auth.Principal, projectID string, change taxonomyChange) (*sdk.CallToolResult, taxonomyChangeApplyOutput, error) {
	var plan areaRetirePlan
	if err := json.Unmarshal(change.PlanJSON, &plan); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("parse area.retire_empty plan: %w", err)
	}

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("begin retire apply tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	archived, blocked, err := archiveEmptyAreas(ctx, tx, projectID, plan.AreaIDs, change.ID)
	if err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("archive empty areas: %w", err)
	}
	actor := taxonomyChangeActor(p)
	if err := markTaxonomyChangeApplied(ctx, tx, change.ID, actor); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("mark applied: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, payload)
		VALUES ($1::uuid, 'taxonomy.change_applied', jsonb_build_object(
			'change_id', $2::text, 'plan_hash', $3::text, 'kind', $4::text,
			'applied_by', $5::text, 'archived_count', $6::int, 'blocked_count', $7::int
		))
	`, projectID, change.ID, change.PlanHash, change.Kind, actor, len(archived), len(blocked)); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("record change_applied event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, taxonomyChangeApplyOutput{}, fmt.Errorf("commit retire apply: %w", err)
	}
	return nil, taxonomyChangeApplyOutput{
		Status:        "applied",
		ChangeID:      change.ID,
		ChangeStatus:  taxonomyChangeStatusApplied,
		Kind:          change.Kind,
		ArchivedCount: len(archived),
		BlockedCount:  len(blocked),
		Message: fmt.Sprintf(
			"Applied change-set %s — archived %d area(s); %d left blocked (still hold artifacts or a live child).",
			change.ID, len(archived), len(blocked)),
	}, nil
}

// diffSlugsJSON marshals a slug list for an event payload's jsonb param.
func diffSlugsJSON(slugs []string) []byte {
	b, err := json.Marshal(slugs)
	if err != nil {
		return []byte("[]")
	}
	return b
}
