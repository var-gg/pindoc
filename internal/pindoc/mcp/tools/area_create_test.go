package tools

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateAreaCreateInputNormalizesValidInput(t *testing.T) {
	got, notReady := validateAreaCreateInput(areaCreateInput{
		ParentSlug:  "Architecture ",
		Slug:        " MCP-Surface ",
		Name:        " MCP Surface ",
		Description: " Agent-facing tools ",
	}, "en")
	if notReady != nil {
		t.Fatalf("validateAreaCreateInput returned not_ready: %+v", *notReady)
	}
	if got.ParentSlug != "architecture" || got.Slug != "mcp-surface" ||
		got.Name != "MCP Surface" || got.Description != "Agent-facing tools" {
		t.Fatalf("normalized input mismatch: %+v", got)
	}
}

func TestValidateAreaCreateInputParentMissing(t *testing.T) {
	_, notReady := validateAreaCreateInput(areaCreateInput{
		Slug: "mcp-surface",
		Name: "MCP Surface",
	}, "en")
	assertAreaCreateCode(t, notReady, "PARENT_REQUIRED")
}

func TestClassifyAreaCreateParent(t *testing.T) {
	id := func(s string) *string { return &s }
	depth := func(n int) *int { return &n }

	tests := []struct {
		name                string
		parentID            *string
		parentParentID      *string
		parentGrandparentID *string
		rootMaxDepth        *int
		want                string
	}{
		{"parent not found", nil, nil, nil, nil, "PARENT_NOT_FOUND"},
		{"top-level parent allows depth-1 child", id("tl"), nil, nil, nil, ""},
		{"depth-1 parent rejected when root max_depth is 1", id("p"), id("tl"), nil, depth(1), "PARENT_NOT_TOP_LEVEL"},
		{"depth-1 parent rejected when root max_depth is unknown", id("p"), id("tl"), nil, nil, "PARENT_NOT_TOP_LEVEL"},
		{"depth-1 parent allowed when root max_depth is 2", id("p"), id("tl"), nil, depth(2), ""},
		{"depth-2 parent always rejected", id("p"), id("d1"), id("tl"), depth(2), "PARENT_NOT_TOP_LEVEL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyAreaCreateParent(tc.parentID, tc.parentParentID, tc.parentGrandparentID, tc.rootMaxDepth); got != tc.want {
				t.Fatalf("classifyAreaCreateParent = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestAreaCreateSlugTakenDetectsUniqueViolation(t *testing.T) {
	err := errors.New(`ERROR: duplicate key value violates unique constraint "areas_project_id_slug_key" (SQLSTATE 23505)`)
	if !isAreaCreateSlugTaken(err) {
		t.Fatal("isAreaCreateSlugTaken returned false for areas unique violation")
	}
}

func TestValidateAreaCreateInputRejectsEmptySlug(t *testing.T) {
	_, notReady := validateAreaCreateInput(areaCreateInput{
		ParentSlug: "architecture",
		Name:       "MCP Surface",
	}, "en")
	assertAreaCreateCode(t, notReady, "SLUG_INVALID")
}

func TestAreaCreateI18NKeysResolve(t *testing.T) {
	for _, lang := range []string{"en", "ko"} {
		for _, code := range []string{
			"PARENT_REQUIRED",
			"PARENT_NOT_FOUND",
			"PARENT_NOT_TOP_LEVEL",
			"SLUG_INVALID",
			"AREA_SLUG_TAKEN",
			"AREA_NAME_INVALID",
			"AREA_DESCRIPTION_TOO_LONG",
		} {
			out := areaCreateNotReady(lang, code, "x")
			if len(out.Checklist) != 1 || strings.HasPrefix(out.Checklist[0], "preflight.") {
				t.Fatalf("missing i18n for lang=%s code=%s: %+v", lang, code, out.Checklist)
			}
		}
	}
}

func assertAreaCreateCode(t *testing.T, out *areaCreateOutput, want string) {
	t.Helper()
	if out == nil {
		t.Fatalf("expected not_ready %s; got nil", want)
	}
	if out.Status != "not_ready" || out.ErrorCode != want {
		t.Fatalf("not_ready = %+v; want code %s", *out, want)
	}
}
