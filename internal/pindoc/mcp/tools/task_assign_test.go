package tools

import "testing"

// TestValidateAssignee covers the three principal shapes defined by
// Decision task-operation-tools-task-assign-단건-task-bulk-assign-배치-reas
// plus the "empty = explicit clear" escape hatch and rejection of
// malformed strings. Database is not touched — this is pure input
// validation and belongs next to the regex definition in task_assign.go.
func TestValidateAssignee(t *testing.T) {
	cases := []struct {
		in       string
		want     string
		wantOK   bool
		wantName string
	}{
		{"", "", true, "empty-clear"},
		{"   ", "", true, "whitespace-clear"},
		{"agent:codex", "agent:codex", true, "agent-plain"},
		{"agent:claude-code", "agent:claude-code", true, "agent-with-dash"},
		{"agent:codex:session-42", "agent:codex:session-42", true, "agent-with-colon"},
		{"user:6f676d29-0d22-43fa-87af", "user:6f676d29-0d22-43fa-87af", true, "user-uuid-prefix"},
		{"@alice", "@alice", true, "handle-plain"},
		{"@alice.bob", "@alice.bob", true, "handle-with-dot"},
		{"not_a_principal", "", false, "no-scheme"},
		{"agent:", "", false, "agent-empty-id"},
		{"@", "", false, "handle-empty"},
		{"claude-code", "", false, "missing-scheme"},
		{"http://example.com", "", false, "url-shape"},
	}
	for _, c := range cases {
		t.Run(c.wantName, func(t *testing.T) {
			got, ok := validateAssignee(c.in)
			if ok != c.wantOK {
				t.Fatalf("validateAssignee(%q): ok = %v, want %v", c.in, ok, c.wantOK)
			}
			if got != c.want {
				t.Fatalf("validateAssignee(%q): got %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestNewBulkOpID confirms the correlation token is a 32-char hex string
// and that two consecutive calls produce distinct values. Not a
// cryptographic test; just guarding against a regression where the
// function returns a constant or a wrong length.
func TestNewBulkOpID(t *testing.T) {
	a, err := newBulkOpID()
	if err != nil {
		t.Fatalf("newBulkOpID: %v", err)
	}
	if len(a) != 32 {
		t.Fatalf("newBulkOpID length: got %d, want 32", len(a))
	}
	for _, r := range a {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !isHex {
			t.Fatalf("newBulkOpID contains non-hex rune: %q in %q", r, a)
		}
	}
	b, err := newBulkOpID()
	if err != nil {
		t.Fatalf("newBulkOpID (second call): %v", err)
	}
	if a == b {
		t.Fatalf("newBulkOpID returned identical values twice: %q", a)
	}
}

func TestTaskBulkReassignBlocked(t *testing.T) {
	cases := []struct {
		name          string
		current       string
		next          string
		allowReassign bool
		wantBlocked   bool
	}{
		{name: "unassigned claim", current: "", next: "agent:codex", wantBlocked: false},
		{name: "same assignee idempotent", current: "agent:codex", next: "agent:codex", wantBlocked: false},
		{name: "different assignee blocked", current: "agent:claude", next: "agent:codex", wantBlocked: true},
		{name: "clear assigned blocked", current: "user:user-1", next: "", wantBlocked: true},
		{name: "explicit reassign allowed", current: "agent:claude", next: "agent:codex", allowReassign: true, wantBlocked: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := taskBulkReassignBlocked(c.current, c.next, c.allowReassign)
			if got != c.wantBlocked {
				t.Fatalf("taskBulkReassignBlocked(%q, %q, %v) = %v, want %v",
					c.current, c.next, c.allowReassign, got, c.wantBlocked)
			}
		})
	}
}
