package diff

import (
	"encoding/json"
	"testing"
)

func TestMetaDeltaForRangeTracksShapePayloadAndSnapshots(t *testing.T) {
	snaps := []RevisionMetaSnapshot{
		{
			RevisionNumber: 1,
			Tags:           []string{"mcp"},
			Completeness:   "partial",
			Shape:          "body_patch",
		},
		{
			RevisionNumber: 2,
			Tags:           []string{"mcp"},
			Completeness:   "partial",
			Shape:          "meta_patch",
			ShapePayload:   json.RawMessage(`{"task_meta":{"assignee":"agent:codex","priority":"p1"}}`),
		},
		{
			RevisionNumber: 3,
			Tags:           []string{"mcp", "ui"},
			Completeness:   "settled",
			Shape:          "body_patch",
		},
		{
			RevisionNumber: 4,
			Tags:           []string{"mcp", "ui"},
			Completeness:   "settled",
			Shape:          "meta_patch",
			ShapePayload:   json.RawMessage(`{"task_meta":{"assignee":"agent:reviewer"}}`),
		},
	}

	got := MetaDeltaForRange(1, 4, snaps)
	byKey := map[string]MetaDeltaEntry{}
	for _, entry := range got {
		byKey[entry.Key] = entry
	}

	if byKey["completeness"].Before != "partial" || byKey["completeness"].After != "settled" {
		t.Fatalf("completeness delta = %#v", byKey["completeness"])
	}
	if byKey["task_meta.assignee"].Before != nil || byKey["task_meta.assignee"].After != "agent:reviewer" {
		t.Fatalf("assignee delta = %#v", byKey["task_meta.assignee"])
	}
	if byKey["task_meta.priority"].Before != nil || byKey["task_meta.priority"].After != "p1" {
		t.Fatalf("priority delta = %#v", byKey["task_meta.priority"])
	}
}

func TestClassifyRevisionType(t *testing.T) {
	cases := []struct {
		name        string
		shape       string
		commit      string
		bodyChanged bool
		metaChanged bool
		want        string
	}{
		{name: "text", shape: "body_patch", bodyChanged: true, want: RevisionTypeTextEdit},
		{name: "acceptance", shape: "acceptance_transition", bodyChanged: true, want: RevisionTypeAcceptanceToggle},
		{name: "meta", shape: "meta_patch", metaChanged: true, want: RevisionTypeMetaChange},
		{name: "system", shape: "meta_patch", commit: "Repair: normalize task meta", metaChanged: true, want: RevisionTypeSystemAuto},
		{name: "mixed", shape: "body_patch", bodyChanged: true, metaChanged: true, want: RevisionTypeMixed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyRevisionType(tc.shape, tc.commit, tc.bodyChanged, tc.metaChanged)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
