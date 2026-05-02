package preflight

import "testing"

func TestDetectForbiddenAcceptanceVerbsCoversCanonicalVerbs(t *testing.T) {
	for _, rule := range ForbiddenAcceptanceVerbs {
		text := "완료 기준: " + rule.Canonical + "."
		if hits := DetectForbiddenAcceptanceVerbs(text); len(hits) != 1 || hits[0].Verb != rule.Canonical {
			t.Fatalf("%q not detected, hits=%+v", rule.Canonical, hits)
		}
	}
}

func TestDetectForbiddenAcceptanceVerbsCoversKoreanEndings(t *testing.T) {
	cases := []string{
		"사용처를 조사한다.",
		"사용처를 조사했다.",
		"사용처를 조사합니다.",
		"레이아웃을 살펴본다.",
		"레이아웃을 살펴봤다.",
		"레이아웃을 살펴봅니다.",
	}
	for _, text := range cases {
		if hits := DetectForbiddenAcceptanceVerbs(text); len(hits) == 0 {
			t.Fatalf("%q should be detected", text)
		}
	}
}

func TestDetectForbiddenAcceptanceVerbsAvoidsFalsePositives(t *testing.T) {
	cases := []string{
		"확인 가능 상태가 응답 JSON에 존재한다.",
		"검토 결과 필드가 비어 있지 않다.",
		"정리된 표가 Outcome 섹션에 존재한다.",
		"재확인합니다라는 예시 문자열을 코드 블록에서 설명한다.",
		"attachment generated output exists.",
	}
	for _, text := range cases {
		if hits := DetectForbiddenAcceptanceVerbs(text); len(hits) != 0 {
			t.Fatalf("%q should not be detected, hits=%+v", text, hits)
		}
	}
}

func TestLintAcceptanceVerbsReturnsLineAndCheckboxIndex(t *testing.T) {
	body := "## TODO — Acceptance criteria\n\n" +
		"- [ ] 첫 항목은 통과한다.\n" +
		"- [ ] 기존 Task를 분류한다.\n" +
		"plain text 확인한다.\n" +
		"- [x] 릴리스 리스크를 정리했다.\n"

	findings := LintAcceptanceVerbs(body)
	if len(findings) != 2 {
		t.Fatalf("findings len=%d, want 2: %+v", len(findings), findings)
	}
	if findings[0].LineNumber != 4 || findings[0].CheckboxIndex != 1 || findings[0].Verb != "분류한다" {
		t.Fatalf("first finding mismatch: %+v", findings[0])
	}
	if findings[1].LineNumber != 6 || findings[1].CheckboxIndex != 2 || findings[1].Verb != "정리한다" {
		t.Fatalf("second finding mismatch: %+v", findings[1])
	}
}

func TestSuggestedRewriteActionsIncludeExamples(t *testing.T) {
	actions := SuggestedRewriteActions(1)
	if len(actions) < 2 {
		t.Fatalf("actions should include guidance and at least one example: %+v", actions)
	}
}
