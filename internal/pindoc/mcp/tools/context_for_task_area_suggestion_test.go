package tools

import (
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

func TestSuggestAreasForTaskDescriptionLiteratureReview(t *testing.T) {
	counts := map[string]int{
		"literature": 0,
		"context":    0,
		"mcp":        3,
	}

	got := suggestAreasForTaskDescription("Prepare a literature review of agent memory papers", nil, counts)
	if len(got) == 0 {
		t.Fatalf("expected at least one suggestion")
	}
	if got[0].AreaSlug != "literature" {
		t.Fatalf("top suggestion got=%q want literature; all=%v", got[0].AreaSlug, got)
	}
	if got[0].Score < 0.50 {
		t.Fatalf("expected confident suggestion, got score=%f", got[0].Score)
	}
}

func TestSuggestAreasForTaskDescriptionLowConfidenceEmpty(t *testing.T) {
	counts := map[string]int{
		"literature": 0,
		"mcp":        0,
		"ui":         0,
	}

	got := suggestAreasForTaskDescription("tidy the thing later", nil, counts)
	if len(got) != 0 {
		t.Fatalf("low-confidence input should omit suggestions, got %v", got)
	}
}

func TestBuildSourceSessionRefIncludesSearchReceipt(t *testing.T) {
	ref := buildSourceSessionRef(&auth.Principal{AgentID: "agent-1"}, artifactProposeInput{
		AuthorID: "codex",
		Basis:    &artifactProposeBasis{SearchReceipt: "sr_123"},
	})
	got, ok := ref.(string)
	if !ok {
		t.Fatalf("expected JSON string source_session_ref, got %#v", ref)
	}
	if !strings.Contains(got, "sr_123") || !strings.Contains(got, "search_receipt") {
		t.Fatalf("source_session_ref missing receipt: %s", got)
	}
}
