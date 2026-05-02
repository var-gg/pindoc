package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestContextForTaskReceiptAreaSlugs(t *testing.T) {
	got := contextForTaskReceiptAreaSlugs(
		contextForTaskInput{Areas: []string{"mcp", "", "mcp"}},
		contextForTaskOutput{
			SuggestedAreas: []AreaSuggestion{{AreaSlug: "ui"}, {AreaSlug: "mcp"}},
			Landings: []ContextLanding{
				{AreaSlug: "data"},
				{AreaSlug: "ui"},
			},
		},
	)
	want := []string{"mcp", "ui", "data"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("area slugs = %v, want %v", got, want)
	}
}

func TestApplyTopMatchSimilarityHint(t *testing.T) {
	near := contextForTaskOutput{Landings: []ContextLanding{{Distance: 0.35}}}
	applyTopMatchSimilarityHint(&near)
	if near.TopMatchSimilarityHint != "high" || near.DecisionHint != "update_recommended" {
		t.Fatalf("near hints = (%q, %q)", near.TopMatchSimilarityHint, near.DecisionHint)
	}

	far := contextForTaskOutput{Landings: []ContextLanding{{Distance: 0.45}}}
	applyTopMatchSimilarityHint(&far)
	if far.TopMatchSimilarityHint != "" || far.DecisionHint != "" {
		t.Fatalf("far hints = (%q, %q), want empty", far.TopMatchSimilarityHint, far.DecisionHint)
	}
}

func TestContextForTaskTaskReceiptSnapshotsIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run context_for_task receipt DB integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	suffix := time.Now().UnixNano()
	projectSlug := fmt.Sprintf("ctx-receipt-%d", suffix)
	activeAreaSlug := fmt.Sprintf("active-%d", suffix)
	emptyAreaSlug := fmt.Sprintf("empty-%d", suffix)
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	activeAreaID := insertContextReceiptArea(t, ctx, pool, projectID, activeAreaSlug)
	insertContextReceiptArea(t, ctx, pool, projectID, emptyAreaSlug)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	activeTaskID := insertContextReceiptTask(t, ctx, pool, projectID, activeAreaID, "active-task")
	deps := Deps{DB: pool}
	readScope := &mcpReadProjectScope{
		ProjectScope: &auth.ProjectScope{
			ProjectID:     projectID,
			ProjectSlug:   projectSlug,
			ProjectLocale: "en",
			Role:          auth.RoleOwner,
		},
		TrustedAll: true,
		Member:     true,
	}

	activeSnapshots := contextForTaskReceiptSnapshots(ctx, deps, readScope, "Task",
		contextForTaskInput{Areas: []string{activeAreaSlug}},
		contextForTaskOutput{},
	)
	if len(activeSnapshots) != 1 {
		t.Fatalf("active area snapshots = %v, want one active task snapshot", activeSnapshots)
	}
	if activeSnapshots[0].ArtifactID != activeTaskID || activeSnapshots[0].RevisionNumber != 1 {
		t.Fatalf("active snapshot = %+v, want id=%s rev=1", activeSnapshots[0], activeTaskID)
	}
	activeTasks, err := activeTasksInArea(ctx, deps, projectID, activeAreaID, "")
	if err != nil {
		t.Fatalf("activeTasksInArea: %v", err)
	}
	if len(activeTasks) != 1 {
		t.Fatalf("active tasks = %v, want one", activeTasks)
	}
	if !receiptSnapshotsContainAny(activeSnapshots, activeTasks) {
		t.Fatalf("context_for_task snapshots should satisfy TASK_ACTIVE_CONTEXT_REQUIRED gate")
	}

	searchSnapshots := headSnapshotsForArtifacts(ctx, deps, []string{activeTaskID})
	if !receiptSnapshotsContainAny(searchSnapshots, activeTasks) {
		t.Fatalf("artifact_search-style snapshots should still satisfy TASK_ACTIVE_CONTEXT_REQUIRED gate")
	}

	emptySnapshots := contextForTaskReceiptSnapshots(ctx, deps, readScope, "Task",
		contextForTaskInput{Areas: []string{emptyAreaSlug}},
		contextForTaskOutput{},
	)
	if len(emptySnapshots) != 0 {
		t.Fatalf("empty area snapshots = %v, want none", emptySnapshots)
	}

	nonTaskSnapshots := contextForTaskReceiptSnapshots(ctx, deps, readScope, "Decision",
		contextForTaskInput{Areas: []string{activeAreaSlug}},
		contextForTaskOutput{},
	)
	if len(nonTaskSnapshots) != 0 {
		t.Fatalf("non-Task snapshots = %v, want no active Task snapshot overhead", nonTaskSnapshots)
	}
}

func insertContextReceiptProject(t *testing.T, ctx context.Context, pool *db.Pool, slug string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (owner_id, organization_id, slug, name, primary_language)
		VALUES (
			'default',
			(SELECT id FROM organizations WHERE slug = 'default' LIMIT 1),
			$1, $2, 'en'
		)
		RETURNING id::text
	`, slug, "test "+slug).Scan(&id); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return id
}

func insertContextReceiptArea(t *testing.T, ctx context.Context, pool *db.Pool, projectID, slug string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO areas (project_id, slug, name)
		VALUES ($1::uuid, $2, $2)
		RETURNING id::text
	`, projectID, slug).Scan(&id); err != nil {
		t.Fatalf("insert area: %v", err)
	}
	return id
}

func insertContextReceiptTask(t *testing.T, ctx context.Context, pool *db.Pool, projectID, areaID, slug string) string {
	t.Helper()
	body := "## TODO\n\n- [ ] read acceptance before creating follow-up\n"
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (
			project_id, area_id, slug, type, title,
			body_markdown, body_locale, author_id, completeness, task_meta
		)
		VALUES ($1::uuid, $2::uuid, $3, 'Task', $3, $4, 'en', 'tester', 'partial', '{"status":"open"}'::jsonb)
		RETURNING id::text
	`, projectID, areaID, slug, body).Scan(&id); err != nil {
		t.Fatalf("insert task artifact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash, tags,
			completeness, author_kind, author_id, commit_msg, revision_shape
		)
		VALUES (
			$1::uuid, 1, $2, $3,
			encode(sha256(convert_to($3, 'UTF8')), 'hex'),
			'{}'::text[], 'partial', 'agent', 'tester', 'seed active task', 'body_patch'
		)
	`, id, slug, body); err != nil {
		t.Fatalf("insert task revision: %v", err)
	}
	return id
}
