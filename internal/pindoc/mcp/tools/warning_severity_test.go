package tools

import (
	"reflect"
	"testing"
)

// TestWarningSeverity classifies every cataloged code plus the unknown
// fallback. Used by sortWarningsBySeverity — if this flips the wrong
// way the response ordering silently changes, so these are worth
// locking down.
func TestWarningSeverity(t *testing.T) {
	cases := map[string]string{
		// base codes
		"CANONICAL_REWRITE_WITHOUT_EVIDENCE": SeverityError,
		"CLAIMED_DONE_INCOMPLETE":            SeverityError,
		"RECEIPT_SUPERSEDED":                 SeverityError,
		"FIELD_VALUE_RESERVED":               SeverityError,
		"CONSENT_REQUIRED_FOR_USER_CHAT":     SeverityWarn,
		"SOURCE_TYPE_UNCLASSIFIED":           SeverityWarn,
		"BODY_HAS_H1_REDUNDANT":              SeverityWarn,
		"TITLE_LONG":                         SeverityWarn,
		"RECOMMEND_READ_BEFORE_CREATE":       SeverityInfo,

		// prefix match — CANONICAL_REWRITE_WITHOUT_EVIDENCE:<sections>
		// should still resolve via the base code.
		"CANONICAL_REWRITE_WITHOUT_EVIDENCE:Root cause+Decision": SeverityError,
		"MISSING_H2:Purpose":                                      SeverityWarn,

		// unknown defaults to warn
		"TOTALLY_UNHEARD_OF_CODE": SeverityWarn,
		"":                        SeverityWarn,
	}
	for code, want := range cases {
		if got := warningSeverity(code); got != want {
			t.Errorf("warningSeverity(%q) = %q; want %q", code, got, want)
		}
	}
}

// TestSortWarningsBySeverity asserts error > warn > info, stable within
// a severity so same-severity warnings keep their emit order.
func TestSortWarningsBySeverity(t *testing.T) {
	in := []string{
		"RECOMMEND_READ_BEFORE_CREATE",       // info
		"CONSENT_REQUIRED_FOR_USER_CHAT",     // warn
		"CANONICAL_REWRITE_WITHOUT_EVIDENCE", // error
		"MISSING_H2:Purpose",                 // warn
		"CLAIMED_DONE_INCOMPLETE",            // error
	}
	got := sortWarningsBySeverity(in)
	want := []string{
		"CANONICAL_REWRITE_WITHOUT_EVIDENCE",
		"CLAIMED_DONE_INCOMPLETE",
		"CONSENT_REQUIRED_FOR_USER_CHAT",
		"MISSING_H2:Purpose",
		"RECOMMEND_READ_BEFORE_CREATE",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sort order\n  got:  %v\n  want: %v", got, want)
	}
}

// TestSortWarningsStableWithinSeverity — same-severity pair shouldn't
// swap when both rank equal.
func TestSortWarningsStableWithinSeverity(t *testing.T) {
	in := []string{"CONSENT_REQUIRED_FOR_USER_CHAT", "SOURCE_TYPE_UNCLASSIFIED"}
	got := sortWarningsBySeverity(in)
	if got[0] != "CONSENT_REQUIRED_FOR_USER_CHAT" {
		t.Fatalf("stable sort broke: %v", got)
	}
}

// TestSortWarningsEmpty — nil / empty input returns itself.
func TestSortWarningsEmpty(t *testing.T) {
	if got := sortWarningsBySeverity(nil); got != nil {
		t.Fatalf("nil input should return nil, got %v", got)
	}
	if got := sortWarningsBySeverity([]string{}); len(got) != 0 {
		t.Fatalf("empty input should stay empty, got %v", got)
	}
}
