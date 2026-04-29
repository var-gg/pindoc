package tools

import (
	"strings"
	"testing"
)

// TestToolsetVersionStable asserts the version is deterministic across
// calls — agents would spuriously see "drift" between back-to-back
// responses otherwise.
func TestToolsetVersionStable(t *testing.T) {
	a := ToolsetVersion()
	b := ToolsetVersion()
	if a != b {
		t.Fatalf("ToolsetVersion not stable: %q vs %q", a, b)
	}
}

// TestToolsetVersionShape checks the "<count>:<hash8>" contract — the
// client-side drift detector may grep the prefix.
func TestToolsetVersionShape(t *testing.T) {
	v := ToolsetVersion()
	if !strings.Contains(v, ":") {
		t.Fatalf("version missing separator: %q", v)
	}
	parts := strings.SplitN(v, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("version shape wrong: %q", v)
	}
	if parts[0] == "" || parts[1] == "" {
		t.Fatalf("version parts empty: %q", v)
	}
	if len(parts[1]) != 8 {
		t.Fatalf("hash part should be 8 chars, got %q", parts[1])
	}
}

func TestToolsetSchemaVersionPresent(t *testing.T) {
	if strings.TrimSpace(ToolsetSchemaVersion) == "" {
		t.Fatalf("ToolsetSchemaVersion must be non-empty so same-name schema drift bumps toolset_version")
	}
}

// TestToolsetVersionChangesWithCatalog — swap a tool in/out and confirm
// the hash moves. This prevents "we added a tool but the hash didn't
// change because someone broke the hashing" regressions.
func TestToolsetVersionChangesWithCatalog(t *testing.T) {
	orig := RegisteredTools
	defer func() { RegisteredTools = orig }()

	before := ToolsetVersion()
	RegisteredTools = append([]string{}, orig...)
	RegisteredTools = append(RegisteredTools, "pindoc.synthetic.new_tool")
	after := ToolsetVersion()
	if before == after {
		t.Fatalf("adding a tool did not change version: before=%q after=%q", before, after)
	}
}
