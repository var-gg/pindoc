package preflight

import "testing"

func TestCheckOutcomeSectionPresent(t *testing.T) {
	body := "## Outcome\n\n" +
		"- 핵심 결과: claim_done pre-flight가 추가됨.\n" +
		"- 코드 변경: commit `36f85c5bc5f4269e4e6e20102befd711b1692779`.\n" +
		"- 회귀 진술: 기존 task.assign tests regression 없음.\n"
	got := CheckOutcomeSection(body, OutcomeCheckOptions{CommitRequired: true})
	if !got.OK() {
		t.Fatalf("expected ok, got %+v", got)
	}
}

func TestCheckOutcomeSectionAbsent(t *testing.T) {
	got := CheckOutcomeSection("## TC / DoD\n\n- tests pass", OutcomeCheckOptions{CommitRequired: true})
	if got.OK() || !containsOutcomeCode(got.Codes, OutcomeSectionMissing) {
		t.Fatalf("expected missing section, got %+v", got)
	}
}

func TestCheckOutcomeSectionPartial(t *testing.T) {
	body := "## Outcome\n\n- 코드 변경: commit `36f85c5`.\n"
	got := CheckOutcomeSection(body, OutcomeCheckOptions{CommitRequired: true})
	for _, want := range []string{OutcomeFindingMissing, OutcomeRegressionMissing} {
		if !containsOutcomeCode(got.Codes, want) {
			t.Fatalf("expected %s in %+v", want, got)
		}
	}
	if containsOutcomeCode(got.Codes, OutcomeCommitMissing) {
		t.Fatalf("commit should be detected: %+v", got)
	}
}

func TestCheckOutcomeSectionCommitExempt(t *testing.T) {
	body := "## 완료 결과\n\n" +
		"- 핵심 결과: 정책 예외 문서가 작성됨.\n" +
		"- 회귀 진술: 기존 정책 링크 regression 없음.\n"
	got := CheckOutcomeSection(body, OutcomeCheckOptions{CommitRequired: false})
	if !got.OK() {
		t.Fatalf("commit-exempt outcome should pass, got %+v", got)
	}
}

func TestCheckOutcomeSectionStaleFirstCompliantSecond(t *testing.T) {
	// A stale "## Outcome (결과)" precedes a fully compliant "## Outcome".
	// The old first-match-only scan failed this; now any satisfying section
	// passes and both headings are reported as inspected.
	body := "## Outcome (결과)\n\n- 작업 시작 메모.\n\n" +
		"## Outcome\n\n" +
		"- 핵심 결과: outcome gate가 구현됨.\n" +
		"- 코드 변경: commit `36f85c5bc5f4269e4e6e20102befd711b1692779`.\n" +
		"- 회귀 진술: 기존 tests regression 없음.\n"
	got := CheckOutcomeSection(body, OutcomeCheckOptions{CommitRequired: true})
	if !got.OK() {
		t.Fatalf("compliant second section should pass, got %+v", got)
	}
	if !got.DuplicateOutcomeSections {
		t.Fatalf("expected DuplicateOutcomeSections=true, got %+v", got)
	}
	if len(got.InspectedHeadings) != 2 || got.InspectedHeadings[0] != "Outcome (결과)" || got.InspectedHeadings[1] != "Outcome" {
		t.Fatalf("expected both headings inspected in order, got %#v", got.InspectedHeadings)
	}
}

func TestCheckOutcomeSectionNoCrossSectionMerge(t *testing.T) {
	// Evidence must NOT be summed across sections: one section has
	// finding+regression, the other has only a commit — neither self-
	// satisfies, so the gate must still fail (no loosening loophole).
	body := "## Outcome\n\n- 핵심 결과: 구현 완료.\n- 회귀 진술: regression 없음.\n\n" +
		"## Outcome (결과)\n\n- 코드 변경: commit `36f85c5`.\n"
	got := CheckOutcomeSection(body, OutcomeCheckOptions{CommitRequired: true})
	if got.OK() {
		t.Fatalf("cross-section evidence must not merge; expected failure, got %+v", got)
	}
	if !got.DuplicateOutcomeSections || len(got.InspectedHeadings) != 2 {
		t.Fatalf("expected duplicate flag + 2 inspected headings, got %+v", got)
	}
}

func TestCheckOutcomeSectionBestOfFewestMissing(t *testing.T) {
	// First section misses all three; second misses only the commit. The
	// reported codes must come from the fewest-missing (second) section.
	body := "## Outcome\n\n- placeholder.\n\n" +
		"## Outcome\n\n- 핵심 결과: 구현됨.\n- 회귀 진술: regression 없음.\n"
	got := CheckOutcomeSection(body, OutcomeCheckOptions{CommitRequired: true})
	if got.OK() {
		t.Fatalf("expected failure, got %+v", got)
	}
	if len(got.Codes) != 1 || got.Codes[0] != OutcomeCommitMissing {
		t.Fatalf("expected only OUTCOME_COMMIT_MISSING from fewest-missing section, got %#v", got.Codes)
	}
}

func TestOutcomeTemplateSuggestedActions(t *testing.T) {
	if len(OutcomeTemplateSuggestedActions()) < 2 {
		t.Fatal("expected template actions")
	}
}

func containsOutcomeCode(codes []string, want string) bool {
	for _, code := range codes {
		if code == want {
			return true
		}
	}
	return false
}
