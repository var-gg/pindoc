package tools

import (
	"strings"
	"testing"
)

func TestNormalizeTransitionIndices(t *testing.T) {
	one := 2
	got, code := normalizeTransitionIndices(&one, []int{4})
	if code != "" {
		t.Fatalf("unexpected code: %s", code)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Fatalf("indices = %v; want [2 4]", got)
	}

	if _, code := normalizeTransitionIndices(nil, nil); code != "ACCEPT_TRANSITION_INDEX_REQUIRED" {
		t.Fatalf("empty code = %q", code)
	}
	if _, code := normalizeTransitionIndices(nil, []int{1, 1}); code != "ACCEPT_TRANSITION_DUPLICATE_INDEX" {
		t.Fatalf("duplicate code = %q", code)
	}
}

func TestApplyAcceptanceTransitionsBulkSingleRevisionMaterial(t *testing.T) {
	body := strings.Join([]string{
		"## Acceptance",
		"- [ ] first",
		"- [ ] second",
		"- [x] third",
	}, "\n")
	got, applied, code := applyAcceptanceTransitions(body, []int{0, 1}, "[x]", "done together")
	if code != "" {
		t.Fatalf("unexpected code: %s", code)
	}
	if len(applied) != 2 {
		t.Fatalf("applied len = %d; want 2", len(applied))
	}
	if !strings.Contains(got, "- [x] first") || !strings.Contains(got, "- [x] second") {
		t.Fatalf("bulk transition did not rewrite both markers:\n%s", got)
	}
}

func TestShouldAutoClaimDoneOnlyOpenTasks(t *testing.T) {
	body := "- [x] one\n- [-] deferred"
	if !shouldAutoClaimDone("Task", []byte(`{"status":"open"}`), body) {
		t.Fatalf("complete open Task should auto-claim")
	}
	if shouldAutoClaimDone("Task", []byte(`{"status":"blocked"}`), body) {
		t.Fatalf("blocked Task should not auto-claim")
	}
	if shouldAutoClaimDone("Decision", nil, body) {
		t.Fatalf("non-Task should not auto-claim")
	}
	if shouldAutoClaimDone("Task", []byte(`{"status":"open"}`), "- [ ] todo") {
		t.Fatalf("unchecked Task should not auto-claim")
	}
}
