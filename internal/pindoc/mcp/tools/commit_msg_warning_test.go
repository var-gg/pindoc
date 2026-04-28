package tools

import (
	"context"
	"strings"
	"testing"
)

func TestApplyCreateCommitMsgFallback(t *testing.T) {
	in := artifactProposeInput{Title: "Policy draft"}
	warnings := applyCreateCommitMsgFallback(&in)
	if len(warnings) != 1 || warnings[0] != "MISSING_COMMIT_MSG_ON_CREATE" {
		t.Fatalf("warnings = %v", warnings)
	}
	if !strings.Contains(in.CommitMsg, "fallback_missing_commit_msg") {
		t.Fatalf("fallback commit_msg should be intentionally visible, got %q", in.CommitMsg)
	}
	if !strings.Contains(in.CommitMsg, "Policy draft") {
		t.Fatalf("fallback commit_msg should include title, got %q", in.CommitMsg)
	}
}

func TestApplyCreateCommitMsgPreservesExplicitValue(t *testing.T) {
	in := artifactProposeInput{Title: "Policy draft", CommitMsg: "  add policy draft  "}
	warnings := applyCreateCommitMsgFallback(&in)
	if len(warnings) != 0 {
		t.Fatalf("explicit commit_msg should not warn: %v", warnings)
	}
	if in.CommitMsg != "add policy draft" {
		t.Fatalf("commit_msg should be trimmed, got %q", in.CommitMsg)
	}
}

func TestUpdatePathStillRequiresCommitMsg(t *testing.T) {
	_, out, err := handleUpdate(context.Background(), Deps{}, nil, nil, artifactProposeInput{
		Type:         "Decision",
		Title:        "Decision title",
		BodyMarkdown: "## Context\nx\n## Decision\ny\n",
		UpdateOf:     "existing-decision",
	}, "en")
	if err != nil {
		t.Fatalf("handleUpdate returned error: %v", err)
	}
	if out.Status != "not_ready" || out.ErrorCode != "MISSING_COMMIT_MSG" {
		t.Fatalf("expected MISSING_COMMIT_MSG not_ready, got status=%q code=%q", out.Status, out.ErrorCode)
	}
}

func TestAcceptanceUncheckedNudgeWarningsPositiveKeywords(t *testing.T) {
	body := `## Acceptance criteria
- [ ] route alias works
- [ ] fallback renders`

	cases := []string{
		"fix today route dead screen",
		"resolve today route dead screen",
		"close today route dead screen",
		"closes pindoc://today-task-route-missing-dead-screen",
		"Today route 완료",
		"Today route 해결",
	}
	for _, commitMsg := range cases {
		t.Run(commitMsg, func(t *testing.T) {
			got := acceptanceUncheckedNudgeWarnings("Task", body, commitMsg)
			if len(got) != 1 || got[0] != warningAcceptanceUnchecked {
				t.Fatalf("warnings = %v; want [%s]", got, warningAcceptanceUnchecked)
			}
		})
	}
}

func TestAcceptanceUncheckedNudgeWarningsNegativeCases(t *testing.T) {
	bodyUnchecked := `## Acceptance criteria
- [ ] route alias works
- [ ] fallback renders`
	bodyPartiallyChecked := `## Acceptance criteria
- [x] route alias works
- [ ] fallback renders`

	cases := []struct {
		name      string
		typ       string
		body      string
		commitMsg string
	}{
		{
			name:      "no close-suggestive keyword",
			typ:       "Task",
			body:      bodyUnchecked,
			commitMsg: "note progress on route investigation",
		},
		{
			name:      "one acceptance already checked",
			typ:       "Task",
			body:      bodyPartiallyChecked,
			commitMsg: "fix today route dead screen",
		},
		{
			name:      "non task type",
			typ:       "Analysis",
			body:      bodyUnchecked,
			commitMsg: "fix today route dead screen",
		},
		{
			name:      "empty commit message",
			typ:       "Task",
			body:      bodyUnchecked,
			commitMsg: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := acceptanceUncheckedNudgeWarnings(tc.typ, tc.body, tc.commitMsg); len(got) != 0 {
				t.Fatalf("warnings = %v; want none", got)
			}
		})
	}
}
