package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestArtifactProposeAcceptanceLabelSelectorIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run acceptance label selector DB integration")
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
	projectSlug := fmt.Sprintf("label-selector-%d", suffix)
	areaSlug := fmt.Sprintf("mcp-%d", suffix)
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	areaID := insertContextReceiptArea(t, ctx, pool, projectID, areaSlug)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	taskSlug := fmt.Sprintf("label-task-%d", suffix)
	taskID := insertContextReceiptTask(t, ctx, pool, projectID, areaID, taskSlug)
	body := "## Purpose\n\nLabel selector integration.\n\n## Scope\n\nMCP acceptance transition.\n\n## TODO\n\n- [ ] DevTI 검증\n- [ ] QA 통과\n"
	if _, err := pool.Exec(ctx, `UPDATE artifacts SET body_markdown = $2 WHERE id = $1::uuid`, taskID, body); err != nil {
		t.Fatalf("seed task body: %v", err)
	}

	readBefore := callArtifactReadForEvidenceTest(t, ctx, pool, projectSlug, taskSlug)
	if len(readBefore.UnresolvedAcceptanceLabels) != 2 {
		t.Fatalf("artifact.read unresolved labels = %+v, want 2", readBefore.UnresolvedAcceptanceLabels)
	}

	call := newArtifactProposeTestCaller(t, ctx, pool, nil)
	out := call(ctx, map[string]any{
		"project_slug":     projectSlug,
		"area_slug":        areaSlug,
		"type":             "Task",
		"title":            taskSlug,
		"update_of":        taskSlug,
		"expected_version": 1,
		"shape":            "acceptance_transition",
		"commit_msg":       "mark QA by label",
		"body_markdown":    body,
		"author_id":        "codex-test",
		"acceptance_transition": map[string]any{
			"checkbox_label_match": "QA",
			"new_state":            "[x]",
		},
	})
	if out.Status != "accepted" || out.RevisionNumber != 2 || len(out.AcceptanceLabelMatches) != 1 || out.AcceptanceLabelMatches[0].Index != 1 {
		t.Fatalf("label transition output = status=%q code=%q checklist=%v rev=%d matches=%+v", out.Status, out.ErrorCode, out.Checklist, out.RevisionNumber, out.AcceptanceLabelMatches)
	}
	var gotBody string
	if err := pool.QueryRow(ctx, `SELECT body_markdown FROM artifacts WHERE id = $1::uuid`, taskID).Scan(&gotBody); err != nil {
		t.Fatalf("read task body: %v", err)
	}
	if !strings.Contains(gotBody, "- [x] QA 통과") || !strings.Contains(gotBody, "- [ ] DevTI 검증") {
		t.Fatalf("body after label transition:\n%s", gotBody)
	}
}
