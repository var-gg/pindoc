package samplefixtures

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestLoadManifestAndFixtures(t *testing.T) {
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if manifest.Project.Slug != ProjectSlug {
		t.Fatalf("project slug = %q, want %q", manifest.Project.Slug, ProjectSlug)
	}
	if manifest.Project.Name == "" || manifest.Project.PrimaryLanguage == "" {
		t.Fatalf("sample project metadata should include name and primary_language: %#v", manifest.Project)
	}
	if got := len(manifest.Artifacts); got < 5 || got > 10 {
		t.Fatalf("sample artifact count = %d, want 5-10", got)
	}

	fixtures, err := LoadFixtures()
	if err != nil {
		t.Fatalf("LoadFixtures() error = %v", err)
	}
	if len(fixtures) != len(manifest.Artifacts) {
		t.Fatalf("LoadFixtures() returned %d fixtures, manifest has %d", len(fixtures), len(manifest.Artifacts))
	}
	for _, fixture := range fixtures {
		if fixture.Slug == "" || fixture.Type == "" || fixture.Area == "" || fixture.Title == "" || fixture.File == "" {
			t.Fatalf("fixture metadata must be complete: %#v", fixture.ArtifactMeta)
		}
		if len(fixture.Tags) == 0 {
			t.Fatalf("fixture %q should include tags", fixture.Slug)
		}
		if strings.TrimSpace(fixture.Body) == "" {
			t.Fatalf("fixture %q body should not be empty", fixture.Slug)
		}
	}
}

func TestFixturesAvoidSensitiveStrings(t *testing.T) {
	banned := []string{
		"curioustore",
		"var.gg",
		"vargg",
		"rhkdwls",
		"naver",
		"api_key",
		"api-key",
		"bearer ",
		"authorization:",
		"github_client_secret",
		"oauth_client_secret",
		"begin private key",
	}
	files, err := fixtureFS.ReadDir(".")
	if err != nil {
		t.Fatalf("read fixture FS: %v", err)
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		raw, err := fixtureFS.ReadFile(file.Name())
		if err != nil {
			t.Fatalf("read %s: %v", file.Name(), err)
		}
		lower := strings.ToLower(string(raw))
		for _, term := range banned {
			if strings.Contains(lower, term) {
				t.Fatalf("sample fixture %s contains sensitive marker %q", file.Name(), term)
			}
		}
	}
}

func TestSeedIdempotentIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run sample fixture DB integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("db.Migrate() error = %v", err)
	}

	cleanupSample(t, ctx, pool)
	t.Cleanup(func() { cleanupSample(t, context.Background(), pool) })

	ownerUserID := ensureSampleTestUser(t, ctx, pool)
	fixtures, err := LoadFixtures()
	if err != nil {
		t.Fatalf("LoadFixtures() error = %v", err)
	}

	first, err := Seed(ctx, pool, ownerUserID)
	if err != nil {
		t.Fatalf("Seed() first run error = %v", err)
	}
	if !first.ProjectCreated {
		t.Fatalf("first Seed() should create project: %#v", first)
	}
	if first.ArtifactsInserted != len(fixtures) || first.ArtifactsSkipped != 0 {
		t.Fatalf("first Seed() result = %#v, want %d inserted and 0 skipped", first, len(fixtures))
	}

	second, err := Seed(ctx, pool, ownerUserID)
	if err != nil {
		t.Fatalf("Seed() second run error = %v", err)
	}
	if second.ProjectCreated {
		t.Fatalf("second Seed() should reuse project: %#v", second)
	}
	if second.ArtifactsInserted != 0 || second.ArtifactsSkipped != len(fixtures) {
		t.Fatalf("second Seed() result = %#v, want 0 inserted and %d skipped", second, len(fixtures))
	}

	var (
		ownerID                   string
		orgSlug                   string
		visibility                string
		defaultArtifactVisibility string
	)
	if err := pool.QueryRow(ctx, `
		SELECT p.owner_id, o.slug, p.visibility, p.default_artifact_visibility
		  FROM projects p
		  JOIN organizations o ON o.id = p.organization_id
		 WHERE p.slug = $1
	`, ProjectSlug).Scan(&ownerID, &orgSlug, &visibility, &defaultArtifactVisibility); err != nil {
		t.Fatalf("lookup sample project: %v", err)
	}
	if ownerID != sampleOwnerSlug || orgSlug != sampleOwnerSlug {
		t.Fatalf("sample project owner/org = %q/%q, want %q", ownerID, orgSlug, sampleOwnerSlug)
	}
	if visibility != projects.VisibilityPublic || defaultArtifactVisibility != projects.VisibilityPublic {
		t.Fatalf("sample project visibility = %q/%q, want public", visibility, defaultArtifactVisibility)
	}

	assertSampleArtifactRows(t, ctx, pool, len(fixtures))
}

func cleanupSample(t *testing.T, ctx context.Context, pool *db.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DELETE FROM projects WHERE slug = $1`, ProjectSlug); err != nil {
		t.Fatalf("cleanup sample project: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE slug = $1`, sampleOwnerSlug); err != nil {
		t.Fatalf("cleanup sample organization: %v", err)
	}
}

func ensureSampleTestUser(t *testing.T, ctx context.Context, pool *db.Pool) string {
	t.Helper()
	var userID string
	err := pool.QueryRow(ctx, `
		SELECT id::text FROM users WHERE email = 'sample-fixture-test@example.invalid'
	`).Scan(&userID)
	if err == nil {
		return userID
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("lookup sample test user: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ('Sample Fixture Test', 'sample-fixture-test@example.invalid', 'pindoc_admin')
		RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatalf("ensure sample test user: %v", err)
	}
	return userID
}

func assertSampleArtifactRows(t *testing.T, ctx context.Context, pool *db.Pool, fixtureCount int) {
	t.Helper()
	var artifactCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		  FROM artifacts a
		  JOIN projects p ON p.id = a.project_id
		 WHERE p.slug = $1
		   AND a.tags @> ARRAY['sample']::text[]
		   AND a.visibility = $2
	`, ProjectSlug, projects.VisibilityPublic).Scan(&artifactCount); err != nil {
		t.Fatalf("count sample artifacts: %v", err)
	}
	if artifactCount != fixtureCount {
		t.Fatalf("sample public artifact count = %d, want %d", artifactCount, fixtureCount)
	}

	var revisionCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		  FROM artifact_revisions r
		  JOIN artifacts a ON a.id = r.artifact_id
		  JOIN projects p ON p.id = a.project_id
		 WHERE p.slug = $1
		   AND a.tags @> ARRAY['sample']::text[]
	`, ProjectSlug).Scan(&revisionCount); err != nil {
		t.Fatalf("count sample revisions: %v", err)
	}
	if revisionCount != fixtureCount {
		t.Fatalf("sample revision count = %d, want %d", revisionCount, fixtureCount)
	}

	var metaJSON []byte
	if err := pool.QueryRow(ctx, `
		SELECT artifact_meta
		  FROM artifacts a
		  JOIN projects p ON p.id = a.project_id
		 WHERE p.slug = $1
		   AND a.slug = 'start-here-agent-written-memory'
	`, ProjectSlug).Scan(&metaJSON); err != nil {
		t.Fatalf("read sample artifact meta: %v", err)
	}
	var meta map[string]string
	if err := json.Unmarshal(metaJSON, &meta); err != nil {
		t.Fatalf("decode sample artifact meta: %v", err)
	}
	if meta["source_type"] != "artifact" || meta["verification_state"] != "verified" {
		t.Fatalf("sample artifact meta = %#v, want source_type=artifact and verification_state=verified", meta)
	}
}
