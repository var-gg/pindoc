package tools

import (
	"strings"
	"testing"
)

// TestProjectCreateDescriptionAdvertisesAreaSeed locks the agent-facing
// tool description: it must advertise the 9-area auto-seed and reference
// the governing Decision so agents reading the tool catalog know to
// expect the area skeleton without an extra round-trip. Implementation
// of the seed itself moved to internal/pindoc/projects — its tests live
// next to the data.
func TestProjectCreateDescriptionAdvertisesAreaSeed(t *testing.T) {
	if !strings.Contains(projectCreateDescription, "Auto-creates 9 top-level/project-root areas") {
		t.Fatalf("project_create description should advertise 9 project-root areas")
	}
	if !strings.Contains(projectCreateDescription, "area-구조-top-level-고정-골격-depth-2-sub-area") {
		t.Fatalf("project_create description should reference the governing Decision slug")
	}
}

// TestProjectCreateDescriptionRequiresExplicitImmutableLanguage locks the
// language-handling guidance the description carries. The wording is the
// agent's primary cue to ASK the user before calling — losing it leads
// to silent en defaults (Decision project_create primary_language 강한
// 포획). The corresponding runtime enforcement lives in
// projects.NormalizeLanguage; see projects/create_test.go.
func TestProjectCreateDescriptionRequiresExplicitImmutableLanguage(t *testing.T) {
	for _, want := range []string{
		"primary_language is required",
		"No default",
		"Supported languages are en, ko, ja",
		"immutable after creation",
		"recreate the project",
	} {
		if !strings.Contains(projectCreateDescription, want) {
			t.Fatalf("project_create description missing locale guidance %q", want)
		}
	}
}
