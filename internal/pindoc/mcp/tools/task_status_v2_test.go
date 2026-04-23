package tools

import (
	"context"
	"strings"
	"testing"
)

// TestCountAcceptanceCheckboxes covers the checklist progress helper used
// by the claimed_done evidence gate (migration 0013). The important shapes
// to get right are: mixed bullet markers (`-`, `*`, `+`), case-insensitive
// fill, and the "no checkboxes at all" case that must stay quiet so Tasks
// without a checklist structure aren't blocked.
func TestCountAcceptanceCheckboxes(t *testing.T) {
	cases := []struct {
		name              string
		body              string
		wantDone          int
		wantTotal         int
		wantBlockedByGate bool // helper assertion: gate fires when total>0 && done!=total
	}{
		{
			name:      "no checkboxes is permissive",
			body:      "## Purpose\nsome narrative\n## Scope\nmore words\n",
			wantDone:  0,
			wantTotal: 0,
		},
		{
			name: "all checked passes",
			body: `## Acceptance criteria
- [x] first item
- [x] second item
- [x] third`,
			wantDone:  3,
			wantTotal: 3,
		},
		{
			name: "partially checked blocks",
			body: `- [x] done
- [ ] still open
- [x] also done`,
			wantDone:          2,
			wantTotal:         3,
			wantBlockedByGate: true,
		},
		{
			name: "mixed bullet markers still counted",
			body: `* [x] star
+ [ ] plus
- [x] dash`,
			wantDone:          2,
			wantTotal:         3,
			wantBlockedByGate: true,
		},
		{
			name: "uppercase X accepted as done",
			body: `- [X] upper
- [x] lower`,
			wantDone:  2,
			wantTotal: 2,
		},
		{
			name: "non-checkbox bullets ignored",
			body: `- just a bullet
- [ ] real checkbox
- [x] another`,
			wantDone:          1,
			wantTotal:         2,
			wantBlockedByGate: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done, total := countAcceptanceCheckboxes(tc.body)
			if done != tc.wantDone || total != tc.wantTotal {
				t.Fatalf("done/total got=%d/%d want=%d/%d", done, total, tc.wantDone, tc.wantTotal)
			}
			blocked := total > 0 && done != total
			if blocked != tc.wantBlockedByGate {
				t.Fatalf("gate behaviour got=%v want=%v", blocked, tc.wantBlockedByGate)
			}
		})
	}
}

// TestPreflightTaskStatusV2Transitions covers the three new status-related
// preflight rules (migration 0013):
//   - task_meta.status='verified' via artifact.propose → rejected
//     (VER_VIA_VERIFY_TOOL_ONLY)
//   - task_meta.status='claimed_done' with unchecked acceptance boxes →
//     rejected (CLAIMED_DONE_INCOMPLETE)
//   - task_meta.status='claimed_done' with all boxes checked → clean
func TestPreflightTaskStatusV2Transitions(t *testing.T) {
	baseBodyChecked := `## Purpose
mark complete
## Acceptance criteria
- [x] step one
- [x] step two`

	baseBodyUnchecked := `## Purpose
mark complete
## Acceptance criteria
- [x] step one
- [ ] step two`

	t.Run("verified via propose is rejected", func(t *testing.T) {
		in := artifactProposeInput{
			Type:         "Task",
			Title:        "t",
			BodyMarkdown: baseBodyChecked,
			AreaSlug:     "misc",
			AuthorID:     "test-agent",
			TaskMeta:     &TaskMetaInput{Status: "verified"},
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if !containsCode(failed, "VER_VIA_VERIFY_TOOL_ONLY") {
			t.Fatalf("expected VER_VIA_VERIFY_TOOL_ONLY in failed=%v", failed)
		}
	})

	t.Run("claimed_done with unchecked boxes is rejected", func(t *testing.T) {
		in := artifactProposeInput{
			Type:         "Task",
			Title:        "t",
			BodyMarkdown: baseBodyUnchecked,
			AreaSlug:     "misc",
			AuthorID:     "test-agent",
			TaskMeta:     &TaskMetaInput{Status: "claimed_done"},
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if !containsCode(failed, "CLAIMED_DONE_INCOMPLETE") {
			t.Fatalf("expected CLAIMED_DONE_INCOMPLETE in failed=%v", failed)
		}
	})

	t.Run("claimed_done with all boxes checked passes status gate", func(t *testing.T) {
		in := artifactProposeInput{
			Type:         "Task",
			Title:        "t",
			BodyMarkdown: baseBodyChecked,
			AreaSlug:     "misc",
			AuthorID:     "test-agent",
			TaskMeta:     &TaskMetaInput{Status: "claimed_done"},
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if containsCode(failed, "CLAIMED_DONE_INCOMPLETE") || containsCode(failed, "VER_VIA_VERIFY_TOOL_ONLY") {
			t.Fatalf("claimed_done with complete checkboxes should pass status gates, got failed=%v", failed)
		}
	})

	t.Run("legacy 'done' string is rejected by enum", func(t *testing.T) {
		// Migration 0013 retired 'done' in favour of claimed_done/verified.
		// preflight should trip TASK_STATUS_INVALID so clients noticing the
		// error update their strings.
		in := artifactProposeInput{
			Type:         "Task",
			Title:        "t",
			BodyMarkdown: baseBodyChecked,
			AreaSlug:     "misc",
			AuthorID:     "test-agent",
			TaskMeta:     &TaskMetaInput{Status: "done"},
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if !containsCode(failed, "TASK_STATUS_INVALID") {
			t.Fatalf("expected TASK_STATUS_INVALID for legacy 'done', got %v", failed)
		}
	})
}

// TestPreflightVerificationReport asserts the verdict-keyword rule fires
// when the VerificationReport body does not explicitly declare pass /
// partial / fail (or Korean equivalents). Without the verdict a downstream
// verify tool cannot parse the result.
func TestPreflightVerificationReport(t *testing.T) {
	t.Run("body with no verdict is rejected", func(t *testing.T) {
		in := artifactProposeInput{
			Type:         "VerificationReport",
			Title:        "verify report",
			BodyMarkdown: "## Evidence\nlooked at some code",
			AreaSlug:     "misc",
			AuthorID:     "verifier",
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if !containsCode(failed, "VER_NO_VERDICT") {
			t.Fatalf("expected VER_NO_VERDICT, got %v", failed)
		}
	})

	t.Run("body with pass verdict passes", func(t *testing.T) {
		in := artifactProposeInput{
			Type:         "VerificationReport",
			Title:        "verify report",
			BodyMarkdown: "## Verdict\npass\n",
			AreaSlug:     "misc",
			AuthorID:     "verifier",
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if containsCode(failed, "VER_NO_VERDICT") {
			t.Fatalf("verdict keyword present but gate fired: %v", failed)
		}
	})

	t.Run("korean verdict keyword accepted", func(t *testing.T) {
		in := artifactProposeInput{
			Type:         "VerificationReport",
			Title:        "verify report",
			BodyMarkdown: "## 판정\n합격",
			AreaSlug:     "misc",
			AuthorID:     "verifier",
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if containsCode(failed, "VER_NO_VERDICT") {
			t.Fatalf("korean verdict present but gate fired: %v", failed)
		}
	})
}

func containsCode(list []string, code string) bool {
	for _, c := range list {
		if strings.EqualFold(c, code) {
			return true
		}
	}
	return false
}
