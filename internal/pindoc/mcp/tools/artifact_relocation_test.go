package tools

import (
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// TestExecuteArtifactRelocationIntegration covers the T12 relocation
// executor: a fixed artifact set moves to a target area, each move emits
// an artifact.area_changed event tagged with taxonomy_change_id, and a
// move to an unknown target errors without committing.
func TestExecuteArtifactRelocationIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	srcID := insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "experience", "reloc-src")
	insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "experience", "reloc-dst")
	insertMCPVisibilityArtifact(t, ctx, pool, fixture.projectID, srcID, "reloc-art-1", projects.VisibilityOrg, fixture.ownerUserID, "ko")
	insertMCPVisibilityArtifact(t, ctx, pool, fixture.projectID, srcID, "reloc-art-2", projects.VisibilityOrg, fixture.ownerUserID, "ko")

	const changeID = "a1a2a3a4-b1b2-c1c2-d1d2-e1e2e3e4e5e6"

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	moved, err := executeArtifactRelocation(ctx, tx, owner, fixture.projectID, changeID, "profile.adopt relocation", []artifactRelocationMove{
		{ToAreaSlug: "reloc-dst", ArtifactIDs: []string{"reloc-art-1", "reloc-art-2"}},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("executeArtifactRelocation: %v", err)
	}
	if moved != 2 {
		_ = tx.Rollback(ctx)
		t.Fatalf("moved = %d, want 2", moved)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit relocation: %v", err)
	}

	assertArtifactAreaSlug(t, ctx, pool, fixture.projectID, "reloc-art-1", "reloc-dst")
	assertArtifactAreaSlug(t, ctx, pool, fixture.projectID, "reloc-art-2", "reloc-dst")

	var taggedEvents int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM events
		 WHERE project_id = $1::uuid
		   AND kind = 'artifact.area_changed'
		   AND payload->>'taxonomy_change_id' = $2
	`, fixture.projectID, changeID).Scan(&taggedEvents); err != nil {
		t.Fatalf("count tagged events: %v", err)
	}
	if taggedEvents != 2 {
		t.Fatalf("artifact.area_changed events tagged with taxonomy_change_id = %d, want 2", taggedEvents)
	}

	// A move to an unknown target errors — the caller rolls the tx back,
	// so a partial relocation never commits.
	badTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin bad tx: %v", err)
	}
	if _, err := executeArtifactRelocation(ctx, badTx, owner, fixture.projectID, changeID, "bad move", []artifactRelocationMove{
		{ToAreaSlug: "does-not-exist-area", ArtifactIDs: []string{"reloc-art-1"}},
	}); err == nil {
		_ = badTx.Rollback(ctx)
		t.Fatal("executeArtifactRelocation with an unknown target should error")
	}
	_ = badTx.Rollback(ctx)
}
