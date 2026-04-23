package tools

import (
	"strings"
	"testing"
)

// TestIterateCheckboxes covers the 4-state checkbox locator underneath
// both countAcceptanceResolution and applyAcceptanceTransition. The key
// promise is document-order indexing across ' ', 'x', 'X', '~', '-' —
// callers rely on it to keep acceptance_transition and
// body_patch.checkbox_toggle on the same index axis.
func TestIterateCheckboxes(t *testing.T) {
	body := strings.Join([]string{
		"## Acceptance",
		"- [ ] first",
		"- [x] second",
		"- [~] third partial",
		"- [-] fourth deferred",
		"* [X] fifth uppercase star",
		"+ [ ] sixth plus bullet",
		"plain line",
		"  - [x] indented with two spaces",
		"- not a checkbox",
	}, "\n")
	got := iterateCheckboxes(body)
	if len(got) != 7 {
		t.Fatalf("iterateCheckboxes got %d hits, want 7", len(got))
	}
	wantMarkers := []byte{' ', 'x', '~', '-', 'X', ' ', 'x'}
	for i, cb := range got {
		if cb.marker != wantMarkers[i] {
			t.Errorf("hit %d marker=%q want=%q", i, string(cb.marker), string(wantMarkers[i]))
		}
	}
}

// TestCountAcceptanceResolution asserts resolved counts include [x], [~],
// [-] — the Phase D widening from binary to 4-state. Essential for the
// claimed_done gate: only [ ] remains blocking.
func TestCountAcceptanceResolution(t *testing.T) {
	cases := []struct {
		name          string
		body          string
		wantResolved  int
		wantTotal     int
		wantUnchecked int
	}{
		{
			name: "four-state mixture",
			body: `- [ ] todo
- [x] done
- [~] partial
- [-] deferred`,
			wantResolved:  3,
			wantTotal:     4,
			wantUnchecked: 1,
		},
		{
			name:         "only deferred counts as resolved",
			body:         "- [-] moved to task-X",
			wantResolved: 1, wantTotal: 1,
		},
		{
			name: "binary legacy body stays backwards-compatible",
			body: `- [x] a
- [x] b`,
			wantResolved: 2, wantTotal: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, total := countAcceptanceResolution(tc.body)
			if r != tc.wantResolved || total != tc.wantTotal {
				t.Fatalf("resolved/total got=%d/%d want=%d/%d", r, total, tc.wantResolved, tc.wantTotal)
			}
			if tc.wantUnchecked != 0 && total-r != tc.wantUnchecked {
				t.Fatalf("unchecked got=%d want=%d", total-r, tc.wantUnchecked)
			}
		})
	}
}

// TestParseAcceptanceState asserts the 4 legal states + rejection of
// unknown markers. Case-sensitive on 'x' — handler canonicalises to
// lowercase when writing back so body stays consistent.
func TestParseAcceptanceState(t *testing.T) {
	cases := []struct {
		in     string
		wantOK bool
		wantM  byte
	}{
		{"[ ]", true, ' '},
		{"[x]", true, 'x'},
		{"[~]", true, '~'},
		{"[-]", true, '-'},
		{"  [x]  ", true, 'x'},
		{"[X]", true, 'x'}, // uppercase tolerated
		{"[o]", false, 0},
		{"[xx]", false, 0},
		{"", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			m, ok := parseAcceptanceState(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("parseAcceptanceState(%q) ok=%v want=%v", tc.in, ok, tc.wantOK)
			}
			if ok && m != tc.wantM {
				t.Fatalf("parseAcceptanceState(%q) marker=%q want=%q", tc.in, string(m), string(tc.wantM))
			}
		})
	}
}

// TestApplyAcceptanceTransition covers every validation branch plus the
// happy path. The handler in artifact_propose.go stores from_state +
// new_state on shape_payload, so from-state correctness matters for
// audit-trail queries.
func TestApplyAcceptanceTransition(t *testing.T) {
	body := strings.Join([]string{
		"## Acceptance",
		"- [ ] first",
		"- [x] second",
		"- [ ] third",
	}, "\n")

	intPtr := func(n int) *int { return &n }

	t.Run("index required", func(t *testing.T) {
		_, _, code := applyAcceptanceTransition(body, &AcceptanceTransitionInput{NewState: "[x]"})
		if code != "ACCEPT_TRANSITION_INDEX_REQUIRED" {
			t.Fatalf("code=%q want=ACCEPT_TRANSITION_INDEX_REQUIRED", code)
		}
	})

	t.Run("state invalid", func(t *testing.T) {
		_, _, code := applyAcceptanceTransition(body, &AcceptanceTransitionInput{
			CheckboxIndex: intPtr(0), NewState: "[bogus]",
		})
		if code != "ACCEPT_TRANSITION_STATE_INVALID" {
			t.Fatalf("code=%q want=ACCEPT_TRANSITION_STATE_INVALID", code)
		}
	})

	t.Run("reason required for partial", func(t *testing.T) {
		_, _, code := applyAcceptanceTransition(body, &AcceptanceTransitionInput{
			CheckboxIndex: intPtr(0), NewState: "[~]",
		})
		if code != "ACCEPT_TRANSITION_REASON_REQUIRED" {
			t.Fatalf("code=%q want=ACCEPT_TRANSITION_REASON_REQUIRED", code)
		}
	})

	t.Run("reason required for deferred", func(t *testing.T) {
		_, _, code := applyAcceptanceTransition(body, &AcceptanceTransitionInput{
			CheckboxIndex: intPtr(0), NewState: "[-]",
		})
		if code != "ACCEPT_TRANSITION_REASON_REQUIRED" {
			t.Fatalf("code=%q want=ACCEPT_TRANSITION_REASON_REQUIRED", code)
		}
	})

	t.Run("reason optional for done", func(t *testing.T) {
		got, from, code := applyAcceptanceTransition(body, &AcceptanceTransitionInput{
			CheckboxIndex: intPtr(0), NewState: "[x]",
		})
		if code != "" {
			t.Fatalf("unexpected code=%q", code)
		}
		if from != ' ' {
			t.Fatalf("from=%q want=' '", string(from))
		}
		if !strings.Contains(got, "- [x] first") {
			t.Fatalf("rewritten body missing marker flip:\n%s", got)
		}
	})

	t.Run("out of range", func(t *testing.T) {
		_, _, code := applyAcceptanceTransition(body, &AcceptanceTransitionInput{
			CheckboxIndex: intPtr(99), NewState: "[x]",
		})
		if code != "ACCEPT_TRANSITION_INDEX_OUT_OF_RANGE" {
			t.Fatalf("code=%q want=ACCEPT_TRANSITION_INDEX_OUT_OF_RANGE", code)
		}
	})

	t.Run("no-op is rejected", func(t *testing.T) {
		// second checkbox is already [x]; transitioning to [x] is a no-op
		_, _, code := applyAcceptanceTransition(body, &AcceptanceTransitionInput{
			CheckboxIndex: intPtr(1), NewState: "[x]",
		})
		if code != "ACCEPT_TRANSITION_NOOP" {
			t.Fatalf("code=%q want=ACCEPT_TRANSITION_NOOP", code)
		}
	})

	t.Run("defer with reason succeeds", func(t *testing.T) {
		got, from, code := applyAcceptanceTransition(body, &AcceptanceTransitionInput{
			CheckboxIndex: intPtr(2), NewState: "[-]", Reason: "moved to task-X",
		})
		if code != "" {
			t.Fatalf("unexpected code=%q", code)
		}
		if from != ' ' {
			t.Fatalf("from=%q want=' '", string(from))
		}
		if !strings.Contains(got, "- [-] third") {
			t.Fatalf("rewritten body missing marker flip:\n%s", got)
		}
	})

	t.Run("reopen clears done marker", func(t *testing.T) {
		got, from, code := applyAcceptanceTransition(body, &AcceptanceTransitionInput{
			CheckboxIndex: intPtr(1), NewState: "[ ]",
		})
		if code != "" {
			t.Fatalf("unexpected code=%q", code)
		}
		if from != 'x' {
			t.Fatalf("from=%q want='x'", string(from))
		}
		if !strings.Contains(got, "- [ ] second") {
			t.Fatalf("expected second item reopened, got:\n%s", got)
		}
	})

	t.Run("uppercase X canonicalises on compare", func(t *testing.T) {
		src := "- [X] upper"
		_, _, code := applyAcceptanceTransition(src, &AcceptanceTransitionInput{
			CheckboxIndex: intPtr(0), NewState: "[x]",
		})
		if code != "ACCEPT_TRANSITION_NOOP" {
			t.Fatalf("expected no-op since [X] canonicalises to [x]; got %q", code)
		}
	})
}
