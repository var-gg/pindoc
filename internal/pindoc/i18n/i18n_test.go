package i18n

import (
	"strings"
	"testing"
)

// TestTaskStatusViaTransitionToolMentionsInterimPath keeps the runtime
// guidance aligned with the actual tool surface: task.transition is not
// registered yet, so the message must point agents at body_patch /
// artifact.verify instead of a non-existent shortcut.
func TestTaskStatusViaTransitionToolMentionsInterimPath(t *testing.T) {
	en := T("en", "preflight.task_status_via_transition_tool")
	for _, needle := range []string{
		"shape=body_patch",
		"pindoc.artifact.verify",
		"not available yet",
	} {
		if !strings.Contains(en, needle) {
			t.Fatalf("english message missing %q: %q", needle, en)
		}
	}

	ko := T("ko", "preflight.task_status_via_transition_tool")
	for _, needle := range []string{
		"shape=body_patch",
		"pindoc.artifact.verify",
		"아직 구현되지 않았",
	} {
		if !strings.Contains(ko, needle) {
			t.Fatalf("korean message missing %q: %q", needle, ko)
		}
	}
}
