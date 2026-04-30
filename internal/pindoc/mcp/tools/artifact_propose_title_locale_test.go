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
		{"english with number", "ko", "API v2 rollout", true},
		{"english with symbol", "ja", "Session handoff: retry policy", true},
		{"hangul present", "ko", "세션 종료 signal guidance", false},
		{"han present", "ko", "終了 signal guidance", false},
		{"katakana present", "ja", "セッション signal guidance", false},
		{"hiragana present", "ja", "ひらがな signal guidance", false},
		{"digits only", "ko", "2026-04-30", false},
		{"non ascii non cjk", "ko", "Résumé policy", false},
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
	body := "## Context\nx\n## Decision\ny\n## Rationale\nz\n## Alternatives considered\na\n## Consequences\nb\n"
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
