package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/i18n"
)

func TestPreflightDecisionAreaDeprecated(t *testing.T) {
	in := artifactProposeInput{
		Type:         "Decision",
		Title:        "Decision subject area guard",
		BodyMarkdown: "## Context\nwhy\n## Decision\nwhat\n",
		AreaSlug:     "decisions",
		AuthorID:     "test-agent",
	}

	checklist, failed, code := preflight(context.Background(), Deps{}, "", &in, "en")
	if code != "DECISION_AREA_DEPRECATED" {
		t.Fatalf("code got=%q want DECISION_AREA_DEPRECATED; failed=%v checklist=%v", code, failed, checklist)
	}
	if !containsCode(failed, "DECISION_AREA_DEPRECATED") {
		t.Fatalf("expected DECISION_AREA_DEPRECATED in failed=%v", failed)
	}
	if !strings.Contains(strings.Join(checklist, "\n"), "docs/19-area-taxonomy.md") {
		t.Fatalf("expected taxonomy doc link in checklist=%v", checklist)
	}
}

func TestDecisionSubjectAreaWarnings(t *testing.T) {
	in := artifactProposeInput{Type: "Decision", AreaSlug: "_unsorted"}
	got := decisionSubjectAreaWarnings(in)
	if len(got) != 1 || !strings.HasPrefix(got[0], "DECISION_AREA_MUST_BE_SUBJECT") {
		t.Fatalf("expected DECISION_AREA_MUST_BE_SUBJECT warning, got %v", got)
	}
	if warningSeverity(got[0]) != SeverityWarn {
		t.Fatalf("decision area warning should be warn severity, got %s", warningSeverity(got[0]))
	}

	if got := decisionSubjectAreaWarnings(artifactProposeInput{Type: "Decision", AreaSlug: "policies"}); len(got) != 0 {
		t.Fatalf("subject area should not warn, got %v", got)
	}
}

func TestDecisionAreaMessagesStayShortAndActionable(t *testing.T) {
	for _, lang := range []string{"en", "ko"} {
		for _, key := range []string{"preflight.decision_area_deprecated", "preflight.area_not_found"} {
			msg := i18n.T(lang, key)
			if len([]rune(msg)) > 200 {
				t.Fatalf("%s %s message too long: %d runes", lang, key, len([]rune(msg)))
			}
			if !strings.Contains(msg, "docs/19-area-taxonomy.md") {
				t.Fatalf("%s %s message missing taxonomy link: %q", lang, key, msg)
			}
		}
	}
}
