package tools

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestSectionDuplicatesEdgesWarnings(t *testing.T) {
	body := `## Purpose

Explain the task.

## Dependencies / 선후

- relates_to -> pindoc://some-task
`
	got := sectionDuplicatesEdgesWarnings(body)
	if len(got) != 1 || got[0] != sectionDuplicatesEdgesWarning {
		t.Fatalf("sectionDuplicatesEdgesWarnings = %v, want [%s]", got, sectionDuplicatesEdgesWarning)
	}
	if severity := warningSeverity(got[0]); severity != SeverityWarn {
		t.Fatalf("warningSeverity(%q) = %q, want %q", got[0], severity, SeverityWarn)
	}
}

func TestSectionDuplicatesEdgesWarningsCleanBody(t *testing.T) {
	body := `## Purpose

Explain the task.

## Scope

Narrative only.
`
	if got := sectionDuplicatesEdgesWarnings(body); len(got) != 0 {
		t.Fatalf("clean body should not warn, got %v", got)
	}
}

func TestSectionDuplicatesEdgesWarningsSkipTemplateArtifacts(t *testing.T) {
	for _, seed := range projects.TemplateSeeds {
		t.Run(seed.Slug, func(t *testing.T) {
			got := sectionDuplicatesEdgesWarningsForArtifact(seed.Slug, seed.Body, ShapeBodyPatch)
			if len(got) != 0 {
				t.Fatalf("template body should not warn for recommended relationship section, got %v", got)
			}
		})
	}
}

func TestSectionDuplicatesEdgesWarningsSkipAcceptanceTransition(t *testing.T) {
	body := `## Purpose

Body.

## 연관

This section predates typed edges.
`
	if got := sectionDuplicatesEdgesWarningsForArtifact("task-with-legacy-section", body, ShapeAcceptanceTransition); len(got) != 0 {
		t.Fatalf("acceptance_transition should not re-emit section duplicate warning, got %v", got)
	}
	if got := sectionDuplicatesEdgesWarningsForArtifact("task-with-legacy-section", body, ShapeBodyPatch); len(got) != 1 || got[0] != sectionDuplicatesEdgesWarning {
		t.Fatalf("body_patch should still warn for normal artifacts, got %v", got)
	}
}

func TestSectionDuplicatesEdgesSuggestedActionIsShort(t *testing.T) {
	actions := sectionDuplicatesEdgesSuggestedActions([]string{sectionDuplicatesEdgesWarning})
	if len(actions) != 1 {
		t.Fatalf("actions = %v, want one action", actions)
	}
	if !strings.Contains(actions[0], "relates_to") {
		t.Fatalf("action should point to relates_to, got %q", actions[0])
	}
	if utf8.RuneCountInString(actions[0]) > 200 {
		t.Fatalf("action too long: %d runes", utf8.RuneCountInString(actions[0]))
	}
}
