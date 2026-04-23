package tools

import (
	"testing"
)

// TestPatchFieldsForScopeDefer covers the retry hints for scope_defer
// error codes — agents need "scope_defer" as the patchable field for
// every rejection so they know exactly what to re-supply.
func TestPatchFieldsForScopeDefer(t *testing.T) {
	cases := []string{
		"SCOPE_DEFER_REQUIRED",
		"SCOPE_DEFER_REASON_REQUIRED",
		"SCOPE_DEFER_TARGET_NOT_FOUND",
		"SCOPE_DEFER_SELF",
	}
	for _, code := range cases {
		got := patchFieldsFor(code)
		if len(got) != 1 || got[0] != "scope_defer" {
			t.Fatalf("patchFieldsFor(%q) = %v; want [scope_defer]", code, got)
		}
	}
	got := defaultNextTools("SCOPE_DEFER_TARGET_NOT_FOUND")
	// target-not-found points at search/area.list so the agent can
	// confirm the intended destination exists.
	hasSearch := false
	for _, tl := range got {
		if tl == "pindoc.artifact.search" {
			hasSearch = true
		}
	}
	if !hasSearch {
		t.Fatalf("SCOPE_DEFER_TARGET_NOT_FOUND next_tools missing pindoc.artifact.search: %v", got)
	}
}
