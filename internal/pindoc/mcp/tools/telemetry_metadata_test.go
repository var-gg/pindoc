package tools

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestExtractToolMetadataWorkspaceDetect asserts the workspace.detect
// extractor pulls "via" out of the output and skips the field when it
// is absent (older response shape, or pre-detect failure).
func TestExtractToolMetadataWorkspaceDetect(t *testing.T) {
	outputJSON := []byte(`{"project_slug":"pindoc","via":"git_remote_url"}`)
	got := extractToolMetadata("pindoc.workspace.detect", struct{}{}, outputJSON)
	want := map[string]any{"via": "git_remote_url"}
	assertJSON(t, got, want)

	empty := extractToolMetadata("pindoc.workspace.detect", struct{}{}, []byte(`{"project_slug":"pindoc"}`))
	if empty != nil {
		t.Fatalf("expected nil payload when via missing, got %s", string(empty))
	}
}

// TestExtractToolMetadataAreaList confirms the include_templates input
// flag is recorded — both the true and the (default) false case so the
// "people are turning this on" trend stays queryable.
func TestExtractToolMetadataAreaList(t *testing.T) {
	type areaListInput struct {
		ProjectSlug      string
		IncludeTemplates bool
	}

	got := extractToolMetadata("pindoc.area.list", areaListInput{IncludeTemplates: true}, nil)
	assertJSON(t, got, map[string]any{"include_templates": true})

	got = extractToolMetadata("pindoc.area.list", areaListInput{IncludeTemplates: false}, nil)
	assertJSON(t, got, map[string]any{"include_templates": false})
}

// TestExtractToolMetadataArtifactPropose covers the propose dimensions
// the codex DX feedback called out: shape (body_patch / meta_patch /
// acceptance_transition / scope_defer) and type (Decision / Task / …),
// plus area_slug for "where do new artifacts land" trends.
func TestExtractToolMetadataArtifactPropose(t *testing.T) {
	type proposeInput struct {
		ProjectSlug string
		Type        string
		AreaSlug    string
		Shape       string
	}

	got := extractToolMetadata("pindoc.artifact.propose",
		proposeInput{Type: "Task", AreaSlug: "ui", Shape: "body_patch"}, nil)
	assertJSON(t, got, map[string]any{
		"artifact_type": "Task",
		"area_slug":     "ui",
		"shape":         "body_patch",
	})

	// Empty fields drop out so the metadata payload stays sparse.
	got = extractToolMetadata("pindoc.artifact.propose", proposeInput{}, nil)
	if got != nil {
		t.Fatalf("expected nil for empty propose input, got %s", string(got))
	}
}

// TestExtractToolMetadataArtifactSearch records the "how is search
// being driven?" dimensions — top_k, include_templates flag, and the
// hit count returned by the server.
func TestExtractToolMetadataArtifactSearch(t *testing.T) {
	type searchInput struct {
		ProjectSlug      string
		Query            string
		TopK             int
		IncludeTemplates bool
	}

	output := []byte(`{"hits":[{"slug":"a"},{"slug":"b"},{"slug":"c"}]}`)
	got := extractToolMetadata("pindoc.artifact.search",
		searchInput{TopK: 5, IncludeTemplates: false}, output)
	assertJSON(t, got, map[string]any{
		"top_k":             float64(5), // json round-trips numbers as float64
		"include_templates": false,
		"hits_count":        float64(3),
	})

	// Zero hits still records hits_count=0 so empty searches are visible.
	emptyOut := []byte(`{"hits":[]}`)
	got = extractToolMetadata("pindoc.artifact.search",
		searchInput{TopK: 0, IncludeTemplates: false}, emptyOut)
	assertJSON(t, got, map[string]any{
		"include_templates": false,
		"hits_count":        float64(0),
	})
}

// TestExtractToolMetadataUnknownTool returns nil for any tool that
// doesn't have an extractor wired up — the row defaults to '{}' in the
// DB so the column stays NOT NULL safe.
func TestExtractToolMetadataUnknownTool(t *testing.T) {
	got := extractToolMetadata("pindoc.something.else", struct{}{}, []byte(`{}`))
	if got != nil {
		t.Fatalf("expected nil for unknown tool, got %s", string(got))
	}
}

// assertJSON compares a serialised payload to the expected map by
// re-parsing the bytes — direct map equality would trip on numeric
// type drift between int and float64 once JSON round-trips.
func assertJSON(t *testing.T, raw json.RawMessage, want map[string]any) {
	t.Helper()
	if raw == nil {
		t.Fatalf("metadata payload is nil; want %v", want)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v (raw=%s)", err, string(raw))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata mismatch:\n got:  %v\n want: %v", got, want)
	}
}
