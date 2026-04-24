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

// TestApplyTaskCreateDefaults keeps new Task artifacts out of the
// "no status" bucket by injecting the lifecycle baseline only on create /
// supersede-create paths. Explicit caller choices still win.
func TestApplyTaskCreateDefaults(t *testing.T) {
	t.Run("missing task_meta gets open status and default assignee", func(t *testing.T) {
		in := artifactProposeInput{
			Type:     "Task",
			AuthorID: "codex",
		}
		applyTaskCreateDefaults(&in)
		if in.TaskMeta == nil {
			t.Fatal("expected task_meta to be allocated")
		}
		if in.TaskMeta.Status != "open" {
			t.Fatalf("expected status=open, got %q", in.TaskMeta.Status)
		}
		if in.TaskMeta.Assignee != "agent:codex" {
			t.Fatalf("expected assignee=agent:codex, got %q", in.TaskMeta.Assignee)
		}
	})

	t.Run("blank status is normalized without losing other fields", func(t *testing.T) {
		in := artifactProposeInput{
			Type:     "Task",
			AuthorID: "codex",
			TaskMeta: &TaskMetaInput{
				Priority: "p1",
			},
		}
		applyTaskCreateDefaults(&in)
		if in.TaskMeta.Status != "open" {
			t.Fatalf("expected status=open, got %q", in.TaskMeta.Status)
		}
		if in.TaskMeta.Priority != "p1" {
			t.Fatalf("expected priority to survive, got %q", in.TaskMeta.Priority)
		}
		if in.TaskMeta.Assignee != "agent:codex" {
			t.Fatalf("expected default assignee, got %q", in.TaskMeta.Assignee)
		}
	})

	t.Run("explicit status and assignee are preserved", func(t *testing.T) {
		in := artifactProposeInput{
			Type:     "Task",
			AuthorID: "codex",
			TaskMeta: &TaskMetaInput{
				Status:   "blocked",
				Assignee: "user:1234",
			},
		}
		applyTaskCreateDefaults(&in)
		if in.TaskMeta.Status != "blocked" {
			t.Fatalf("explicit status overwritten: %q", in.TaskMeta.Status)
		}
		if in.TaskMeta.Assignee != "user:1234" {
			t.Fatalf("explicit assignee overwritten: %q", in.TaskMeta.Assignee)
		}
	})

	t.Run("update path is left untouched", func(t *testing.T) {
		in := artifactProposeInput{
			Type:     "Task",
			AuthorID: "codex",
			UpdateOf: "existing-task",
		}
		applyTaskCreateDefaults(&in)
		if in.TaskMeta != nil {
			t.Fatalf("expected update path to stay nil, got %+v", in.TaskMeta)
		}
	})
}

func TestHasExplicitMetadataUpdate(t *testing.T) {
	if hasExplicitMetadataUpdate(artifactProposeInput{}) {
		t.Fatal("empty update should not count as metadata change")
	}
	if !hasExplicitMetadataUpdate(artifactProposeInput{TaskMeta: &TaskMetaInput{Status: "claimed_done"}}) {
		t.Fatal("task_meta status transition must count as metadata change")
	}
	if !hasExplicitMetadataUpdate(artifactProposeInput{Completeness: "settled"}) {
		t.Fatal("completeness update must count as metadata change")
	}
	if !hasExplicitMetadataUpdate(artifactProposeInput{Tags: []string{}}) {
		t.Fatal("explicit tags update must count as metadata change")
	}
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
