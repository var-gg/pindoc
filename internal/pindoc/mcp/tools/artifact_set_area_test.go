package tools

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestArtifactSetAreaTargetPolicy(t *testing.T) {
	tests := []struct {
		name string
		area setAreaInfo
		want string
	}{
		{name: "misc top-level allowed", area: setAreaInfo{ID: "a", Slug: "misc"}, want: ""},
		{name: "unsorted top-level allowed", area: setAreaInfo{ID: "a", Slug: "_unsorted"}, want: ""},
		{name: "fixed top-level protected", area: setAreaInfo{ID: "a", Slug: "content"}, want: "AREA_TOP_LEVEL_PROTECTED"},
		{name: "depth one child allowed", area: setAreaInfo{ID: "a", Slug: "character-lore", ParentID: "p", ParentSlug: "content"}, want: ""},
		{name: "grandchild rejected", area: setAreaInfo{ID: "a", Slug: "too-deep", ParentID: "p", ParentSlug: "character-lore", GrandparentID: "g"}, want: "AREA_DEPTH_VIOLATION"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateSetAreaTarget(tc.area); got != tc.want {
				t.Fatalf("validateSetAreaTarget() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestArtifactSetAreaWarningHelpers(t *testing.T) {
	warnings := areaSlugIgnoredWarnings("character-lore", "content")
	if len(warnings) != 1 || !strings.HasPrefix(warnings[0], "AREA_SLUG_IGNORED:") {
		t.Fatalf("areaSlugIgnoredWarnings = %v", warnings)
	}
	if warningSeverity(warnings[0]) != SeverityWarn {
		t.Fatalf("AREA_SLUG_IGNORED severity = %s", warningSeverity(warnings[0]))
	}
	actions := areaSlugIgnoredSuggestedActions(warnings)
	if len(actions) != 1 || !strings.Contains(actions[0], "pindoc.artifact.set_area") {
		t.Fatalf("areaSlugIgnoredSuggestedActions = %v", actions)
	}
	if got := areaSlugIgnoredWarnings("content", "content"); len(got) != 0 {
		t.Fatalf("same-area warning = %v", got)
	}
}

func TestMCPArtifactSetAreaSingleIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "content", "character-lore")

	out := callVisibilityTool[artifactSetAreaOutput](t, ctx, pool, nil, owner, "pindoc.artifact.set_area", map[string]any{
		"project_slug":     fixture.projectSlug,
		"slug_or_id":       "vis-public",
		"area_slug":        "character-lore",
		"expected_version": 2,
		"reason":           "dogfood content split",
		"author_id":        "agent:codex",
	})
	if out.Status != "ok" || out.Affected != 1 || out.RevisionNumber != 3 || out.FromAreaSlug != "misc" || out.AreaSlug != "character-lore" {
		t.Fatalf("set_area single output = %+v", out)
	}
	assertArtifactAreaSlug(t, ctx, pool, fixture.projectID, "vis-public", "character-lore")

	var commitMsg, payloadKind, fromArea, toArea string
	if err := pool.QueryRow(ctx, `
		SELECT commit_msg,
		       shape_payload->>'kind',
		       shape_payload#>>'{area_slug,from}',
		       shape_payload#>>'{area_slug,to}'
		  FROM artifact_revisions r
		  JOIN artifacts a ON a.id = r.artifact_id
		 WHERE a.project_id = $1::uuid
		   AND a.slug = 'vis-public'
		   AND r.revision_number = 3
	`, fixture.projectID).Scan(&commitMsg, &payloadKind, &fromArea, &toArea); err != nil {
		t.Fatalf("select area revision: %v", err)
	}
	if commitMsg != "set_area: dogfood content split" || payloadKind != "area_change" || fromArea != "misc" || toArea != "character-lore" {
		t.Fatalf("area revision payload = commit=%q kind=%q from=%q to=%q", commitMsg, payloadKind, fromArea, toArea)
	}
	var eventCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM events
		 WHERE project_id = $1::uuid
		   AND kind = 'artifact.area_changed'
		   AND payload->>'from_area_slug' = 'misc'
		   AND payload->>'to_area_slug' = 'character-lore'
	`, fixture.projectID).Scan(&eventCount); err != nil {
		t.Fatalf("select area changed event: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("artifact.area_changed event count = %d, want 1", eventCount)
	}

	areas := callVisibilityTool[areaListOutput](t, ctx, pool, nil, owner, "pindoc.area.list", map[string]any{
		"project_slug": fixture.projectSlug,
	})
	if count := areaListCountBySlug(areas, "character-lore"); count != 1 {
		t.Fatalf("area.list character-lore count = %d, want 1", count)
	}
}

func TestMCPArtifactSetAreaBulkIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	fromAreaID := insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "content", "narrative-script")
	insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "content", "art-pipeline")
	insertMCPVisibilityArtifact(t, ctx, pool, fixture.projectID, fromAreaID, "area-bulk-one", projects.VisibilityOrg, fixture.ownerUserID, "ko")
	insertMCPVisibilityArtifact(t, ctx, pool, fixture.projectID, fromAreaID, "area-bulk-two", projects.VisibilityOrg, fixture.ownerUserID, "ko")
	insertMCPVisibilityArtifact(t, ctx, pool, fixture.projectID, fromAreaID, "area-bulk-superseded", projects.VisibilityOrg, fixture.ownerUserID, "ko")
	setArtifactStatus(t, ctx, pool, fixture.projectID, "area-bulk-superseded", "superseded")

	dryRun := callVisibilityTool[artifactSetAreaOutput](t, ctx, pool, nil, owner, "pindoc.artifact.set_area", map[string]any{
		"project_slug":        fixture.projectSlug,
		"bulk_all_in_project": true,
		"from_area_slug":      "narrative-script",
		"area_slug":           "art-pipeline",
		"reason":              "dogfood bulk split",
	})
	if dryRun.Status != "informational" || dryRun.Code != "BULK_CONFIRM_REQUIRED" || dryRun.WouldAffect != 2 || dryRun.Affected != 0 {
		t.Fatalf("set_area bulk dry-run = %+v", dryRun)
	}
	assertArtifactAreaSlug(t, ctx, pool, fixture.projectID, "area-bulk-one", "narrative-script")

	confirmed := callVisibilityTool[artifactSetAreaOutput](t, ctx, pool, nil, owner, "pindoc.artifact.set_area", map[string]any{
		"project_slug":        fixture.projectSlug,
		"bulk_all_in_project": true,
		"from_area_slug":      "narrative-script",
		"area_slug":           "art-pipeline",
		"confirm":             true,
		"reason":              "dogfood bulk split",
	})
	if confirmed.Status != "ok" || confirmed.Affected != 2 || confirmed.FromAreaSlug != "narrative-script" || confirmed.AreaSlug != "art-pipeline" {
		t.Fatalf("set_area bulk confirmed = %+v", confirmed)
	}
	assertArtifactAreaSlug(t, ctx, pool, fixture.projectID, "area-bulk-one", "art-pipeline")
	assertArtifactAreaSlug(t, ctx, pool, fixture.projectID, "area-bulk-two", "art-pipeline")
	assertArtifactAreaSlug(t, ctx, pool, fixture.projectID, "area-bulk-superseded", "narrative-script")
}

func TestMCPArtifactSetAreaRejectsIntegration(t *testing.T) {
	ctx, pool, fixture, owner := setupSetAreaIntegration(t)

	childID := insertSetAreaSubArea(t, ctx, pool, fixture.projectID, "content", "narrative-process")
	insertSetAreaGrandchild(t, ctx, pool, fixture.projectID, childID, "too-deep")
	insertMCPVisibilityArtifact(t, ctx, pool, fixture.projectID, childID, "area-superseded", projects.VisibilityOrg, fixture.ownerUserID, "ko")
	setArtifactStatus(t, ctx, pool, fixture.projectID, "area-superseded", "superseded")

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "missing target area",
			args: map[string]any{
				"project_slug": fixture.projectSlug,
				"slug_or_id":   "vis-public",
				"area_slug":    "does-not-exist",
				"reason":       "reject missing area",
			},
			want: "AREA_NOT_FOUND",
		},
		{
			name: "same target area",
			args: map[string]any{
				"project_slug": fixture.projectSlug,
				"slug_or_id":   "vis-public",
				"area_slug":    "misc",
				"reason":       "reject unchanged area",
			},
			want: "AREA_UNCHANGED",
		},
		{
			name: "protected top-level",
			args: map[string]any{
				"project_slug": fixture.projectSlug,
				"slug_or_id":   "vis-public",
				"area_slug":    "content",
				"reason":       "reject top-level area",
			},
			want: "AREA_TOP_LEVEL_PROTECTED",
		},
		{
			name: "depth violation",
			args: map[string]any{
				"project_slug": fixture.projectSlug,
				"slug_or_id":   "vis-public",
				"area_slug":    "too-deep",
				"reason":       "reject depth violation",
			},
			want: "AREA_DEPTH_VIOLATION",
		},
		{
			name: "superseded artifact",
			args: map[string]any{
				"project_slug": fixture.projectSlug,
				"slug_or_id":   "area-superseded",
				"area_slug":    "misc",
				"reason":       "reject superseded artifact",
			},
			want: "ARTIFACT_STATUS_NOT_ACTIVE",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := callVisibilityTool[artifactSetAreaOutput](t, ctx, pool, nil, owner, "pindoc.artifact.set_area", tc.args)
			if out.Status != "not_ready" || out.ErrorCode != tc.want {
				t.Fatalf("set_area reject = %+v, want error_code=%s", out, tc.want)
			}
		})
	}
}

func setupSetAreaIntegration(t *testing.T) (context.Context, *db.Pool, mcpVisibilityFixture, *auth.Principal) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run MCP artifact set_area DB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	fixture := seedMCPVisibilityFixture(t, ctx, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, fixture.projectID)
	})
	owner := &auth.Principal{UserID: fixture.ownerUserID, AgentID: "agent:set-area-test", Source: auth.SourceOAuth}
	return ctx, pool, fixture, owner
}

func insertSetAreaSubArea(t *testing.T, ctx context.Context, pool *db.Pool, projectID, parentSlug, slug string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO areas (project_id, parent_id, slug, name, description)
		SELECT $1::uuid, parent.id, $2, $3, $4
		  FROM areas parent
		 WHERE parent.project_id = $1::uuid
		   AND parent.slug = $5
		RETURNING id::text
	`, projectID, slug, strings.Title(strings.ReplaceAll(slug, "-", " ")), "set_area test area "+slug, parentSlug).Scan(&id); err != nil {
		t.Fatalf("insert sub-area %s: %v", slug, err)
	}
	return id
}

func insertSetAreaGrandchild(t *testing.T, ctx context.Context, pool *db.Pool, projectID, parentID, slug string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO areas (project_id, parent_id, slug, name, description)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5)
		RETURNING id::text
	`, projectID, parentID, slug, strings.Title(strings.ReplaceAll(slug, "-", " ")), "set_area test grandchild").Scan(&id); err != nil {
		t.Fatalf("insert grandchild area %s: %v", slug, err)
	}
	return id
}

func setArtifactStatus(t *testing.T, ctx context.Context, pool *db.Pool, projectID, slug, status string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		UPDATE artifacts
		   SET status = $3
		 WHERE project_id = $1::uuid
		   AND slug = $2
	`, projectID, slug, status); err != nil {
		t.Fatalf("set artifact status %s=%s: %v", slug, status, err)
	}
}

func assertArtifactAreaSlug(t *testing.T, ctx context.Context, pool *db.Pool, projectID, slug, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `
		SELECT ar.slug
		  FROM artifacts a
		  JOIN areas ar ON ar.id = a.area_id
		 WHERE a.project_id = $1::uuid
		   AND a.slug = $2
	`, projectID, slug).Scan(&got); err != nil {
		t.Fatalf("select artifact area %s: %v", slug, err)
	}
	if got != want {
		t.Fatalf("%s area_slug = %q, want %q", slug, got, want)
	}
}

func areaListCountBySlug(out areaListOutput, slug string) int {
	for _, area := range out.Areas {
		if area.Slug == slug {
			return area.ArtifactCount
		}
	}
	return -1
}
