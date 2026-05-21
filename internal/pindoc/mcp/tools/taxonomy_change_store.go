package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Decision taxonomy-change-operation T7: taxonomy_changes is the operation
// record shared by taxonomy.change.propose / diff / approve / apply. This
// file is the store + audit contract — the change kinds themselves land in
// later tasks (T10 top_level.add, T11 area.retire_empty, T13/T14
// profile.adopt).

// taxonomy change-set kinds.
const (
	taxonomyChangeKindTopLevelAdd  = "top_level.add"
	taxonomyChangeKindProfileAdopt = "profile.adopt"
	taxonomyChangeKindAreaRetire   = "area.retire_empty"
)

// taxonomy change-set statuses. proposed -> approved -> applied is the
// success path; rejected and stale are terminal.
const (
	taxonomyChangeStatusProposed = "proposed"
	taxonomyChangeStatusApproved = "approved"
	taxonomyChangeStatusApplied  = "applied"
	taxonomyChangeStatusRejected = "rejected"
	taxonomyChangeStatusStale    = "stale"
)

// taxonomyChangeQuerier is satisfied by both *db.Pool and pgx.Tx, so the
// store works on a pool for read-only calls and inside a transaction for
// the apply path.
type taxonomyChangeQuerier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// taxonomyChange is one taxonomy_changes row.
type taxonomyChange struct {
	ID                string
	ProjectID         string
	Kind              string
	Status            string
	SourceProfileSlug string
	TargetProfileSlug string
	PlanJSON          json.RawMessage
	DiffJSON          json.RawMessage
	PlanHash          string
	CreatedBy         string
	ApprovedBy        string
	AppliedBy         string
}

// computeTaxonomyPlanHash returns a deterministic sha256 hex digest over
// the plan value. The plan is the source of truth for what apply will do,
// so callers MUST pre-sort every slice inside it (artifact IDs, area
// specs) — Go's json.Marshal sorts map keys but preserves slice order, so
// an unsorted slice would make the hash unstable across the propose and
// apply-time re-derivation.
func computeTaxonomyPlanHash(plan any) (string, error) {
	encoded, err := json.Marshal(plan)
	if err != nil {
		return "", fmt.Errorf("marshal taxonomy plan: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// taxonomyChangeEventPayload builds the audit payload every taxonomy.*
// event carries. The event is an audit copy only — apply reads the
// taxonomy_changes row, never the event.
func taxonomyChangeEventPayload(changeID, planHash, kind string, extra map[string]any) map[string]any {
	payload := map[string]any{
		"change_id": changeID,
		"plan_hash": planHash,
		"kind":      kind,
	}
	for k, v := range extra {
		payload[k] = v
	}
	return payload
}

// insertTaxonomyChange records a proposed change-set and returns its id.
func insertTaxonomyChange(ctx context.Context, q taxonomyChangeQuerier, tc taxonomyChange) (string, error) {
	if tc.Status == "" {
		tc.Status = taxonomyChangeStatusProposed
	}
	planJSON := tc.PlanJSON
	if len(planJSON) == 0 {
		planJSON = json.RawMessage("{}")
	}
	diffJSON := tc.DiffJSON
	if len(diffJSON) == 0 {
		diffJSON = json.RawMessage("{}")
	}
	var id string
	err := q.QueryRow(ctx, `
		INSERT INTO taxonomy_changes (
			project_id, kind, status, source_profile_slug, target_profile_slug,
			plan_json, diff_json, plan_hash, created_by
		) VALUES (
			$1::uuid, $2, $3, NULLIF($4, ''), NULLIF($5, ''),
			$6::jsonb, $7::jsonb, $8, $9
		)
		RETURNING id::text
	`, tc.ProjectID, tc.Kind, tc.Status, tc.SourceProfileSlug, tc.TargetProfileSlug,
		string(planJSON), string(diffJSON), tc.PlanHash, tc.CreatedBy).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert taxonomy change: %w", err)
	}
	return id, nil
}

// getTaxonomyChange loads one change-set by id.
func getTaxonomyChange(ctx context.Context, q taxonomyChangeQuerier, id string) (taxonomyChange, error) {
	var tc taxonomyChange
	var src, tgt, approvedBy, appliedBy *string
	var planJSON, diffJSON []byte
	err := q.QueryRow(ctx, `
		SELECT id::text, project_id::text, kind, status,
		       source_profile_slug, target_profile_slug,
		       plan_json, diff_json, plan_hash,
		       created_by, approved_by, applied_by
		  FROM taxonomy_changes
		 WHERE id = $1::uuid
	`, id).Scan(
		&tc.ID, &tc.ProjectID, &tc.Kind, &tc.Status,
		&src, &tgt, &planJSON, &diffJSON, &tc.PlanHash,
		&tc.CreatedBy, &approvedBy, &appliedBy,
	)
	if err != nil {
		return taxonomyChange{}, err
	}
	if src != nil {
		tc.SourceProfileSlug = *src
	}
	if tgt != nil {
		tc.TargetProfileSlug = *tgt
	}
	if approvedBy != nil {
		tc.ApprovedBy = *approvedBy
	}
	if appliedBy != nil {
		tc.AppliedBy = *appliedBy
	}
	tc.PlanJSON = json.RawMessage(planJSON)
	tc.DiffJSON = json.RawMessage(diffJSON)
	return tc, nil
}

// markTaxonomyChangeApproved moves a proposed change to approved. The
// status guard in the WHERE clause makes a wrong-state transition a
// no-op error rather than a silent overwrite.
func markTaxonomyChangeApproved(ctx context.Context, q taxonomyChangeQuerier, id, approvedBy string) error {
	ct, err := q.Exec(ctx, `
		UPDATE taxonomy_changes
		   SET status = 'approved', approved_by = $2, approved_at = now(), updated_at = now()
		 WHERE id = $1::uuid AND status = 'proposed'
	`, id, approvedBy)
	if err != nil {
		return fmt.Errorf("approve taxonomy change: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("approve taxonomy change %s: not in 'proposed' status", id)
	}
	return nil
}

// markTaxonomyChangeApplied moves an approved change to applied.
func markTaxonomyChangeApplied(ctx context.Context, q taxonomyChangeQuerier, id, appliedBy string) error {
	ct, err := q.Exec(ctx, `
		UPDATE taxonomy_changes
		   SET status = 'applied', applied_by = $2, applied_at = now(), updated_at = now()
		 WHERE id = $1::uuid AND status = 'approved'
	`, id, appliedBy)
	if err != nil {
		return fmt.Errorf("apply taxonomy change: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("apply taxonomy change %s: not in 'approved' status", id)
	}
	return nil
}

// setTaxonomyChangeTerminal marks a non-terminal change rejected or stale.
func setTaxonomyChangeTerminal(ctx context.Context, q taxonomyChangeQuerier, id, status string) error {
	if status != taxonomyChangeStatusRejected && status != taxonomyChangeStatusStale {
		return fmt.Errorf("taxonomy change: %q is not a terminal status", status)
	}
	ct, err := q.Exec(ctx, `
		UPDATE taxonomy_changes
		   SET status = $2, updated_at = now()
		 WHERE id = $1::uuid AND status IN ('proposed', 'approved')
	`, id, status)
	if err != nil {
		return fmt.Errorf("set taxonomy change terminal: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("set taxonomy change %s to %s: not in a non-terminal status", id, status)
	}
	return nil
}
