package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
)

func TestWorkspaceDetectPriorityChain(t *testing.T) {
	frontmatter := &pingPindocFrontmatter{ProjectSlug: "pindoc", SchemaVersion: "1"}
	got := detectWorkspaceFromSources(workspaceDetectInput{
		WorkspacePath: "/work/other",
		GitRemoteURL:  "https://github.com/var-gg/other.git",
		Frontmatter:   frontmatter,
	}, []string{"pindoc", "other"}, func(string) (string, bool) { return "other", true })
	assertWorkspaceDetect(t, got, "pindoc", "high", "pindoc_md", nil)

	got = detectWorkspaceFromSources(workspaceDetectInput{
		WorkspacePath: "/work/other",
		GitRemoteURL:  "https://github.com/var-gg/pindoc.git",
	}, []string{"pindoc", "other"}, func(string) (string, bool) { return "pindoc", true })
	assertWorkspaceDetect(t, got, "pindoc", "high", "git_remote", nil)

	got = detectWorkspaceFromSources(workspaceDetectInput{
		WorkspacePath: "/work/pindoc",
	}, []string{"pindoc", "var-gg"}, nil)
	assertWorkspaceDetect(t, got, "pindoc", "medium", "directory_match", nil)

	got = detectWorkspaceFromSources(workspaceDetectInput{
		WorkspacePath: "/work/client-vargg",
	}, []string{"pindoc", "vargg"}, nil)
	assertWorkspaceDetect(t, got, "vargg", "low", "directory_match", []string{"vargg"})

	got = detectWorkspaceFromSources(workspaceDetectInput{}, []string{"pindoc"}, nil)
	assertWorkspaceDetect(t, got, "pindoc", "medium", "fallback_only_one", nil)

	got = detectWorkspaceFromSources(workspaceDetectInput{}, []string{"pindoc", "vargg"}, nil)
	assertWorkspaceDetect(t, got, "", "none", "fallback_required", []string{"pindoc", "vargg"})
}

func TestWorkspaceDetectMembershipGuard(t *testing.T) {
	frontmatter := &pingPindocFrontmatter{ProjectSlug: "secret", SchemaVersion: "1"}
	got := detectWorkspaceFromSources(workspaceDetectInput{
		Frontmatter: frontmatter,
	}, []string{"pindoc"}, nil)
	assertWorkspaceDetect(t, got, "", "none", "pindoc_md", nil)

	got = detectWorkspaceFromSources(workspaceDetectInput{
		GitRemoteURL: "https://github.com/acme/secret.git",
	}, []string{"pindoc"}, func(string) (string, bool) { return "secret", true })
	assertWorkspaceDetect(t, got, "pindoc", "medium", "fallback_only_one", nil)
}

func TestWorkspaceDetectMCPIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run workspace.detect MCP integration")
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

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	slug := "workspace-detect-" + suffix
	remote := "https://github.com/var-gg/workspace-detect-" + suffix + ".git"
	if err := createWorkspaceDetectProject(ctx, pool, slug, remote); err != nil {
		t.Fatalf("create project: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	agentID := "workspace-detect-test-" + suffix
	tele := telemetry.New(ctx, pool.Pool, logger, telemetry.Options{BatchSize: 1, FlushEvery: 10 * time.Millisecond})
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-test", Version: "test"}, nil)
	RegisterWorkspaceDetect(server, Deps{
		DB:        pool,
		Logger:    logger,
		Telemetry: tele,
		AuthChain: auth.NewChain(auth.NewTrustedLocalResolver("", agentID)),
		BindAddr:  config.DefaultBindAddr,
	})

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "workspace-detect-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() {
		clientSession.Close()
		serverSession.Wait()
	}()

	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name: "pindoc.workspace.detect",
		Arguments: map[string]any{
			"git_remote_url": remote,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool result error: %s", toolResultText(res))
	}
	var out workspaceDetectOutput
	if err := decodeStructuredContent(res.StructuredContent, &out); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	assertWorkspaceDetect(t, out, slug, "high", "git_remote", nil)

	tele.Close()
	assertWorkspaceDetectTelemetry(t, ctx, pool, agentID)
}

func assertWorkspaceDetect(t *testing.T, got workspaceDetectOutput, slug, confidence, via string, candidates []string) {
	t.Helper()
	if got.ProjectSlug != slug || got.Confidence != confidence || got.Via != via {
		t.Fatalf("workspace detect = slug=%q confidence=%q via=%q candidates=%v reason=%q; want slug=%q confidence=%q via=%q candidates=%v",
			got.ProjectSlug, got.Confidence, got.Via, got.Candidates, got.Reason, slug, confidence, via, candidates)
	}
	if strings.Join(got.Candidates, ",") != strings.Join(candidates, ",") {
		t.Fatalf("candidates = %v, want %v", got.Candidates, candidates)
	}
}

func createWorkspaceDetectProject(ctx context.Context, pool *db.Pool, slug, remote string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            slug,
		Name:            "Workspace Detect " + slug,
		PrimaryLanguage: "en",
		GitRemoteURL:    remote,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func decodeStructuredContent(raw any, out any) error {
	body, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func toolResultText(res *sdk.CallToolResult) string {
	if res == nil {
		return "<nil>"
	}
	var parts []string
	for _, content := range res.Content {
		if text, ok := content.(*sdk.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func assertWorkspaceDetectTelemetry(t *testing.T, ctx context.Context, pool *db.Pool, agentID string) {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM mcp_tool_calls
		 WHERE tool_name = 'pindoc.workspace.detect'
		   AND agent_id = $1
	`, agentID).Scan(&n); err != nil {
		t.Fatalf("query telemetry: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected workspace.detect telemetry row for agent %s", agentID)
	}
}
