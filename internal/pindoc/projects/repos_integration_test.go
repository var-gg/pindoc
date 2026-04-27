package projects

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestProjectReposIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run project_repos DB integration")
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

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := CreateProject(ctx, tx, CreateProjectInput{
		Slug:            "repo-integration",
		Name:            "Project Repo Integration",
		PrimaryLanguage: "en",
		GitRemoteURL:    "git@github.com:Var-GG/Pindoc.git",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	assertProjectRepoRow(t, ctx, tx, out.ID, "github.com/var-gg/pindoc", "git@github.com:Var-GG/Pindoc.git")
	if got, err := LookupProjectSlugByGitRemoteURL(ctx, tx, "https://github.com/var-gg/pindoc.git"); err != nil || got != out.Slug {
		t.Fatalf("LookupProjectSlugByGitRemoteURL = %q, %v; want %q, nil", got, err, out.Slug)
	}
	if _, err := AddProjectRepo(ctx, tx, ProjectRepoInput{
		ProjectID:    out.ID,
		GitRemoteURL: "https://github.com/var-gg/pindoc.git",
	}); err != nil {
		t.Fatalf("AddProjectRepo duplicate normalized remote: %v", err)
	}
	assertProjectRepoCount(t, ctx, tx, out.ID, 1)
	assertProjectRepoIndexUsed(t, ctx, tx)
	assertProjectRepoCascade(t, ctx, tx, out.ID)
}

func assertProjectRepoRow(t *testing.T, ctx context.Context, tx pgx.Tx, projectID, wantRemote, wantOriginal string) {
	t.Helper()
	var gotRemote, gotOriginal, gotName, gotBranch string
	if err := tx.QueryRow(ctx, `
		SELECT git_remote_url, git_remote_url_original, name, default_branch
		  FROM project_repos
		 WHERE project_id = $1::uuid
	`, projectID).Scan(&gotRemote, &gotOriginal, &gotName, &gotBranch); err != nil {
		t.Fatalf("select project repo: %v", err)
	}
	if gotRemote != wantRemote || gotOriginal != wantOriginal || gotName != "origin" || gotBranch != "main" {
		t.Fatalf("project repo = remote=%q original=%q name=%q branch=%q", gotRemote, gotOriginal, gotName, gotBranch)
	}
}

func assertProjectRepoCount(t *testing.T, ctx context.Context, tx pgx.Tx, projectID string, want int) {
	t.Helper()
	var got int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM project_repos WHERE project_id = $1::uuid
	`, projectID).Scan(&got); err != nil {
		t.Fatalf("count project repos: %v", err)
	}
	if got != want {
		t.Fatalf("project repo count = %d, want %d", got, want)
	}
}

func assertProjectRepoIndexUsed(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("disable seqscan: %v", err)
	}
	rows, err := tx.Query(ctx, `EXPLAIN SELECT project_id FROM project_repos WHERE git_remote_url = 'github.com/var-gg/pindoc'`)
	if err != nil {
		t.Fatalf("explain project_repos lookup: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if !strings.Contains(plan.String(), "idx_project_repos_git_remote_url") {
		t.Fatalf("expected idx_project_repos_git_remote_url in plan:\n%s", plan.String())
	}
}

func assertProjectRepoCascade(t *testing.T, ctx context.Context, tx pgx.Tx, projectID string) {
	t.Helper()
	if _, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1::uuid`, projectID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	assertProjectRepoCount(t, ctx, tx, projectID, 0)
}
