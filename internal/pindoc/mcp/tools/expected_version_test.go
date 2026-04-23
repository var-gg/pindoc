package tools

import (
	"context"
	"testing"
)

// TestPreflightExpectedVersionReserved covers the Phase A legal-zero rule:
// migration 0017 guarantees every artifact has revision >= 1, so
// expected_version = 0 is no longer valid. Agents that pass 0 get a
// distinct FIELD_VALUE_RESERVED code so the retry hint points at
// pindoc.artifact.revisions rather than a generic NEED_VER.
func TestPreflightExpectedVersionReserved(t *testing.T) {
	baseBody := "## Context\nwhy\n## Decision\nwhat\n"

	zero := 0
	one := 1
	negative := -3

	t.Run("expected_version=0 is FIELD_VALUE_RESERVED", func(t *testing.T) {
		in := artifactProposeInput{
			Type:            "Decision",
			Title:           "t",
			BodyMarkdown:    baseBody,
			AreaSlug:        "misc",
			AuthorID:        "test-agent",
			UpdateOf:        "some-slug",
			CommitMsg:       "update",
			ExpectedVersion: &zero,
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if !containsCode(failed, "FIELD_VALUE_RESERVED") {
			t.Fatalf("expected FIELD_VALUE_RESERVED in failed=%v", failed)
		}
		if containsCode(failed, "VER_INVALID") {
			t.Fatalf("VER_INVALID should only fire for negative values, got %v", failed)
		}
	})

	t.Run("expected_version<0 is VER_INVALID", func(t *testing.T) {
		in := artifactProposeInput{
			Type:            "Decision",
			Title:           "t",
			BodyMarkdown:    baseBody,
			AreaSlug:        "misc",
			AuthorID:        "test-agent",
			UpdateOf:        "some-slug",
			CommitMsg:       "update",
			ExpectedVersion: &negative,
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if !containsCode(failed, "VER_INVALID") {
			t.Fatalf("expected VER_INVALID in failed=%v", failed)
		}
		if containsCode(failed, "FIELD_VALUE_RESERVED") {
			t.Fatalf("FIELD_VALUE_RESERVED should only fire for exact zero, got %v", failed)
		}
	})

	t.Run("expected_version>=1 passes zero/negative gates", func(t *testing.T) {
		in := artifactProposeInput{
			Type:            "Decision",
			Title:           "t",
			BodyMarkdown:    baseBody,
			AreaSlug:        "misc",
			AuthorID:        "test-agent",
			UpdateOf:        "some-slug",
			CommitMsg:       "update",
			ExpectedVersion: &one,
		}
		_, failed, _ := preflight(context.Background(), Deps{}, &in, "en")
		if containsCode(failed, "FIELD_VALUE_RESERVED") || containsCode(failed, "VER_INVALID") {
			t.Fatalf("expected_version=1 should pass zero/negative gates, got %v", failed)
		}
	})
}

// TestPatchFieldsForFieldValueReserved asserts the retry hint for
// FIELD_VALUE_RESERVED points at expected_version — the only field it's
// currently wired to.
func TestPatchFieldsForFieldValueReserved(t *testing.T) {
	fields := patchFieldsFor("FIELD_VALUE_RESERVED")
	if len(fields) != 1 || fields[0] != "expected_version" {
		t.Fatalf("expected patchable=[expected_version], got %v", fields)
	}
	tools := defaultNextTools("FIELD_VALUE_RESERVED")
	if len(tools) == 0 {
		t.Fatalf("FIELD_VALUE_RESERVED should hint a next tool, got %v", tools)
	}
	hasRev := false
	for _, tl := range tools {
		if tl == "pindoc.artifact.revisions" {
			hasRev = true
			break
		}
	}
	if !hasRev {
		t.Fatalf("FIELD_VALUE_RESERVED next_tools should include pindoc.artifact.revisions, got %v", tools)
	}
}
