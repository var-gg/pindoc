package tools

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestStableMCPErrorCodeNormalizesToScreamingSnake(t *testing.T) {
	cases := map[string]string{
		"DEC_NO_SECTIONS":       "DEC_NO_SECTIONS",
		"MISSING_H2:목적":         "MISSING_H2",
		" response-format bad ": "RESPONSE_FORMAT_BAD",
	}
	for raw, want := range cases {
		if got := stableMCPErrorCode(raw); got != want {
			t.Fatalf("stableMCPErrorCode(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestApplyMCPErrorContractAddsCodesAndChecklistItems(t *testing.T) {
	out := artifactProposeOutput{
		Status:    "not_ready",
		ErrorCode: "MISSING_H2:목적",
		Failed:    []string{"MISSING_H2:목적", "DEC_NO_SECTIONS"},
		Checklist: []string{"목적 섹션이 필요합니다.", "Decision에는 context/decision 섹션이 필요합니다."},
	}

	got := applyMCPErrorContract(out, "ko-KR")
	assertErrorContract(t, got.ErrorCodes, got.ChecklistItems, got.MessageLocale, "ko")
	if got.ErrorCodes[0] != "MISSING_H2" || got.ErrorCodes[1] != "DEC_NO_SECTIONS" {
		t.Fatalf("unexpected error_codes: %v", got.ErrorCodes)
	}
	if got.Failed[0] != "MISSING_H2" {
		t.Fatalf("failed[] should be normalized stable codes, got %v", got.Failed)
	}
	if !strings.Contains(got.ChecklistItems[0].Message, "목적") {
		t.Fatalf("checklist_items[0].message should carry localized copy: %+v", got.ChecklistItems[0])
	}
}

func TestTaskAcceptanceTransitionOutputErrorContract(t *testing.T) {
	out := taskAcceptanceTransitionOutput{
		Status:    "not_ready",
		ErrorCode: "ACCEPT_TRANSITION_REASON_REQUIRED",
		Failed:    []string{"ACCEPT_TRANSITION_REASON_REQUIRED"},
		Checklist: []string{"부분 완료나 이관에는 reason이 필요합니다."},
	}

	got := applyMCPErrorContract(out, "ko")
	assertErrorContract(t, got.ErrorCodes, got.ChecklistItems, got.MessageLocale, "ko")
	if got.ChecklistItems[0].Code != "ACCEPT_TRANSITION_REASON_REQUIRED" {
		t.Fatalf("checklist item code = %q", got.ChecklistItems[0].Code)
	}
}

func TestHarnessInstallInvalidResponseFormatNotReadyContract(t *testing.T) {
	got := harnessInstallResponseFormatNotReady("ko")
	assertErrorContract(t, got.ErrorCodes, got.ChecklistItems, got.MessageLocale, "ko")
	if got.ErrorCode != "HARNESS_RESPONSE_FORMAT_INVALID" {
		t.Fatalf("error_code = %q", got.ErrorCode)
	}
	if !strings.Contains(got.ChecklistItems[0].Message, "허용") {
		t.Fatalf("expected Korean checklist message, got %+v", got.ChecklistItems[0])
	}
}

func TestProjectCreateNotReadyContract(t *testing.T) {
	out, ok := projectCreateNotReady("ko", errors.New("plain"))
	if ok || out.Status != "" {
		t.Fatalf("unknown errors should stay handler errors: ok=%v out=%+v", ok, out)
	}

	got, ok := projectCreateNotReady("ko", projects.ErrLangRequired)
	if !ok {
		t.Fatalf("expected project create validation error to map to not_ready")
	}
	assertErrorContract(t, got.ErrorCodes, got.ChecklistItems, got.MessageLocale, "ko")
	if got.ErrorCode != "LANG_REQUIRED" {
		t.Fatalf("error_code = %q", got.ErrorCode)
	}
	if !strings.Contains(got.ChecklistItems[0].Message, "primary_language") {
		t.Fatalf("expected primary_language guidance, got %+v", got.ChecklistItems[0])
	}
}

func assertErrorContract(t *testing.T, codes []string, items []ErrorChecklistItem, locale, wantLocale string) {
	t.Helper()
	if locale != wantLocale {
		t.Fatalf("message_locale = %q, want %q", locale, wantLocale)
	}
	if len(codes) == 0 {
		t.Fatalf("expected error_codes")
	}
	codePattern := regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	for _, code := range codes {
		if !codePattern.MatchString(code) {
			t.Fatalf("error code %q is not SCREAMING_SNAKE_CASE", code)
		}
	}
	if len(items) == 0 {
		t.Fatalf("expected checklist_items")
	}
	for _, item := range items {
		if !codePattern.MatchString(item.Code) {
			t.Fatalf("checklist item code %q is not SCREAMING_SNAKE_CASE", item.Code)
		}
		if strings.TrimSpace(item.Message) == "" {
			t.Fatalf("checklist item message is empty: %+v", item)
		}
	}
}
