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

func TestTaskQueueAcrossProjectsIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run task.queue across_projects DB integration")
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
	projectSlugA := fmt.Sprintf("task-queue-across-a-%d", suffix)
	projectSlugB := fmt.Sprintf("task-queue-across-b-%d", suffix)
	projectIDA := insertContextReceiptProject(t, ctx, pool, projectSlugA)
	projectIDB := insertContextReceiptProject(t, ctx, pool, projectSlugB)
	areaIDA := insertContextReceiptArea(t, ctx, pool, projectIDA, "mcp")
	areaIDB := insertContextReceiptArea(t, ctx, pool, projectIDB, "mcp")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id IN ($1::uuid, $2::uuid)`, projectIDA, projectIDB)
	})

	taskIDA := insertContextReceiptTask(t, ctx, pool, projectIDA, areaIDA, "assigned-a")
	taskIDB := insertContextReceiptTask(t, ctx, pool, projectIDB, areaIDB, "assigned-b")
	setTaskQueueTestAssignee(t, ctx, pool, taskIDA, "agent:codex")
	setTaskQueueTestAssignee(t, ctx, pool, taskIDB, "agent:codex")

	statusFilter, ok := normalizeTaskQueueStatusFilter("")
	if !ok {
		t.Fatal("default status filter should normalize")
	}
	deps := Deps{
		DB:       pool,
		RepoRoot: `A:\vargg-workspace\pindoc`,
	}
	principal := &auth.Principal{AgentID: "codex", Source: auth.SourceLoopback}
	directOut, err := handleTaskQueueAcrossProjects(ctx, deps, principal, taskQueueInput{
		AcrossProjects: true,
		Compact:        true,
	}, statusFilter, "", taskQueueDefaultLimit)
	if err != nil {
		t.Fatalf("handleTaskQueueAcrossProjects: %v", err)
	}
	assertTaskQueueAcrossOutput(t, directOut, projectSlugA, projectSlugB)

	toolOut := callTaskQueueForTest(t, ctx, pool, Deps{
		RepoRoot: `A:\vargg-workspace\pindoc`,
	}, map[string]any{
		"across_projects": true,
		"compact":         true,
	})
	assertTaskQueueAcrossOutput(t, toolOut, projectSlugA, projectSlugB)

	scopedOut := callTaskQueueForTest(t, ctx, pool, Deps{
		DefaultProjectSlug: projectSlugA,
		RepoRoot:           `A:\vargg-workspace\pindoc`,
	}, map[string]any{
		"assignee": "agent:codex",
	})
	if len(scopedOut.Warnings) != 1 || scopedOut.Warnings[0].Code != taskWarningMultiProjectWorkspace {
		t.Fatalf("omitted project_slug should warn in multi-project workspace: %+v", scopedOut.Warnings)
	}

	missingDefaultOut := callTaskQueueForTest(t, ctx, pool, Deps{}, map[string]any{
		"assignee": "agent:codex",
	})
	if len(missingDefaultOut.Warnings) != 1 || missingDefaultOut.Warnings[0].Code != taskWarningMultiProjectWorkspace {
		t.Fatalf("omitted project_slug without default should still warn in multi-project workspace: %+v", missingDefaultOut.Warnings)
	}
	if len(missingDefaultOut.Items) != 0 {
		t.Fatalf("warning-only omitted project_slug response should not imply a scoped queue: %+v", missingDefaultOut.Items)
	}

	explicitOut := callTaskQueueForTest(t, ctx, pool, Deps{
		DefaultProjectSlug: projectSlugA,
		RepoRoot:           `A:\vargg-workspace\pindoc`,
	}, map[string]any{
		"project_slug": projectSlugA,
		"assignee":     "agent:codex",
	})
	if len(explicitOut.Warnings) != 0 {
		t.Fatalf("explicit project_slug should not warn: %+v", explicitOut.Warnings)
	}

	ctxWithDefault := withProjectSlugDefaultResult(ctx, projectSlugDefaultResult{
		ProjectSlug: projectSlugA,
		Via:         projectSlugDefaultEnv,
	})
	warning, ok := taskQueueMultiProjectWorkspaceWarning(ctxWithDefault, deps, principal)
	if !ok {
		t.Fatal("expected omitted project_slug warning in multi-project workspace")
	}
	if warning.Code != taskWarningMultiProjectWorkspace {
		t.Fatalf("warning code = %q", warning.Code)
	}
	if !taskQueueWarningIncludesProject(warning, projectSlugA) || !taskQueueWarningIncludesProject(warning, projectSlugB) {
		t.Fatalf("warning detected_projects should include test projects: %+v", warning)
	}
	if _, ok := taskQueueMultiProjectWorkspaceWarning(ctx, deps, principal); ok {
		t.Fatal("explicit project_slug path without defaulting context must not warn")
	}
}

func assertTaskQueueAcrossOutput(t *testing.T, out taskQueueOutput, projectSlugA, projectSlugB string) {
	t.Helper()
	if !out.AcrossProjects {
		t.Fatalf("across_projects mirror should be true")
	}
	if out.WorkspaceRoot == "" {
		t.Fatalf("workspace_root should be present in across_projects output")
	}
	if out.TotalAssigneeOpenCount == nil || *out.TotalAssigneeOpenCount < 2 {
		t.Fatalf("total_assignee_open_count = %v, want at least 2", out.TotalAssigneeOpenCount)
	}
	assertTaskQueueAcrossProject(t, out, projectSlugA)
	assertTaskQueueAcrossProject(t, out, projectSlugB)
}

func setTaskQueueTestAssignee(t *testing.T, ctx context.Context, pool *db.Pool, taskID, assignee string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		UPDATE artifacts
		   SET task_meta = jsonb_build_object('status', 'open', 'assignee', $2::text, 'priority', 'p2')
		 WHERE id = $1::uuid
	`, taskID, assignee); err != nil {
		t.Fatalf("set task assignee: %v", err)
	}
}

func assertTaskQueueAcrossProject(t *testing.T, out taskQueueOutput, projectSlug string) {
	t.Helper()
	projectOut, ok := out.Projects[projectSlug]
	if !ok {
		t.Fatalf("projects map missing %q in keys %v", projectSlug, taskQueueProjectKeys(out.Projects))
	}
	if projectOut.AssigneeOpenCount != 1 {
		t.Fatalf("%s assignee_open_count = %d, want 1", projectSlug, projectOut.AssigneeOpenCount)
	}
	if len(projectOut.Items) != 1 {
		t.Fatalf("%s items len = %d, want 1 (%+v)", projectSlug, len(projectOut.Items), projectOut.Items)
	}
	if projectOut.Items[0].ProjectSlug != projectSlug {
		t.Fatalf("%s item project_slug = %q", projectSlug, projectOut.Items[0].ProjectSlug)
	}
}

func taskQueueWarningIncludesProject(warning taskQueueWarning, projectSlug string) bool {
	for _, got := range warning.DetectedProjects {
		if got == projectSlug {
			return true
		}
	}
	return false
}

func taskQueueProjectKeys(projects map[string]taskQueueProjectOutput) []string {
	keys := make([]string, 0, len(projects))
	for key := range projects {
		keys = append(keys, key)
	}
	return keys
}

func callTaskQueueForTest(t *testing.T, ctx context.Context, pool *db.Pool, deps Deps, args map[string]any) taskQueueOutput {
	t.Helper()
	deps.DB = pool
	if deps.AuthChain == nil {
		deps.AuthChain = auth.NewChain(auth.NewTrustedLocalResolver("", "codex"))
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-task-queue-test", Version: "test"}, nil)
	RegisterTaskQueue(server, deps)

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "task-queue-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})

	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name:      "pindoc.task.queue",
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool task.queue: %v", err)
	}
	if res.IsError {
		t.Fatalf("task.queue result error: %s", toolResultText(res))
	}
	var out taskQueueOutput
	if err := decodeStructuredContent(res.StructuredContent, &out); err != nil {
		t.Fatalf("decode task.queue structured content: %v", err)
	}
	return out
}
