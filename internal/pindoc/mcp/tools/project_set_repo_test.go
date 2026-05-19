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
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
	"github.com/var-gg/pindoc/internal/pindoc/telemetry"
)

func TestProjectSetRepoMCPIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run project.set_repo MCP integration")
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
	slug := "set-repo-" + suffix
	if err := createSetRepoProject(ctx, pool, slug); err != nil {
		t.Fatalf("create project: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	agentID := "set-repo-test-" + suffix
	tele := telemetry.New(ctx, pool.Pool, logger, telemetry.Options{BatchSize: 1, FlushEvery: 10 * time.Millisecond})
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-test", Version: "test"}, nil)
	deps := Deps{
		DB:        pool,
		Logger:    logger,
		Telemetry: tele,
		AuthChain: auth.NewChain(auth.NewTrustedLocalResolver("", agentID)),
		BindAddr:  config.DefaultBindAddr,
	}
	RegisterProjectSetRepo(server, deps)
	RegisterWorkspaceDetect(server, deps)

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "set-repo-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() {
		clientSession.Close()
		serverSession.Wait()
	}()

	remote := "https://github.com/var-gg/" + slug + ".git"
	wantNormalized := "github.com/var-gg/" + slug
	localPath := "/tmp/" + slug + "/checkout"

	// First call — registers the row.
	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name: "pindoc.project.set_repo",
		Arguments: map[string]any{
			"project_slug":   slug,
			"git_remote_url": remote,
			"local_paths":    []string{localPath},
		},
	})
	if err != nil {
		t.Fatalf("CallTool first set_repo: %v", err)
	}
	if res.IsError {
		t.Fatalf("first set_repo error: %s", toolResultText(res))
	}
	var first projectSetRepoOutput
	if err := decodeStructuredContent(res.StructuredContent, &first); err != nil {
		t.Fatalf("decode first set_repo: %v", err)
	}
	if first.Status != "ok" {
		t.Fatalf("first set_repo status = %q, want ok (full=%+v)", first.Status, first)
	}
	if !first.Created {
		t.Fatalf("first set_repo created = false, want true")
	}
	if first.Code != "PROJECT_REPO_REGISTERED" {
		t.Fatalf("first set_repo code = %q, want PROJECT_REPO_REGISTERED", first.Code)
	}
	if first.GitRemoteURL != wantNormalized {
		t.Fatalf("first set_repo normalized git_remote_url = %q, want %q", first.GitRemoteURL, wantNormalized)
	}

	// Second call — idempotent refresh, merges a new local_path.
	res2, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name: "pindoc.project.set_repo",
		Arguments: map[string]any{
			"project_slug":   slug,
			"git_remote_url": remote,
			"local_paths":    []string{"/tmp/" + slug + "/another"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool second set_repo: %v", err)
	}
	if res2.IsError {
		t.Fatalf("second set_repo error: %s", toolResultText(res2))
	}
	var second projectSetRepoOutput
	if err := decodeStructuredContent(res2.StructuredContent, &second); err != nil {
		t.Fatalf("decode second set_repo: %v", err)
	}
	if second.Created {
		t.Fatalf("second set_repo created = true, want false")
	}
	if second.Code != "PROJECT_REPO_REFRESHED" {
		t.Fatalf("second set_repo code = %q, want PROJECT_REPO_REFRESHED", second.Code)
	}
	if second.RepoID != first.RepoID {
		t.Fatalf("second set_repo repo_id = %q, want %q", second.RepoID, first.RepoID)
	}
	if len(second.LocalPaths) != 2 {
		t.Fatalf("second set_repo local_paths = %v, want 2 merged entries", second.LocalPaths)
	}

	// Subsequent workspace.detect with the same git_remote_url should now
	// resolve via git_remote (high) and not attach a register hint.
	res3, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name: "pindoc.workspace.detect",
		Arguments: map[string]any{
			"git_remote_url": remote,
		},
	})
	if err != nil {
		t.Fatalf("CallTool post-set_repo workspace.detect: %v", err)
	}
	if res3.IsError {
		t.Fatalf("post-set_repo workspace.detect error: %s", toolResultText(res3))
	}
	var detect workspaceDetectOutput
	if err := decodeStructuredContent(res3.StructuredContent, &detect); err != nil {
		t.Fatalf("decode workspace.detect: %v", err)
	}
	if detect.ProjectSlug != slug || detect.Confidence != "high" || detect.Via != "git_remote" {
		t.Fatalf("post-set_repo workspace.detect = %+v, want slug=%s via=git_remote confidence=high", detect, slug)
	}
	if detect.NextAction != nil {
		t.Fatalf("post-set_repo workspace.detect attached next_action %+v, want nil", detect.NextAction)
	}
	for _, w := range detect.Warnings {
		if strings.HasPrefix(w, "PROJECT_REPOS_EMPTY_") {
			t.Fatalf("post-set_repo workspace.detect emitted %s, want no warning", w)
		}
	}
}

func TestWorkspaceDetectEmitsRegisterHint(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run workspace.detect register-hint test")
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
	slug := "register-hint-" + suffix
	if err := createSetRepoProject(ctx, pool, slug); err != nil {
		t.Fatalf("create project: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	agentID := "register-hint-test-" + suffix
	tele := telemetry.New(ctx, pool.Pool, logger, telemetry.Options{BatchSize: 1, FlushEvery: 10 * time.Millisecond})
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-test", Version: "test"}, nil)
	deps := Deps{
		DB:        pool,
		Logger:    logger,
		Telemetry: tele,
		AuthChain: auth.NewChain(auth.NewTrustedLocalResolver("", agentID)),
		BindAddr:  config.DefaultBindAddr,
	}
	RegisterWorkspaceDetect(server, deps)

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "register-hint-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() {
		clientSession.Close()
		serverSession.Wait()
	}()

	// Frontmatter resolves the slug high-confidence; git_remote_url is
	// provided but has no project_repos row yet — the dogfood scenario.
	remote := "https://github.com/var-gg/" + slug + ".git"
	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name: "pindoc.workspace.detect",
		Arguments: map[string]any{
			"git_remote_url": remote,
			"workspace_path": "/tmp/" + slug,
			"pindoc_md_frontmatter": map[string]any{
				"project_slug": slug,
			},
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
	if out.ProjectSlug != slug || out.Confidence != "high" || out.Via != "pindoc_md" {
		t.Fatalf("workspace.detect = %+v, want pindoc_md high-confidence for slug=%s", out, slug)
	}
	if out.NextAction == nil {
		t.Fatalf("workspace.detect did not attach next_action, want project.set_repo hint")
	}
	if out.NextAction.Tool != "pindoc.project.set_repo" {
		t.Fatalf("next_action.tool = %q, want pindoc.project.set_repo", out.NextAction.Tool)
	}
	gotSlug, _ := out.NextAction.Args["project_slug"].(string)
	if gotSlug != slug {
		t.Fatalf("next_action.args.project_slug = %q, want %q", gotSlug, slug)
	}
	gotRemote, _ := out.NextAction.Args["git_remote_url"].(string)
	if gotRemote != remote {
		t.Fatalf("next_action.args.git_remote_url = %q, want %q", gotRemote, remote)
	}
	if !containsWarning(out.Warnings, "PROJECT_REPOS_EMPTY_FOR_PINDOC_MD_MATCH") {
		t.Fatalf("warnings = %v, want PROJECT_REPOS_EMPTY_FOR_PINDOC_MD_MATCH", out.Warnings)
	}
}

func containsWarning(warnings []string, code string) bool {
	for _, w := range warnings {
		if w == code {
			return true
		}
	}
	return false
}

func createSetRepoProject(ctx context.Context, pool *db.Pool, slug string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            slug,
		Name:            "Set Repo " + slug,
		PrimaryLanguage: "en",
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
