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

func TestTitleLocaleMismatchWarnings(t *testing.T) {
	cases := []struct {
		name     string
		locale   string
		title    string
		wantWarn bool
	}{
		{"english project", "en", "Implement session termination signal guidance", false},
		{"english only ko project", "ko", "Implement session termination signal guidance", true},
		{"english with non ascii punctuation ko project", "ko", "Task flow lens — cross-project sequence", true},
		{"technical version token", "ko", "API v2 rollout", false},
		{"english with symbol", "ja", "Session handoff: retry policy", true},
		{"hangul present", "ko", "세션 종료 signal guidance", false},
		{"han without hangul in ko project", "ko", "終了 signal guidance", true},
		{"katakana present", "ja", "セッション signal guidance", false},
		{"hiragana present", "ja", "ひらがな signal guidance", false},
		{"han present in ja project", "ja", "終了 signal guidance", false},
		{"mixed ko dev terms", "ko", "MCP Task 흐름 lens — task.flow + task.next", false},
		{"technical product name English title", "ko", "Combat Sandbox Inspector closure", false},
		{"generic English title-case verb still warns", "ko", "Implement Session Termination Signal Guidance", true},
		{"digits only", "ko", "2026-04-30", false},
		{"latin accented without anchor", "ko", "Résumé policy", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := titleLocaleMismatchWarnings(tc.locale, tc.title)
			if (len(got) > 0) != tc.wantWarn {
				t.Fatalf("titleLocaleMismatchWarnings(%q, %q) = %v, want warning=%v", tc.locale, tc.title, got, tc.wantWarn)
			}
			if tc.wantWarn && got[0] != titleLocaleMismatchWarning {
				t.Fatalf("warning = %v, want %q", got, titleLocaleMismatchWarning)
			}
		})
	}
}

func TestTitleLocaleMismatchSuggestedActions(t *testing.T) {
	actions := titleLocaleMismatchSuggestedActions([]string{titleLocaleMismatchWarning})
	joined := strings.Join(actions, "\n")
	for _, want := range []string{"pindoc.artifact.propose", "title", "expected_version", "pindoc.artifact.wording_fix"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("suggested action missing %q: %q", want, joined)
		}
	}
}

func TestArtifactProposeTitleLocaleMismatchIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run title locale warning integration")
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
	projectSlug := fmt.Sprintf("title-locale-%d", suffix)
	areaSlug := "mcp"
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	insertContextReceiptArea(t, ctx, pool, projectID, areaSlug)
	if _, err := pool.Exec(ctx, `UPDATE projects SET primary_language = 'ko' WHERE id = $1::uuid`, projectID); err != nil {
		t.Fatalf("set project primary_language: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	call := newArtifactProposeTestCaller(t, ctx, pool, nil)
	slug := fmt.Sprintf("title-locale-%d", suffix)
	body := validDecisionBodyForPropose("x", "y")
	created := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     areaSlug,
		"type":          "Decision",
		"title":         "Implement session termination signal guidance",
		"slug":          slug,
		"body_markdown": body,
		"author_id":     "codex-test",
	})
	if created.Status != "accepted" || !containsString(created.Warnings, titleLocaleMismatchWarning) {
		t.Fatalf("create warnings = status=%q warnings=%v", created.Status, created.Warnings)
	}
	if !strings.Contains(strings.Join(created.SuggestedActions, "\n"), "pindoc.artifact.wording_fix") {
		t.Fatalf("create suggested_actions missing wording_fix guidance: %v", created.SuggestedActions)
	}

	updated := call(ctx, map[string]any{
		"project_slug":     projectSlug,
		"area_slug":        areaSlug,
		"type":             "Decision",
		"title":            "Implement session drift signal guidance",
		"update_of":        slug,
		"expected_version": 1,
		"commit_msg":       "title locale warning update path",
		"body_markdown":    body,
		"author_id":        "codex-test",
	})
	if updated.Status != "accepted" || !containsString(updated.Warnings, titleLocaleMismatchWarning) {
		t.Fatalf("update warnings = status=%q warnings=%v", updated.Status, updated.Warnings)
	}
}

func TestArtifactProposeBodyLocaleSafeSubsetIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run body locale integration")
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
	projectSlug := fmt.Sprintf("body-locale-%d", suffix)
	areaSlug := "mcp"
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	insertContextReceiptArea(t, ctx, pool, projectID, areaSlug)
	if _, err := pool.Exec(ctx, `UPDATE projects SET primary_language = 'ko' WHERE id = $1::uuid`, projectID); err != nil {
		t.Fatalf("set project primary_language: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	call := newArtifactProposeTestCaller(t, ctx, pool, nil)
	body := validDecisionBodyForPropose("x", "y")
	invalidCreate := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     areaSlug,
		"type":          "Decision",
		"title":         "본문 locale invalid create",
		"slug":          fmt.Sprintf("body-locale-invalid-%d", suffix),
		"body_markdown": body,
		"body_locale":   "fr-FR",
		"author_id":     "codex-test",
	})
	if invalidCreate.Status != "not_ready" || invalidCreate.ErrorCode != "BODY_LOCALE_INVALID" {
		t.Fatalf("invalid create = status=%q code=%q checklist=%v", invalidCreate.Status, invalidCreate.ErrorCode, invalidCreate.Checklist)
	}

	slug := fmt.Sprintf("body-locale-valid-%d", suffix)
	created := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     areaSlug,
		"type":          "Decision",
		"title":         "본문 locale default 유지",
		"slug":          slug,
		"body_markdown": body,
		"author_id":     "codex-test",
	})
	if created.Status != "accepted" {
		t.Fatalf("valid create = status=%q code=%q", created.Status, created.ErrorCode)
	}
	var storedLocale string
	if err := pool.QueryRow(ctx, `SELECT body_locale FROM artifacts WHERE project_id = $1::uuid AND slug = $2`, projectID, slug).Scan(&storedLocale); err != nil {
		t.Fatalf("read stored body_locale: %v", err)
	}
	if storedLocale != "ko" {
		t.Fatalf("stored body_locale = %q; want project primary_language default ko", storedLocale)
	}

	invalidUpdate := call(ctx, map[string]any{
		"project_slug":     projectSlug,
		"area_slug":        areaSlug,
		"type":             "Decision",
		"title":            "본문 locale invalid update",
		"update_of":        slug,
		"expected_version": 1,
		"commit_msg":       "invalid body locale update path",
		"body_markdown":    body,
		"body_locale":      "zh-Hans",
		"author_id":        "codex-test",
	})
	if invalidUpdate.Status != "not_ready" || invalidUpdate.ErrorCode != "BODY_LOCALE_INVALID" {
		t.Fatalf("invalid update = status=%q code=%q checklist=%v", invalidUpdate.Status, invalidUpdate.ErrorCode, invalidUpdate.Checklist)
	}
}
