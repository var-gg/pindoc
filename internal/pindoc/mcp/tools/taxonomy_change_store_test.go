package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// TestComputeTaxonomyPlanHashDeterministic locks the plan_hash contract:
// identical plans hash equal, any change diverges, and the digest is a
// 64-char sha256 hex string.
func TestComputeTaxonomyPlanHashDeterministic(t *testing.T) {
	planA := map[string]any{
		"kind": "profile.adopt", "project_id": "p1",
		"target_profile": "game-narrative",
		"artifact_ids":   []string{"a1", "a2", "a3"},
	}
	planB := map[string]any{
		"kind": "profile.adopt", "project_id": "p1",
		"target_profile": "game-narrative",
		"artifact_ids":   []string{"a1", "a2", "a3"},
	}
	planC := map[string]any{
		"kind": "profile.adopt", "project_id": "p1",
		"target_profile": "game-narrative",
		"artifact_ids":   []string{"a1", "a2", "a9"},
	}
	hA, err := computeTaxonomyPlanHash(planA)
	if err != nil {
		t.Fatalf("hash planA: %v", err)
	}
	hB, err := computeTaxonomyPlanHash(planB)
	if err != nil {
		t.Fatalf("hash planB: %v", err)
	}
	hC, err := computeTaxonomyPlanHash(planC)
	if err != nil {
		t.Fatalf("hash planC: %v", err)
	}
	if hA != hB {
		t.Fatalf("identical plans must hash equal: %s vs %s", hA, hB)
	}
	if hA == hC {
		t.Fatal("different plans must not hash equal")
	}
	if len(hA) != 64 {
		t.Fatalf("sha256 hex length = %d, want 64", len(hA))
	}
}

// TestTaxonomyChangeEventPayloadCarriesAuditKeys covers T7 acceptance: the
// event payload always carries change_id / plan_hash / kind alongside any
// kind-specific extras.
func TestTaxonomyChangeEventPayloadCarriesAuditKeys(t *testing.T) {
	payload := taxonomyChangeEventPayload("chg-1", "hash-1", taxonomyChangeKindTopLevelAdd, map[string]any{
		"candidate_slug": "combat",
	})
	if payload["change_id"] != "chg-1" || payload["plan_hash"] != "hash-1" ||
		payload["kind"] != taxonomyChangeKindTopLevelAdd || payload["candidate_slug"] != "combat" {
		t.Fatalf("event payload missing audit keys: %+v", payload)
	}
}

// TestTaxonomyChangeStoreRoundtripIntegration exercises insert -> get ->
// status transitions and the proposed->approved->applied status guard.
func TestTaxonomyChangeStoreRoundtripIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run taxonomy_changes store integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	projectID := insertContextReceiptProject(t, ctx, pool, fmt.Sprintf("taxchg-store-%d", time.Now().UnixNano()))
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	planHash, err := computeTaxonomyPlanHash(map[string]any{"kind": "top_level.add", "slug": "combat"})
	if err != nil {
		t.Fatalf("plan hash: %v", err)
	}
	id, err := insertTaxonomyChange(ctx, pool, taxonomyChange{
		ProjectID: projectID,
		Kind:      taxonomyChangeKindTopLevelAdd,
		PlanJSON:  json.RawMessage(`{"slug":"combat","fileable":true}`),
		DiffJSON:  json.RawMessage(`{"to_create":["combat"]}`),
		PlanHash:  planHash,
		CreatedBy: "agent:claude-code",
	})
	if err != nil {
		t.Fatalf("insert taxonomy change: %v", err)
	}

	got, err := getTaxonomyChange(ctx, pool, id)
	if err != nil {
		t.Fatalf("get taxonomy change: %v", err)
	}
	if got.Status != taxonomyChangeStatusProposed || got.Kind != taxonomyChangeKindTopLevelAdd ||
		got.PlanHash != planHash || got.CreatedBy != "agent:claude-code" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}

	// apply before approve must fail the status guard.
	if err := markTaxonomyChangeApplied(ctx, pool, id, "agent:claude-code"); err == nil {
		t.Fatal("apply on a proposed change should fail the status guard")
	}
	if err := markTaxonomyChangeApproved(ctx, pool, id, "user:owner"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := markTaxonomyChangeApplied(ctx, pool, id, "agent:claude-code"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	final, err := getTaxonomyChange(ctx, pool, id)
	if err != nil {
		t.Fatalf("get final: %v", err)
	}
	if final.Status != taxonomyChangeStatusApplied || final.ApprovedBy != "user:owner" ||
		final.AppliedBy != "agent:claude-code" {
		t.Fatalf("final state mismatch: %+v", final)
	}
}
