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
			if got.Mode != taskDoneCheckModeStrict {
				t.Fatalf("mode = %q, want strict", got.Mode)
			}
			if got.ModeIsDone != tc.wantDone {
				t.Fatalf("mode_is_done = %v, want %v", got.ModeIsDone, tc.wantDone)
			}
			if got.CurrentOpenWorkDone != (tc.wantOpen == 0) {
				t.Fatalf("current_open_work_done = %v, want %v", got.CurrentOpenWorkDone, tc.wantOpen == 0)
			}
			if got.HistoricalAcceptanceDebtClear != (tc.wantUnresolved == 0) {
				t.Fatalf("historical_acceptance_debt_clear = %v, want %v", got.HistoricalAcceptanceDebtClear, tc.wantUnresolved == 0)
			}
			if !strings.Contains(got.Summary, "open queue") || !strings.Contains(got.Summary, "historical claimed_done acceptance debt") {
				t.Fatalf("summary does not separate current queue and historical debt: %q", got.Summary)
			}
		})
	}
}

func TestBuildTaskDoneCheckOutputModes(t *testing.T) {
	scope := &auth.ProjectScope{ProjectSlug: "pindoc", ProjectLocale: "en"}
	records := []taskDoneCheckRecord{{
		Slug:      "claimed-partial",
		RawStatus: "claimed_done",
		Body:      "- [~] QA partial",
	}}

	strict := buildTaskDoneCheckOutputForMode(scope, Deps{}, "agent:codex", records, taskDoneCheckModeStrict)
	if strict.IsDone || strict.ModeIsDone || !strict.CurrentOpenWorkDone || strict.HistoricalAcceptanceDebtClear {
		t.Fatalf("strict mode = %+v, want current clear + historical debt blocking strict done", strict)
	}

	currentOnly := buildTaskDoneCheckOutputForMode(scope, Deps{}, "agent:codex", records, taskDoneCheckModeCurrentOpenOnly)
	if currentOnly.IsDone || !currentOnly.ModeIsDone || !currentOnly.CurrentOpenWorkDone || currentOnly.HistoricalAcceptanceDebtClear {
		t.Fatalf("current_open_only mode = %+v, want selected mode clear with strict is_done false", currentOnly)
	}
	for _, want := range []string{"Mode current_open_only clear", "open queue clear", "historical claimed_done acceptance debt remains", "legacy is_done=false"} {
		if !strings.Contains(currentOnly.Summary, want) {
			t.Fatalf("current_open_only summary %q missing %q", currentOnly.Summary, want)
		}
	}

	historicalDebt := buildTaskDoneCheckOutputForMode(scope, Deps{}, "agent:codex", records, taskDoneCheckModeHistoricalDebt)
	if historicalDebt.ModeIsDone || historicalDebt.HistoricalAcceptanceDebtClear {
		t.Fatalf("historical_debt mode = %+v, want debt present", historicalDebt)
	}
}

func TestNormalizeTaskDoneCheckMode(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"", taskDoneCheckModeStrict, true},
		{" strict ", taskDoneCheckModeStrict, true},
		{"CURRENT_OPEN_ONLY", taskDoneCheckModeCurrentOpenOnly, true},
		{"historical_debt", taskDoneCheckModeHistoricalDebt, true},
		{"current", "", false},
	}
	for _, tc := range cases {
		got, ok := normalizeTaskDoneCheckMode(tc.in)
		if ok != tc.wantOK || got != tc.want {
			t.Fatalf("normalizeTaskDoneCheckMode(%q) = %q/%v, want %q/%v", tc.in, got, ok, tc.want, tc.wantOK)
		}
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
	records := loadTaskDoneCheckRecordsForTest(t, ctx, pool, projectID, "agent:codex")

	out := buildTaskDoneCheckOutput(scope, Deps{DB: pool}, "agent:codex", records)
	if out.IsDone || out.OpenTaskCount != 1 || out.UnresolvedAcceptanceTaskCount != 1 {
		t.Fatalf("done_check integration = done %v open %d unresolved %d", out.IsDone, out.OpenTaskCount, out.UnresolvedAcceptanceTaskCount)
	}
	if len(out.UnresolvedAcceptanceTasks) != 1 || len(out.UnresolvedAcceptanceTasks[0].UnresolvedAcceptanceLabels) != 1 {
		t.Fatalf("expected one unresolved acceptance label, got %+v", out.UnresolvedAcceptanceTasks)
	}

	callOut := callTaskDoneCheckForTest(t, ctx, pool, projectSlug, "agent:codex", "")
	if callOut.IsDone || callOut.OpenTaskCount != 1 || callOut.UnresolvedAcceptanceTaskCount != 1 {
		t.Fatalf("done_check tool call = done %v open %d unresolved %d", callOut.IsDone, callOut.OpenTaskCount, callOut.UnresolvedAcceptanceTaskCount)
	}

	_, _ = pool.Exec(ctx, `
		UPDATE artifacts
		   SET task_meta = jsonb_build_object('status', 'claimed_done', 'assignee', 'agent:codex', 'priority', 'p2'),
		       body_markdown = '- [x] complete'
		 WHERE id = $1::uuid
	`, openTaskID)

	records = loadTaskDoneCheckRecordsForTest(t, ctx, pool, projectID, "agent:codex")
	currentOnly := buildTaskDoneCheckOutputForMode(scope, Deps{DB: pool}, "agent:codex", records, taskDoneCheckModeCurrentOpenOnly)
	if currentOnly.IsDone || !currentOnly.ModeIsDone || currentOnly.OpenTaskCount != 0 || currentOnly.UnresolvedAcceptanceTaskCount != 1 {
		t.Fatalf("queue 0 + historical debt current_open_only = %+v", currentOnly)
	}

	currentOnlyCall := callTaskDoneCheckForTest(t, ctx, pool, projectSlug, "agent:codex", taskDoneCheckModeCurrentOpenOnly)
	if currentOnlyCall.IsDone || !currentOnlyCall.ModeIsDone || !currentOnlyCall.CurrentOpenWorkDone || currentOnlyCall.HistoricalAcceptanceDebtClear {
		t.Fatalf("tool queue 0 + historical debt current_open_only = %+v", currentOnlyCall)
	}
	strictCall := callTaskDoneCheckForTest(t, ctx, pool, projectSlug, "agent:codex", taskDoneCheckModeStrict)
	if strictCall.IsDone || strictCall.ModeIsDone || !strictCall.CurrentOpenWorkDone || strictCall.HistoricalAcceptanceDebtClear {
		t.Fatalf("tool queue 0 + historical debt strict = %+v", strictCall)
	}
}

func loadTaskDoneCheckRecordsForTest(t *testing.T, ctx context.Context, pool *db.Pool, projectID, assignee string) []taskDoneCheckRecord {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT a.id::text, a.slug, a.title, ar.slug,
		       COALESCE(a.task_meta->>'priority', ''),
		       COALESCE(a.task_meta->>'status', ''),
		       a.body_markdown
		FROM artifacts a
		JOIN areas ar ON ar.id = a.area_id
		WHERE a.project_id = $1::uuid
		  AND COALESCE(a.task_meta->>'assignee', '') = $2
		ORDER BY a.slug
	`, projectID, assignee)
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
	return records
}

func callTaskDoneCheckForTest(t *testing.T, ctx context.Context, pool *db.Pool, projectSlug, assignee, mode string) taskDoneCheckOutput {
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

	args := map[string]any{
		"project_slug": projectSlug,
		"assignee":     assignee,
	}
	if strings.TrimSpace(mode) != "" {
		args["mode"] = mode
	}
	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name:      "pindoc.task.done_check",
		Arguments: args,
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
