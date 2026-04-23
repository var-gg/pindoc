package tools

import (
	"strings"
	"testing"
)

// TestApplyBodyPatchSectionReplace covers the section_replace mode. Two
// positives (pure heading + mixed ko/en heading) and a not-found miss.
func TestApplyBodyPatchSectionReplace(t *testing.T) {
	body := "## Purpose\n\noriginal purpose line.\n\n## Scope\n\nstays put.\n"

	t.Run("exact heading", func(t *testing.T) {
		got, warns, code := applyBodyPatch(body, &BodyPatchInput{
			Mode:           "section_replace",
			SectionHeading: "Purpose",
			Replacement:    "replaced content.",
		})
		if code != "" {
			t.Fatalf("unexpected code=%q", code)
		}
		if len(warns) != 0 {
			t.Fatalf("unexpected warnings=%v", warns)
		}
		if !strings.Contains(got, "replaced content.") {
			t.Fatalf("missing replacement: %q", got)
		}
		if !strings.Contains(got, "## Scope") {
			t.Fatalf("scope section dropped: %q", got)
		}
		if strings.Contains(got, "original purpose line.") {
			t.Fatalf("old content still present: %q", got)
		}
	})

	t.Run("mixed ko-en heading", func(t *testing.T) {
		mixed := "## 목적 / Purpose\n\nko/en mixed heading body.\n\n## Scope\n\nrest.\n"
		got, _, code := applyBodyPatch(mixed, &BodyPatchInput{
			Mode:           "section_replace",
			SectionHeading: "Purpose",
			Replacement:    "new body.",
		})
		if code != "" {
			t.Fatalf("unexpected code=%q", code)
		}
		if !strings.Contains(got, "## 목적 / Purpose") {
			t.Fatalf("heading line lost: %q", got)
		}
		if !strings.Contains(got, "new body.") {
			t.Fatalf("replacement missing: %q", got)
		}
	})

	t.Run("heading miss", func(t *testing.T) {
		_, _, code := applyBodyPatch(body, &BodyPatchInput{
			Mode:           "section_replace",
			SectionHeading: "Rationale",
			Replacement:    "x",
		})
		if code != "PATCH_SECTION_NOT_FOUND" {
			t.Fatalf("expected PATCH_SECTION_NOT_FOUND, got %q", code)
		}
	})
}

// TestApplyBodyPatchCheckboxToggle covers the checkbox_toggle mode —
// positive toggle, PATCH_NOOP when already in target state, out-of-range.
func TestApplyBodyPatchCheckboxToggle(t *testing.T) {
	body := "## TODO\n\n- [ ] first\n- [ ] second\n- [x] third\n"

	t.Run("toggle unchecked to checked", func(t *testing.T) {
		idx := 0
		state := true
		got, warns, code := applyBodyPatch(body, &BodyPatchInput{
			Mode:          "checkbox_toggle",
			CheckboxIndex: &idx,
			CheckboxState: &state,
		})
		if code != "" {
			t.Fatalf("unexpected code=%q", code)
		}
		if len(warns) != 0 {
			t.Fatalf("unexpected warns=%v", warns)
		}
		if !strings.Contains(got, "- [x] first") {
			t.Fatalf("first item not toggled: %q", got)
		}
		if !strings.Contains(got, "- [ ] second") {
			t.Fatalf("second item changed inadvertently: %q", got)
		}
	})

	t.Run("already in target state -> PATCH_NOOP warning", func(t *testing.T) {
		idx := 2
		state := true
		_, warns, code := applyBodyPatch(body, &BodyPatchInput{
			Mode:          "checkbox_toggle",
			CheckboxIndex: &idx,
			CheckboxState: &state,
		})
		if code != "" {
			t.Fatalf("unexpected code=%q", code)
		}
		if len(warns) != 1 || warns[0] != "PATCH_NOOP" {
			t.Fatalf("expected PATCH_NOOP warning, got %v", warns)
		}
	})

	t.Run("index out of range", func(t *testing.T) {
		idx := 99
		state := true
		_, _, code := applyBodyPatch(body, &BodyPatchInput{
			Mode:          "checkbox_toggle",
			CheckboxIndex: &idx,
			CheckboxState: &state,
		})
		if code != "PATCH_CHECKBOX_OUT_OF_RANGE" {
			t.Fatalf("expected PATCH_CHECKBOX_OUT_OF_RANGE, got %q", code)
		}
	})

	t.Run("missing index", func(t *testing.T) {
		state := true
		_, _, code := applyBodyPatch(body, &BodyPatchInput{
			Mode:          "checkbox_toggle",
			CheckboxState: &state,
		})
		if code != "PATCH_CHECKBOX_INDEX_REQUIRED" {
			t.Fatalf("expected PATCH_CHECKBOX_INDEX_REQUIRED, got %q", code)
		}
	})
}

// TestApplyBodyPatchAppend covers simple text append + empty rejection.
func TestApplyBodyPatchAppend(t *testing.T) {
	t.Run("append text adds blank line separator", func(t *testing.T) {
		body := "## Notes\n\nexisting line.\n"
		got, _, code := applyBodyPatch(body, &BodyPatchInput{
			Mode:       "append",
			AppendText: "- extra bullet",
		})
		if code != "" {
			t.Fatalf("unexpected code=%q", code)
		}
		if !strings.HasSuffix(got, "- extra bullet\n") {
			t.Fatalf("append missing: %q", got)
		}
		if !strings.Contains(got, "existing line.") {
			t.Fatalf("original body dropped: %q", got)
		}
	})

	t.Run("empty append rejects", func(t *testing.T) {
		_, _, code := applyBodyPatch("body\n", &BodyPatchInput{
			Mode:       "append",
			AppendText: "   ",
		})
		if code != "PATCH_APPEND_EMPTY" {
			t.Fatalf("expected PATCH_APPEND_EMPTY, got %q", code)
		}
	})
}

// TestApplyBodyPatchInvalidMode checks that an unknown mode produces the
// stable code rather than silently modifying the body.
func TestApplyBodyPatchInvalidMode(t *testing.T) {
	_, _, code := applyBodyPatch("body\n", &BodyPatchInput{Mode: "foo"})
	if code != "PATCH_MODE_INVALID" {
		t.Fatalf("expected PATCH_MODE_INVALID, got %q", code)
	}
}
