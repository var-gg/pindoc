package tools

import (
	"context"
	"strings"
	"testing"
)

func TestContextProjectSlugDefaultHintFromWrapperContext(t *testing.T) {
	ctx := withProjectSlugDefaultResult(context.Background(), projectSlugDefaultResult{
		ProjectSlug: "pindoc",
		Via:         projectSlugDefaultEnv,
		Reason:      "project_slug omitted; using PINDOC_PROJECT fallback.",
	})

	got := contextProjectSlugDefaultHint(ctx)
	if got == nil {
		t.Fatal("expected project_slug default hint")
	}
	if got.ProjectSlug != "pindoc" || got.Via != projectSlugDefaultEnv {
		t.Fatalf("project_slug default hint = %+v", got)
	}
}

func TestApplyContextProjectSlugDefaultNoticeAppends(t *testing.T) {
	out := contextForTaskOutput{
		Notice: "stub embedder active",
		ProjectSlugDefault: &ProjectSlugDefaultHint{
			ProjectSlug: "pindoc",
			Via:         projectSlugDefaultEnv,
		},
	}
	applyContextProjectSlugDefaultNotice(&out)
	for _, want := range []string{"stub embedder active", "project_slug omitted", "project_slug=\"pindoc\"", "Pass project_slug explicitly"} {
		if !strings.Contains(out.Notice, want) {
			t.Fatalf("notice %q missing %q", out.Notice, want)
		}
	}
}
