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
