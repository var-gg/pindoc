package tools

import (
	"reflect"
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/receipts"
)

func TestTaskMetaPriorityDescriptionIncludesMeanings(t *testing.T) {
	field, ok := reflect.TypeOf(TaskMetaInput{}).FieldByName("Priority")
	if !ok {
		t.Fatalf("TaskMetaInput.Priority field missing")
	}
	schema := field.Tag.Get("jsonschema")
	for _, want := range []string{"p0 release blocker", "p1 must close before release", "p2 next round", "p3 backlog"} {
		if !strings.Contains(schema, want) {
			t.Fatalf("priority jsonschema tag %q missing %q", schema, want)
		}
	}
	if !strings.Contains(schema, "Project-specific priority policy wins") {
		t.Fatalf("priority jsonschema tag %q missing project-specific policy note", schema)
	}
}

func TestArtifactProposeTitleDescriptionIncludesLocalePrompt(t *testing.T) {
	field, ok := reflect.TypeOf(artifactProposeInput{}).FieldByName("Title")
	if !ok {
		t.Fatalf("artifactProposeInput.Title field missing")
	}
	schema := field.Tag.Get("jsonschema")
	for _, want := range []string{
		"non-English primary_language",
		"project-language script anchor",
		"English-only titles fragment Cmd+K keyword search",
		"cross-lingual semantic distance",
		"Mixed Korean/Japanese + English dev terms",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("title jsonschema tag %q missing %q", schema, want)
		}
	}
}

func TestArtifactProposeBodyLocaleDescriptionIncludesSafeSubset(t *testing.T) {
	field, ok := reflect.TypeOf(artifactProposeInput{}).FieldByName("BodyLocale")
	if !ok {
		t.Fatalf("artifactProposeInput.BodyLocale field missing")
	}
	schema := field.Tag.Get("jsonschema")
	for _, want := range []string{
		"BCP 47 safe subset",
		"ko, en, ja, ko-KR, en-US, en-GB, ja-JP",
		"default = project primary_language",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("body_locale jsonschema tag %q missing %q", schema, want)
		}
	}
}

func TestArtifactProposeBodyMarkdownOptionalForPatchSchema(t *testing.T) {
	field, ok := reflect.TypeOf(artifactProposeInput{}).FieldByName("BodyMarkdown")
	if !ok {
		t.Fatalf("artifactProposeInput.BodyMarkdown field missing")
	}
	if tag := field.Tag.Get("json"); !strings.Contains(tag, "omitempty") {
		t.Fatalf("body_markdown json tag should be omitempty for body_patch-only updates, got %q", tag)
	}
	schema := field.Tag.Get("jsonschema")
	for _, want := range []string{"omit on update_of", "body_patch"} {
		if !strings.Contains(schema, want) {
			t.Fatalf("body_markdown jsonschema tag %q missing %q", schema, want)
		}
	}
}

func TestArtifactProposeDescriptionMentionsPatchOnlyUpdate(t *testing.T) {
	desc := artifactProposeToolDescription
	for _, want := range []string{"body_patch instead of body_markdown", "update_of + expected_version + body_patch", "omit body_markdown"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("artifact.propose description missing %q: %q", want, desc)
		}
	}
}

func TestReadBeforeCreateWarningIncludesConcreteCandidate(t *testing.T) {
	scope := &auth.ProjectScope{ProjectSlug: "pindoc", ProjectLocale: "ko"}
	candidates := []semanticCandidate{{
		ArtifactID: "artifact-1",
		Slug:       "existing-task",
		Type:       "Task",
		Title:      "기존 Task",
		Distance:   0.24,
	}}
	out := readBeforeCreateWarningResult(Deps{}, scope, candidates, []receipts.ArtifactRef{{ArtifactID: "artifact-1", RevisionNumber: 3}})
	if len(out.Warnings) != 1 || out.Warnings[0] != "RECOMMEND_READ_BEFORE_CREATE" {
		t.Fatalf("warnings = %v", out.Warnings)
	}
	if len(out.Related) != 1 || out.Related[0].Slug != "existing-task" || out.Related[0].HumanURL == "" {
		t.Fatalf("related candidate missing: %+v", out.Related)
	}
	if len(out.NextTools) != 1 || out.NextTools[0].Tool != "pindoc.artifact.read" || out.NextTools[0].Args["id_or_slug"] != "existing-task" {
		t.Fatalf("next_tools missing concrete read: %+v", out.NextTools)
	}
	if !strings.Contains(strings.Join(out.SuggestedActions, "\n"), "already exposed") {
		t.Fatalf("receipt-aware suggested action missing: %v", out.SuggestedActions)
	}
}
