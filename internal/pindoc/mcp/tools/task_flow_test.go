package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestTaskFlowOrderingAndActorNormalization(t *testing.T) {
	now := time.Now()
	rows := []taskFlowRow{
		{ProjectSlug: "p", Slug: "blocked", Readiness: taskReadinessBlocked, Priority: "p0", UpdatedAt: now.Add(-3 * time.Hour)},
		{ProjectSlug: "p", Slug: "ready-p2", Readiness: taskReadinessReady, Priority: "p2", UpdatedAt: now.Add(-4 * time.Hour)},
		{ProjectSlug: "p", Slug: "ready-p0-new", Readiness: taskReadinessReady, Priority: "p0", UpdatedAt: now.Add(-1 * time.Hour)},
		{ProjectSlug: "p", Slug: "ready-p0-old", Readiness: taskReadinessReady, Priority: "p0", UpdatedAt: now.Add(-2 * time.Hour)},
	}
	sortTaskFlowRows(rows)
	got := []string{rows[0].Slug, rows[1].Slug, rows[2].Slug, rows[3].Slug}
	want := []string{"ready-p0-old", "ready-p0-new", "ready-p2", "blocked"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sorted rows = %v, want %v", got, want)
	}

	agent, err := normalizeTaskFlowActor(&auth.Principal{AgentID: "codex"}, "agent", "", nil, true, true)
	if err != nil {
		t.Fatalf("normalize agent: %v", err)
	}
	if len(agent.IDs) != 1 || agent.IDs[0] != "agent:codex" || !agent.IncludeUnassigned {
		t.Fatalf("agent actor = %+v", agent)
	}

	user, err := normalizeTaskFlowActor(&auth.Principal{UserID: "user-1"}, "user", "", nil, false, true)
	if err != nil {
		t.Fatalf("normalize user: %v", err)
	}
	if len(user.IDs) != 1 || user.IDs[0] != "user:user-1" {
		t.Fatalf("user actor = %+v", user)
	}

	if _, err := normalizeTaskFlowActor(&auth.Principal{}, "agent", "", nil, false, true); err == nil {
		t.Fatalf("missing actor should fail when required")
	}
}

func TestTaskFlowNextIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run task.flow/task.next DB integration")
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

	suffix := time.Now().UnixNano()
	projectSlugA := fmt.Sprintf("task-flow-a-%d", suffix)
	projectSlugB := fmt.Sprintf("task-flow-b-%d", suffix)
	areaSlug := fmt.Sprintf("flow-%d", suffix)
	blockerAreaSlug := fmt.Sprintf("flow-blockers-%d", suffix)
	projectIDA := insertContextReceiptProject(t, ctx, pool, projectSlugA)
	projectIDB := insertContextReceiptProject(t, ctx, pool, projectSlugB)
	areaIDA := insertContextReceiptArea(t, ctx, pool, projectIDA, areaSlug)
	blockerAreaIDA := insertContextReceiptArea(t, ctx, pool, projectIDA, blockerAreaSlug)
	areaIDB := insertContextReceiptArea(t, ctx, pool, projectIDB, areaSlug)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id IN ($1::uuid, $2::uuid)`, projectIDA, projectIDB)
	})

	old := time.Now().Add(-4 * time.Hour)
	readyUnassigned := insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectIDA,
		AreaID:    areaIDA,
		Slug:      "ready-unassigned",
		Type:      "Task",
		Title:     "Ready unassigned",
		Body:      "- [ ] do work\n",
		Locale:    "en",
		Status:    "published",
		TaskMeta:  `{"status":"open","priority":"p0"}`,
		UpdatedAt: old,
	})
	_ = readyUnassigned
	insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectIDA,
		AreaID:    areaIDA,
		Slug:      "ready-assigned",
		Type:      "Task",
		Title:     "Ready assigned",
		Body:      "- [ ] do work\n",
		Locale:    "en",
		Status:    "published",
		TaskMeta:  `{"status":"open","priority":"p1","assignee":"agent:codex"}`,
		UpdatedAt: old.Add(time.Hour),
	})
	blockedTargetID := insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectIDA,
		AreaID:    areaIDA,
		Slug:      "blocked-target",
		Type:      "Task",
		Title:     "Blocked target",
		Body:      "- [ ] blocked work\n",
		Locale:    "en",
		Status:    "published",
		TaskMeta:  `{"status":"open","priority":"p0","assignee":"agent:codex"}`,
		UpdatedAt: old.Add(2 * time.Hour),
	})
	blockerID := insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectIDA,
		AreaID:    blockerAreaIDA,
		Slug:      "blocker-source",
		Type:      "Task",
		Title:     "Blocker source",
		Body:      "- [ ] prerequisite\n",
		Locale:    "en",
		Status:    "published",
		TaskMeta:  `{"status":"open","priority":"p0","assignee":"agent:other"}`,
		UpdatedAt: old,
	})
	insertTaskFlowEdge(t, ctx, pool, blockerID, blockedTargetID)
	insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectIDB,
		AreaID:    areaIDB,
		Slug:      "ready-b",
		Type:      "Task",
		Title:     "Ready B",
		Body:      "- [ ] do work\n",
		Locale:    "en",
		Status:    "published",
		TaskMeta:  `{"status":"open","priority":"p2","assignee":"agent:codex"}`,
		UpdatedAt: old.Add(3 * time.Hour),
	})

	single := callTaskFlowForTest(t, ctx, pool, map[string]any{
		"project_slug": projectSlugA,
		"area_slug":    areaSlug,
		"actor_scope":  "all_visible",
	})
	if !taskFlowHasSlug(single.Items, "ready-assigned") || taskFlowHasSlug(single.Items, "ready-b") {
		t.Fatalf("single-project flow leaked/missed rows: %+v", single.Items)
	}

	visible := callTaskFlowForTest(t, ctx, pool, map[string]any{
		"project_scope": "visible",
		"area_slug":     areaSlug,
		"actor_scope":   "all_visible",
		"limit":         100,
	})
	for _, slug := range []string{"ready-unassigned", "ready-assigned", "blocked-target", "ready-b"} {
		if !taskFlowHasSlug(visible.Items, slug) {
			t.Fatalf("visible flow missing %s: %+v", slug, visible.Items)
		}
	}
	blocked := taskFlowFindBySlug(visible.Items, "blocked-target")
	if blocked == nil || blocked.Readiness != taskReadinessBlocked || len(blocked.Blockers) != 1 || blocked.Blockers[0].Slug != "blocker-source" {
		t.Fatalf("blocked target row = %+v", blocked)
	}

	next := callTaskNextForTest(t, ctx, pool, map[string]any{
		"project_slugs": []string{projectSlugA, projectSlugB},
		"area_slug":     areaSlug,
		"actor_scope":   "agent",
		"actor_id":      "codex",
		"limit":         10,
	})
	if len(next.Candidates) != 3 {
		t.Fatalf("next candidates len = %d, want 3: %+v", len(next.Candidates), next.Candidates)
	}
	if next.Candidates[0].Slug != "ready-unassigned" || !next.Candidates[0].ClaimRequired {
		t.Fatalf("first next candidate = %+v, want unassigned claim-required", next.Candidates[0])
	}
	if len(next.ExcludedBlockers) != 1 || next.ExcludedBlockers[0].Slug != "blocked-target" || !strings.Contains(next.BlockerSummary, "1 Task") {
		t.Fatalf("next excluded blockers = %+v summary=%q", next.ExcludedBlockers, next.BlockerSummary)
	}
	if next.ClaimPolicy.LeaseSupported || next.ClaimPolicy.ClaimTool != "pindoc.task.assign" {
		t.Fatalf("claim policy = %+v", next.ClaimPolicy)
	}
	if len(next.NextTools) == 0 || next.NextTools[0].Tool != "pindoc.task.assign" {
		t.Fatalf("next tools should start with task.assign for unassigned candidate: %+v", next.NextTools)
	}

	noReady := callTaskNextForTest(t, ctx, pool, map[string]any{
		"project_slug":       projectSlugA,
		"area_slug":          areaSlug,
		"actor_scope":        "agent",
		"actor_id":           "codex",
		"include_unassigned": false,
		"priority":           "p0",
	})
	if len(noReady.Candidates) != 0 || len(noReady.ExcludedBlockers) != 1 || noReady.NoReadyReason == "" {
		t.Fatalf("no-ready output = %+v", noReady)
	}
}

func insertTaskFlowEdge(t *testing.T, ctx context.Context, pool *db.Pool, sourceID, targetID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifact_edges (source_id, target_id, relation)
		VALUES ($1::uuid, $2::uuid, 'blocks')
		ON CONFLICT DO NOTHING
	`, sourceID, targetID); err != nil {
		t.Fatalf("insert task flow edge: %v", err)
	}
}

func callTaskFlowForTest(t *testing.T, ctx context.Context, pool *db.Pool, args map[string]any) taskFlowOutput {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-task-flow-test", Version: "test"}, nil)
	RegisterTaskFlow(server, Deps{
		DB:        pool,
		AuthChain: auth.NewChain(auth.NewTrustedLocalResolver("", "agent:codex")),
	})
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "task-flow-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})
	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{Name: "pindoc.task.flow", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool task.flow: %v", err)
	}
	if res.IsError {
		t.Fatalf("task.flow result error: %s", toolResultText(res))
	}
	var out taskFlowOutput
	if err := decodeStructuredContent(res.StructuredContent, &out); err != nil {
		t.Fatalf("decode task.flow structured content: %v", err)
	}
	return out
}

func callTaskNextForTest(t *testing.T, ctx context.Context, pool *db.Pool, args map[string]any) taskNextOutput {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-task-next-test", Version: "test"}, nil)
	RegisterTaskNext(server, Deps{
		DB:        pool,
		AuthChain: auth.NewChain(auth.NewTrustedLocalResolver("", "agent:codex")),
	})
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "task-next-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})
	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{Name: "pindoc.task.next", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool task.next: %v", err)
	}
	if res.IsError {
		t.Fatalf("task.next result error: %s", toolResultText(res))
	}
	var out taskNextOutput
	if err := decodeStructuredContent(res.StructuredContent, &out); err != nil {
		t.Fatalf("decode task.next structured content: %v", err)
	}
	return out
}

func taskFlowHasSlug(rows []taskFlowRow, slug string) bool {
	return taskFlowFindBySlug(rows, slug) != nil
}

func taskFlowFindBySlug(rows []taskFlowRow, slug string) *taskFlowRow {
	for i := range rows {
		if rows[i].Slug == slug {
			return &rows[i]
		}
	}
	return nil
}
