package tools

import "testing"

func TestToolsetDrift(t *testing.T) {
	if got, _ := toolsetDrift("", ToolsetVersion()); got != nil {
		t.Fatalf("missing client hash should produce nil requires_resync")
	}

	got, changed := toolsetDrift(ToolsetVersion(), ToolsetVersion())
	if got == nil || *got {
		t.Fatalf("matching hash requires_resync = %v; want false", got)
	}
	if len(changed) != 0 {
		t.Fatalf("matching hash changed tools = %v; want empty", changed)
	}

	got, changed = toolsetDrift("0:stale", ToolsetVersion())
	if got == nil || !*got {
		t.Fatalf("stale hash requires_resync = %v; want true", got)
	}
	if len(changed) == 0 {
		t.Fatalf("stale hash should surface changed tool names")
	}
}
