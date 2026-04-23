package tools

import (
	"context"
	"testing"
)

// TestParseShape covers the discriminator parser:
//   - empty string defaults to body_patch so legacy agent calls keep
//     working without specifying the field
//   - known shapes pass through verbatim
//   - anything else returns ok=false so preflight can trip SHAPE_INVALID
func TestParseShape(t *testing.T) {
	cases := []struct {
		in        string
		wantShape RevisionShape
		wantOK    bool
	}{
		{"", ShapeBodyPatch, true},
		{"body_patch", ShapeBodyPatch, true},
		{"meta_patch", ShapeMetaPatch, true},
		{"acceptance_transition", ShapeAcceptanceTransition, true},
		{"scope_defer", ShapeScopeDefer, true},
		{"  body_patch  ", ShapeBodyPatch, true},
		{"garbage", "", false},
		{"BODY_PATCH", "", false}, // enums are case-sensitive on the wire
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := parseShape(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("parseShape(%q) ok=%v want=%v", tc.in, ok, tc.wantOK)
			}
			if got != tc.wantShape {
				t.Fatalf("parseShape(%q) shape=%q want=%q", tc.in, got, tc.wantShape)
			}
		})
	}
}

// TestIsTemplateArtifact asserts the slug-prefix check the canonical-
// rewrite guard uses to skip noise on _template_* edits.
func TestIsTemplateArtifact(t *testing.T) {
	cases := []struct {
		slug string
		want bool
	}{
		{"_template_debug", true},
		{"_template_decision", true},
		{"_template_analysis", true},
		{"_template_task", true},
		{"_templated-thing", false}, // prefix must be _template_, not _templated
		{"template_debug", false},   // missing leading underscore
		{"some-decision", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.slug, func(t *testing.T) {
			if got := isTemplateArtifact(tc.slug); got != tc.want {
				t.Fatalf("isTemplateArtifact(%q) = %v; want %v", tc.slug, got, tc.want)
			}
		})
	}
}

// TestPreflightShapeValidation exercises the preflight-level rejection of
// invalid / misplaced shape values so agents see them alongside other
// schema failures rather than mid-handler.
func TestPreflightShapeValidation(t *testing.T) {
	baseBody := "## Context\nwhy\n## Decision\nwhat\n"
	one := 1

	t.Run("unknown shape is SHAPE_INVALID", func(t *testing.T) {
		in := artifactProposeInput{
			Type:            "Decision",
			Title:           "t",
			BodyMarkdown:    baseBody,
			AreaSlug:        "misc",
			AuthorID:        "test-agent",
			UpdateOf:        "some-slug",
			CommitMsg:       "update",
			ExpectedVersion: &one,
			Shape:           "mystery_patch",
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if !containsCode(failed, "SHAPE_INVALID") {
			t.Fatalf("expected SHAPE_INVALID in failed=%v", failed)
		}
	})

	t.Run("non-body_patch shape on create is SHAPE_REQUIRES_UPDATE", func(t *testing.T) {
		in := artifactProposeInput{
			Type:         "Task",
			Title:        "t",
			BodyMarkdown: baseBody,
			AreaSlug:     "misc",
			AuthorID:     "test-agent",
			Shape:        "meta_patch",
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if !containsCode(failed, "SHAPE_REQUIRES_UPDATE") {
			t.Fatalf("expected SHAPE_REQUIRES_UPDATE in failed=%v", failed)
		}
	})

	t.Run("body_patch on create is accepted", func(t *testing.T) {
		in := artifactProposeInput{
			Type:         "Decision",
			Title:        "t",
			BodyMarkdown: baseBody,
			AreaSlug:     "misc",
			AuthorID:     "test-agent",
			Shape:        "body_patch",
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if containsCode(failed, "SHAPE_INVALID") || containsCode(failed, "SHAPE_REQUIRES_UPDATE") {
			t.Fatalf("body_patch on create should pass shape gates, got %v", failed)
		}
	})

	t.Run("empty shape defaults to body_patch (no gate fires)", func(t *testing.T) {
		in := artifactProposeInput{
			Type:         "Decision",
			Title:        "t",
			BodyMarkdown: baseBody,
			AreaSlug:     "misc",
			AuthorID:     "test-agent",
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if containsCode(failed, "SHAPE_INVALID") || containsCode(failed, "SHAPE_REQUIRES_UPDATE") {
			t.Fatalf("omitted shape should default to body_patch silently, got %v", failed)
		}
	})
}

// TestPatchFieldsForShapeCodes asserts the new shape error codes map to
// the 'shape' field so agents know exactly what to patch.
func TestPatchFieldsForShapeCodes(t *testing.T) {
	for _, code := range []string{"SHAPE_INVALID", "SHAPE_REQUIRES_UPDATE", "SHAPE_NOT_IMPLEMENTED"} {
		got := patchFieldsFor(code)
		if len(got) != 1 || got[0] != "shape" {
			t.Fatalf("patchFieldsFor(%q) = %v; want [shape]", code, got)
		}
	}
}
