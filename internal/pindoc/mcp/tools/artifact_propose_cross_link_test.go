package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestArtifactProposeNormalizesPindocLinksIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run artifact.propose cross-link integration")
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
	projectSlug := fmt.Sprintf("cross-link-%d", suffix)
	areaSlug := "mcp"
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	areaID := insertContextReceiptArea(t, ctx, pool, projectID, areaSlug)
	var previousPublicBaseURL string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(public_base_url, '') FROM server_settings WHERE id = 1`).Scan(&previousPublicBaseURL); err != nil {
		t.Fatalf("read public_base_url: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE server_settings SET public_base_url = 'https://docs.example.test' WHERE id = 1`); err != nil {
		t.Fatalf("set public_base_url: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE server_settings SET public_base_url = $1 WHERE id = 1`, previousPublicBaseURL)
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	insertDryRunArtifact(t, ctx, pool, projectID, areaID, "target-doc", "Decision", "Target doc", "## Context\nx\n## Decision\ny\n")
	call := newArtifactProposeTestCaller(t, ctx, pool, nil)
	created := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     areaSlug,
		"type":          "Decision",
		"title":         "Cross link create",
		"slug":          fmt.Sprintf("cross-link-create-%d", suffix),
		"body_markdown": validDecisionBodyForPropose("See pindoc://target-doc.", "Store normalized body."),
		"author_id":     "codex-test",
	})
	if created.Status != "accepted" {
		t.Fatalf("create = status=%q code=%q checklist=%v", created.Status, created.ErrorCode, created.Checklist)
	}
	want := "https://docs.example.test/p/" + projectSlug + "/wiki/target-doc"
	var stored string
	if err := pool.QueryRow(ctx, `SELECT body_markdown FROM artifacts WHERE id = $1::uuid`, created.ArtifactID).Scan(&stored); err != nil {
		t.Fatalf("read stored body: %v", err)
	}
	if !strings.Contains(stored, want+".") || strings.Contains(stored, "pindoc://target-doc") {
		t.Fatalf("stored body = %q; want normalized %q", stored, want)
	}

	invalid := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     areaSlug,
		"type":          "Decision",
		"title":         "Cross link invalid",
		"slug":          fmt.Sprintf("cross-link-invalid-%d", suffix),
		"body_markdown": validDecisionBodyForPropose("See pindoc://missing-doc.", "Reject invalid refs."),
		"author_id":     "codex-test",
	})
	if invalid.Status != "not_ready" || invalid.ErrorCode != "PINDOC_LINK_TARGET_NOT_FOUND" {
		t.Fatalf("invalid = status=%q code=%q checklist=%v", invalid.Status, invalid.ErrorCode, invalid.Checklist)
	}
}
