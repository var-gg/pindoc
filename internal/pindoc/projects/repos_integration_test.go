package projects

import (
	"context"
	"fmt"
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

	// Per-run unique git remote so the test never collides with whatever
	// `pindoc` (or any other project) already has registered in the dev
	// DB the suite runs against. Without this the lookup below returns
	// the older `pindoc` row by ORDER BY pr.created_at ASC and the test
	// fails on a fresh checkout. Repo-integration is checking the lookup
	// contract, not the canonical pindoc slug, so a synthetic remote is
	// fine here.
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	sshRemote := fmt.Sprintf("git@github.com:Repo-IT-%s/Pindoc.git", suffix)
	httpsRemote := fmt.Sprintf("https://github.com/repo-it-%s/pindoc.git", suffix)
	wantNormalized := fmt.Sprintf("github.com/repo-it-%s/pindoc", suffix)

	out, err := CreateProject(ctx, tx, CreateProjectInput{
		Slug:            "repo-integration-" + suffix,
		Name:            "Project Repo Integration",
		PrimaryLanguage: "en",
		GitRemoteURL:    sshRemote,
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	assertProjectRepoRow(t, ctx, tx, out.ID, wantNormalized, sshRemote)
	if got, err := LookupProjectSlugByGitRemoteURL(ctx, tx, httpsRemote); err != nil || got != out.Slug {
		t.Fatalf("LookupProjectSlugByGitRemoteURL = %q, %v; want %q, nil", got, err, out.Slug)
	}
	if _, err := AddProjectRepo(ctx, tx, ProjectRepoInput{
		ProjectID:    out.ID,
		GitRemoteURL: httpsRemote,
	}); err != nil {
		t.Fatalf("AddProjectRepo duplicate normalized remote: %v", err)
	}
	assertProjectRepoCount(t, ctx, tx, out.ID, 1)
	assertProjectRepoIndexUsed(t, ctx, tx)
	assertProjectRepoCascade(t, ctx, tx, out.ID)
}

func TestUpsertProjectRepoIdempotent(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run UpsertProjectRepo idempotency test")
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

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	out, err := CreateProject(ctx, tx, CreateProjectInput{
		Slug:            "upsert-idempotent-" + suffix,
		Name:            "Upsert Idempotent",
		PrimaryLanguage: "en",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	remote := fmt.Sprintf("https://github.com/upsert-it-%s/sample.git", suffix)

	// First call — fresh row, created=true.
	id1, created1, err := UpsertProjectRepo(ctx, tx, ProjectRepoInput{
		ProjectID:    out.ID,
		GitRemoteURL: remote,
		LocalPaths:   []string{"/tmp/upsert-it/checkout"},
	})
	if err != nil {
		t.Fatalf("UpsertProjectRepo first call: %v", err)
	}
	if id1 == "" {
		t.Fatalf("UpsertProjectRepo returned empty id on insert")
	}
	if !created1 {
		t.Fatalf("UpsertProjectRepo created flag = false on first insert, want true")
	}

	// Second call — same key, different local_path. created=false, paths merged.
	id2, created2, err := UpsertProjectRepo(ctx, tx, ProjectRepoInput{
		ProjectID:    out.ID,
		GitRemoteURL: remote,
		LocalPaths:   []string{"/tmp/upsert-it/another-checkout"},
	})
	if err != nil {
		t.Fatalf("UpsertProjectRepo second call: %v", err)
	}
	if id2 != id1 {
		t.Fatalf("UpsertProjectRepo returned different id on update: %q vs %q", id2, id1)
	}
	if created2 {
		t.Fatalf("UpsertProjectRepo created flag = true on second call, want false")
	}

	var localPaths []string
	if err := tx.QueryRow(ctx, `
		SELECT local_paths FROM project_repos WHERE id = $1::uuid
	`, id1).Scan(&localPaths); err != nil {
		t.Fatalf("read local_paths: %v", err)
	}
	if len(localPaths) != 2 {
		t.Fatalf("local_paths = %v, want 2 merged entries", localPaths)
	}
	assertProjectRepoCount(t, ctx, tx, out.ID, 1)
}

func TestUpsertProjectRepoPreservesMetadataOnRefresh(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run UpsertProjectRepo metadata-preservation test")
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

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	out, err := CreateProject(ctx, tx, CreateProjectInput{
		Slug:            "upsert-preserve-" + suffix,
		Name:            "Upsert Preserve",
		PrimaryLanguage: "en",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	remote := fmt.Sprintf("https://github.com/preserve-it-%s/sample.git", suffix)

	// First call sets a custom remote name and non-default branch.
	if _, _, err := UpsertProjectRepo(ctx, tx, ProjectRepoInput{
		ProjectID:     out.ID,
		GitRemoteURL:  remote,
		Name:          "upstream",
		DefaultBranch: "develop",
		LocalPaths:    []string{"/tmp/preserve-it/checkout"},
	}); err != nil {
		t.Fatalf("UpsertProjectRepo first call: %v", err)
	}

	// Second call omits name/default_branch (the "just add a local_path"
	// case). The custom values must survive.
	if _, created, err := UpsertProjectRepo(ctx, tx, ProjectRepoInput{
		ProjectID:    out.ID,
		GitRemoteURL: remote,
		LocalPaths:   []string{"/tmp/preserve-it/another"},
	}); err != nil {
		t.Fatalf("UpsertProjectRepo second call: %v", err)
	} else if created {
		t.Fatalf("UpsertProjectRepo second call created=true, want false")
	}

	var gotName, gotBranch string
	var gotLocalPaths []string
	if err := tx.QueryRow(ctx, `
		SELECT name, default_branch, local_paths
		  FROM project_repos
		 WHERE project_id = $1::uuid
	`, out.ID).Scan(&gotName, &gotBranch, &gotLocalPaths); err != nil {
		t.Fatalf("select project repo: %v", err)
	}
	if gotName != "upstream" {
		t.Fatalf("name = %q after omitted-name refresh, want %q (preserved)", gotName, "upstream")
	}
	if gotBranch != "develop" {
		t.Fatalf("default_branch = %q after omitted-branch refresh, want %q (preserved)", gotBranch, "develop")
	}
	if len(gotLocalPaths) != 2 {
		t.Fatalf("local_paths = %v, want 2 merged entries", gotLocalPaths)
	}
}

func TestUpsertProjectRepoScrubsCredentials(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run UpsertProjectRepo credential-scrub test")
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

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	out, err := CreateProject(ctx, tx, CreateProjectInput{
		Slug:            "upsert-scrub-" + suffix,
		Name:            "Upsert Scrub",
		PrimaryLanguage: "en",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	secret := "ghp_should_not_be_persisted"
	remoteWithToken := fmt.Sprintf("https://x-access-token:%s@github.com/scrub-it-%s/sample.git", secret, suffix)
	if _, _, err := UpsertProjectRepo(ctx, tx, ProjectRepoInput{
		ProjectID:    out.ID,
		GitRemoteURL: remoteWithToken,
	}); err != nil {
		t.Fatalf("UpsertProjectRepo with token remote: %v", err)
	}

	var gotOriginal string
	var gotURLs []string
	if err := tx.QueryRow(ctx, `
		SELECT git_remote_url_original, urls
		  FROM project_repos
		 WHERE project_id = $1::uuid
	`, out.ID).Scan(&gotOriginal, &gotURLs); err != nil {
		t.Fatalf("select project repo: %v", err)
	}
	if strings.Contains(gotOriginal, secret) {
		t.Fatalf("git_remote_url_original leaked credential: %q", gotOriginal)
	}
	for _, u := range gotURLs {
		if strings.Contains(u, secret) {
			t.Fatalf("urls leaked credential: %q", u)
		}
	}
}

func assertProjectRepoRow(t *testing.T, ctx context.Context, tx pgx.Tx, projectID, wantRemote, wantOriginal string) {
	t.Helper()
	var gotRemote, gotOriginal, gotName, gotBranch string
	var gotLocalPaths []string
	if err := tx.QueryRow(ctx, `
		SELECT git_remote_url, git_remote_url_original, name, default_branch, local_paths
		  FROM project_repos
		 WHERE project_id = $1::uuid
	`, projectID).Scan(&gotRemote, &gotOriginal, &gotName, &gotBranch, &gotLocalPaths); err != nil {
		t.Fatalf("select project repo: %v", err)
	}
	if gotRemote != wantRemote || gotOriginal != wantOriginal || gotName != "origin" || gotBranch != "main" {
		t.Fatalf("project repo = remote=%q original=%q name=%q branch=%q", gotRemote, gotOriginal, gotName, gotBranch)
	}
	if gotLocalPaths == nil || len(gotLocalPaths) != 0 {
		t.Fatalf("project repo local_paths = %#v, want empty non-NULL array", gotLocalPaths)
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
