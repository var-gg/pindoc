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

func TestClassifyAreaCreateParentRejectsSubAreaParent(t *testing.T) {
	parentID := "parent-area-id"
	parentParentID := "top-level-id"
	if got := classifyAreaCreateParent(&parentID, &parentParentID); got != "PARENT_NOT_TOP_LEVEL" {
		t.Fatalf("classifyAreaCreateParent = %q; want PARENT_NOT_TOP_LEVEL", got)
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
