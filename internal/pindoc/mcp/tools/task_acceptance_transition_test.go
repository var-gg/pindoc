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

func TestResolveAcceptanceLabelMatchesAll(t *testing.T) {
	body := "- [~] QA smoke pass\n- [~] QA regression pass\n- [ ] deploy verified\n- [x] code merged\n"

	indices, matches, _, code := resolveAcceptanceLabelMatchesAll(body, "QA")
	if code != "" {
		t.Fatalf("match_all QA should resolve, got code %q", code)
	}
	if len(indices) != 2 || len(matches) != 2 {
		t.Fatalf("expected 2 QA matches, got indices=%v matches=%d", indices, len(matches))
	}
	// Only unresolved ([ ]/[~]) checkboxes are eligible — the [x] item
	// (index 3) is never selected.
	for _, idx := range indices {
		if idx == 3 {
			t.Fatal("resolved [x] checkbox must not be selected")
		}
	}

	if _, _, _, code := resolveAcceptanceLabelMatchesAll(body, "nonexistent label"); code != "ACCEPTANCE_LABEL_NOT_FOUND" {
		t.Fatalf("no match should yield ACCEPTANCE_LABEL_NOT_FOUND, got %q", code)
	}

	// The single-match path still enforces exactly-one (ambiguous on >1),
	// so match_all is the opt-in for multi-resolution.
	if _, _, _, code := resolveAcceptanceLabelMatch(body, "QA"); code != "ACCEPTANCE_LABEL_AMBIGUOUS" {
		t.Fatalf("single-match QA should stay ambiguous, got %q", code)
	}
}

func TestPartialAcceptanceLabels(t *testing.T) {
	body := "- [x] done\n- [~] manual QA partial\n- [ ] still open\n- [-] deferred\n- [~] perf check\n"
	got := partialAcceptanceLabels(body)
	if len(got) != 2 {
		t.Fatalf("expected 2 partial [~] items, got %d (%+v)", len(got), got)
	}
	for _, l := range got {
		if l.State != "[~]" {
			t.Fatalf("partialAcceptanceLabels returned non-partial state %q", l.State)
		}
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

func TestAcceptanceLabelRefsAndFuzzyMatch(t *testing.T) {
	body := strings.Join([]string{
		"## Acceptance",
		"- [ ] DevTI 검증",
		"  - [~] QA 통과",
		"- [x] done item",
	}, "\n")
	labels := unresolvedAcceptanceLabels(body)
	if len(labels) != 2 {
		t.Fatalf("unresolved labels len = %d, want 2 (%+v)", len(labels), labels)
	}
	if labels[0].Index != 0 || labels[0].State != "[ ]" || labels[0].Label != "DevTI 검증" || labels[0].IndentLevel != 0 {
		t.Fatalf("first label = %+v", labels[0])
	}
	if labels[1].Index != 1 || labels[1].State != "[~]" || labels[1].Label != "QA 통과" || labels[1].IndentLevel != 2 {
		t.Fatalf("second label = %+v", labels[1])
	}
	if !acceptanceLabelMatches("devti", labels[0].Label) || !acceptanceLabelMatches("QA", labels[1].Label) {
		t.Fatalf("expected case-insensitive fuzzy matches for mixed labels")
	}
}

func TestResolveAcceptanceLabelMatchMatrix(t *testing.T) {
	body := strings.Join([]string{
		"- [ ] QA browser",
		"- [~] QA mobile",
		"- [ ] DevTI 검증",
	}, "\n")
	idx, matches, unresolved, code := resolveAcceptanceLabelMatch(body, "DevTI")
	if code != "" || idx != 2 || len(matches) != 1 {
		t.Fatalf("single match = idx %d matches %v code %q", idx, matches, code)
	}
	_, matches, unresolved, code = resolveAcceptanceLabelMatch(body, "QA")
	if code != "ACCEPTANCE_LABEL_AMBIGUOUS" || len(matches) != 2 || len(unresolved) != 3 {
		t.Fatalf("ambiguous = matches %v unresolved %v code %q", matches, unresolved, code)
	}
	_, matches, unresolved, code = resolveAcceptanceLabelMatch(body, "missing")
	if code != "ACCEPTANCE_LABEL_NOT_FOUND" || len(matches) != 0 || len(unresolved) != 3 {
		t.Fatalf("not found = matches %v unresolved %v code %q", matches, unresolved, code)
	}
	if !labelMatchesCheckboxIndex(body, 0, "browser") {
		t.Fatalf("index label verification should match selected checkbox")
	}
	if labelMatchesCheckboxIndex(body, 0, "mobile") {
		t.Fatalf("index label verification should detect mismatch")
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
