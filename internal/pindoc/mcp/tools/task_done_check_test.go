package tools

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestBuildTaskDoneCheckOutputMatrix(t *testing.T) {
	scope := &auth.ProjectScope{ProjectSlug: "pindoc", ProjectLocale: "en"}
	cases := []struct {
		name           string
		records        []taskDoneCheckRecord
		wantDone       bool
		wantOpen       int
		wantUnresolved int
	}{
		{name: "all clear", wantDone: true},
		{
			name: "open task blocks done",
			records: []taskDoneCheckRecord{{
				Slug:      "open-task",
				Title:     "Open task",
				RawStatus: "open",
				Body:      "- [x] complete",
			}},
			wantDone: false,
			wantOpen: 1,
		},
		{
			name: "missing status blocks done",
			records: []taskDoneCheckRecord{{
				Slug: "missing-status",
				Body: "- [x] complete",
			}},
			wantDone: false,
			wantOpen: 1,
		},
		{
			name: "claimed done with unchecked acceptance blocks done",
			records: []taskDoneCheckRecord{{
				Slug:      "unchecked",
				RawStatus: "claimed_done",
				Body:      "- [ ] QA pass",
			}},
			wantDone:       false,
			wantUnresolved: 1,
		},
		{
			name: "claimed done with partial acceptance blocks done",
			records: []taskDoneCheckRecord{{
				Slug:      "partial",
				RawStatus: "claimed_done",
				Body:      "- [~] manual QA partial",
			}},
			wantDone:       false,
			wantUnresolved: 1,
		},
		{
			name: "claimed done with resolved acceptance is clear",
			records: []taskDoneCheckRecord{{
				Slug:      "resolved",
				RawStatus: "claimed_done",
				Body:      "- [x] QA pass\n- [-] deferred elsewhere",
			}},
			wantDone: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildTaskDoneCheckOutput(scope, Deps{}, "agent:codex", tc.records)
			if got.IsDone != tc.wantDone || got.OpenTaskCount != tc.wantOpen || got.UnresolvedAcceptanceTaskCount != tc.wantUnresolved {
				t.Fatalf("done_check = done %v open %d unresolved %d, want done %v open %d unresolved %d",
					got.IsDone, got.OpenTaskCount, got.UnresolvedAcceptanceTaskCount,
					tc.wantDone, tc.wantOpen, tc.wantUnresolved)
			}
		})
	}
}

func TestTaskDoneCheckIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run task.done_check DB integration")
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
	projectSlug := fmt.Sprintf("done-check-%d", suffix)
	areaSlug := fmt.Sprintf("mcp-%d", suffix)
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	areaID := insertContextReceiptArea(t, ctx, pool, projectID, areaSlug)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	openTaskID := insertContextReceiptTask(t, ctx, pool, projectID, areaID, "open-task")
	_, _ = pool.Exec(ctx, `
		UPDATE artifacts
		   SET task_meta = jsonb_build_object('status', 'open', 'assignee', 'agent:codex', 'priority', 'p2')
		 WHERE id = $1::uuid
	`, openTaskID)
	claimedID := insertContextReceiptTask(t, ctx, pool, projectID, areaID, "claimed-partial")
	_, _ = pool.Exec(ctx, `
		UPDATE artifacts
		   SET task_meta = jsonb_build_object('status', 'claimed_done', 'assignee', 'agent:codex'),
		       body_markdown = '- [~] QA partial'
		 WHERE id = $1::uuid
	`, claimedID)

	scope := &auth.ProjectScope{ProjectID: projectID, ProjectSlug: projectSlug, ProjectLocale: "en", Role: "owner"}
	rows, err := pool.Query(ctx, `
		SELECT a.id::text, a.slug, a.title, ar.slug,
		       COALESCE(a.task_meta->>'priority', ''),
		       COALESCE(a.task_meta->>'status', ''),
		       a.body_markdown
		FROM artifacts a
		JOIN areas ar ON ar.id = a.area_id
		WHERE a.project_id = $1::uuid
		  AND COALESCE(a.task_meta->>'assignee', '') = 'agent:codex'
		ORDER BY a.slug
	`, projectID)
	if err != nil {
		t.Fatalf("query records: %v", err)
	}
	defer rows.Close()
	var records []taskDoneCheckRecord
	for rows.Next() {
		var rec taskDoneCheckRecord
		if err := rows.Scan(&rec.ArtifactID, &rec.Slug, &rec.Title, &rec.AreaSlug, &rec.Priority, &rec.RawStatus, &rec.Body); err != nil {
			t.Fatalf("scan records: %v", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	out := buildTaskDoneCheckOutput(scope, Deps{DB: pool}, "agent:codex", records)
	if out.IsDone || out.OpenTaskCount != 1 || out.UnresolvedAcceptanceTaskCount != 1 {
		t.Fatalf("done_check integration = done %v open %d unresolved %d", out.IsDone, out.OpenTaskCount, out.UnresolvedAcceptanceTaskCount)
	}
	if len(out.UnresolvedAcceptanceTasks) != 1 || len(out.UnresolvedAcceptanceTasks[0].UnresolvedAcceptanceLabels) != 1 {
		t.Fatalf("expected one unresolved acceptance label, got %+v", out.UnresolvedAcceptanceTasks)
	}

	callOut := callTaskDoneCheckForTest(t, ctx, pool, projectSlug, "agent:codex")
	if callOut.IsDone || callOut.OpenTaskCount != 1 || callOut.UnresolvedAcceptanceTaskCount != 1 {
		t.Fatalf("done_check tool call = done %v open %d unresolved %d", callOut.IsDone, callOut.OpenTaskCount, callOut.UnresolvedAcceptanceTaskCount)
	}
}

func callTaskDoneCheckForTest(t *testing.T, ctx context.Context, pool *db.Pool, projectSlug, assignee string) taskDoneCheckOutput {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-done-check-test", Version: "test"}, nil)
	RegisterTaskDoneCheck(server, Deps{
		DB:        pool,
		Logger:    logger,
		AuthChain: auth.NewChain(auth.NewTrustedLocalResolver("", "agent:codex")),
	})
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "task-done-check-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})

	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name: "pindoc.task.done_check",
		Arguments: map[string]any{
			"project_slug": projectSlug,
			"assignee":     assignee,
		},
	})
	if err != nil {
		t.Fatalf("CallTool task.done_check: %v", err)
	}
	if res.IsError {
		t.Fatalf("task.done_check result error: %s", toolResultText(res))
	}
	var out taskDoneCheckOutput
	if err := decodeStructuredContent(res.StructuredContent, &out); err != nil {
		t.Fatalf("decode task.done_check structured content: %v", err)
	}
	return out
}
