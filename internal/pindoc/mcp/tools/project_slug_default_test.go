package tools

import (
	"context"
	"strings"
	"testing"
)

func TestApplyProjectSlugDefaultingPreservesExplicit(t *testing.T) {
	in := struct {
		ProjectSlug string `json:"project_slug,omitempty"`
	}{ProjectSlug: "explicit"}
	res := applyProjectSlugDefaulting(context.Background(), Deps{DefaultProjectSlug: "env-default"}, nil, &in)
	if in.ProjectSlug != "explicit" {
		t.Fatalf("explicit project_slug overwritten: %q", in.ProjectSlug)
	}
	if res.ProjectSlug != "" {
		t.Fatalf("explicit input should not report defaulting: %+v", res)
	}
}

func TestApplyProjectSlugDefaultingUsesPindocProjectFallback(t *testing.T) {
	in := struct {
		ProjectSlug string `json:"project_slug,omitempty"`
	}{}
	res := applyProjectSlugDefaulting(context.Background(), Deps{DefaultProjectSlug: "pindoc"}, nil, &in)
	if in.ProjectSlug != "pindoc" {
		t.Fatalf("project_slug = %q, want pindoc", in.ProjectSlug)
	}
	if res.Via != projectSlugDefaultEnv {
		t.Fatalf("via = %q, want %q", res.Via, projectSlugDefaultEnv)
	}
}

func TestUniqueProjectSlugDefaultWorkspaceCases(t *testing.T) {
	unique := uniqueProjectSlugDefault([]string{"pindoc"}, "workspace")
	if unique.ProjectSlug != "pindoc" || unique.Via != "workspace" {
		t.Fatalf("unique = %+v", unique)
	}

	ambiguous := uniqueProjectSlugDefault([]string{"vargg", "pindoc"}, "workspace")
	if ambiguous.ProjectSlug != "" || ambiguous.Via != "ambiguous" || strings.Join(ambiguous.Candidates, ",") != "pindoc,vargg" {
		t.Fatalf("ambiguous = %+v", ambiguous)
	}

	missing := uniqueProjectSlugDefault(nil, "workspace")
	if missing.ProjectSlug != "" || missing.Via != "missing" {
		t.Fatalf("missing = %+v", missing)
	}
}

func TestProjectSlugDefaultNotReadyOutput(t *testing.T) {
	out, ok := projectSlugDefaultNotReady[artifactProposeOutput](projectSlugDefaultResult{
		Via:        "ambiguous",
		Candidates: []string{"pindoc", "vargg"},
	})
	if !ok {
		t.Fatal("artifactProposeOutput should support not_ready projection")
	}
	if out.Status != "not_ready" || out.ErrorCode != "PROJECT_SLUG_REQUIRED" {
		t.Fatalf("not_ready status/code = %q/%q", out.Status, out.ErrorCode)
	}
	if len(out.Checklist) != 1 || !strings.Contains(out.Checklist[0], "pindoc, vargg") {
		t.Fatalf("checklist = %+v", out.Checklist)
	}
	if len(out.SuggestedActions) == 0 {
		t.Fatalf("expected suggested action")
	}
}
