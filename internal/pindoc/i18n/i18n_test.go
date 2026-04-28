package i18n

import (
	"strings"
	"testing"
)

// TestTaskStatusViaTransitionToolMentionsInterimPath keeps the runtime
// guidance aligned with the actual tool surface: task.transition is not
// registered yet, so the message must point agents at claim_done /
// body_patch instead of a non-existent shortcut.
func TestTaskStatusViaTransitionToolMentionsInterimPath(t *testing.T) {
	en := T("en", "preflight.task_status_via_transition_tool")
	for _, needle := range []string{
		"pindoc.task.claim_done",
		"shape=body_patch",
		"not available yet",
	} {
		if !strings.Contains(en, needle) {
			t.Fatalf("english message missing %q: %q", needle, en)
		}
	}

	ko := T("ko", "preflight.task_status_via_transition_tool")
	for _, needle := range []string{
		"pindoc.task.claim_done",
		"shape=body_patch",
		"아직 구현되지 않았",
	} {
		if !strings.Contains(ko, needle) {
			t.Fatalf("korean message missing %q: %q", needle, ko)
		}
	}
}
