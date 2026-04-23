package tools

import (
	"reflect"
	"testing"
)

// TestMetaFieldsChangedList asserts the stable sort contract the
// artifact.meta_patched event depends on. Downstream dashboards query
// fields_changed ordering, so a flap there ripples into their cache keys.
func TestMetaFieldsChangedList(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		want    []string
	}{
		{
			name:    "empty payload returns empty slice",
			payload: map[string]any{},
			want:    []string{},
		},
		{
			name:    "single field",
			payload: map[string]any{"tags": []string{"x"}},
			want:    []string{"tags"},
		},
		{
			name: "alphabetic order",
			payload: map[string]any{
				"tags":          nil,
				"artifact_meta": nil,
				"task_meta":     nil,
				"completeness":  "",
			},
			want: []string{"artifact_meta", "completeness", "tags", "task_meta"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := metaFieldsChangedList(tc.payload)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("metaFieldsChangedList\n  got:  %v\n  want: %v", got, tc.want)
			}
		})
	}
}

// TestPatchFieldsForMetaPatch asserts the retry hints for the two new
// meta_patch error codes so agents know exactly what to patch.
func TestPatchFieldsForMetaPatch(t *testing.T) {
	got := patchFieldsFor("META_PATCH_HAS_BODY")
	wantBody := []string{"body_markdown", "body_patch", "shape"}
	if !reflect.DeepEqual(got, wantBody) {
		t.Fatalf("META_PATCH_HAS_BODY patchable=%v want=%v", got, wantBody)
	}
	got = patchFieldsFor("META_PATCH_EMPTY")
	wantEmpty := []string{"tags", "completeness", "task_meta", "artifact_meta"}
	if !reflect.DeepEqual(got, wantEmpty) {
		t.Fatalf("META_PATCH_EMPTY patchable=%v want=%v", got, wantEmpty)
	}
}
