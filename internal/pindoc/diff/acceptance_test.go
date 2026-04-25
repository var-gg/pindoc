package diff

import (
	"encoding/json"
	"testing"
)

func TestAcceptanceChecklistSummaryUsesTransitionPayload(t *testing.T) {
	before := "- [ ] first\n- [ ] second\n- [~] third\n"
	after := "- [ ] first\n- [x] second\n- [~] third\n"
	payload := json.RawMessage(`{"checkbox_index":1,"from_state":" ","new_state":"[x]","reason":"done by test"}`)

	got := AcceptanceChecklistSummary(before, after, "acceptance_transition", payload)
	if !got.HasChange {
		t.Fatal("expected HasChange")
	}
	if got.ChangedIndex == nil || *got.ChangedIndex != 1 {
		t.Fatalf("changed index = %#v", got.ChangedIndex)
	}
	if len(got.Items) != 3 {
		t.Fatalf("items = %d", len(got.Items))
	}
	item := got.Items[1]
	if !item.Changed || item.FromState != "[ ]" || item.ToState != "[x]" || item.Reason != "done by test" {
		t.Fatalf("changed item = %#v", item)
	}
}

func TestAcceptanceChecklistSummaryFallsBackToStateDiff(t *testing.T) {
	before := "- [ ] first\n- [ ] second\n"
	after := "- [ ] first\n- [-] second\n"

	got := AcceptanceChecklistSummary(before, after, "body_patch", nil)
	if !got.HasChange {
		t.Fatal("expected HasChange")
	}
	item := got.Items[1]
	if !item.Changed || item.FromState != "[ ]" || item.ToState != "[-]" {
		t.Fatalf("changed item = %#v", item)
	}
}
