package tools

import (
	"os"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

func TestCallerAgentAssigneeGate(t *testing.T) {
	if got := callerAgentAssignee(&auth.Principal{AgentID: "codex"}); got != "agent:codex" {
		t.Fatalf("callerAgentAssignee = %q", got)
	}
	for _, id := range []string{"", "user:abc", "@alice"} {
		if got := callerAgentAssignee(&auth.Principal{AgentID: id}); got != "" {
			t.Fatalf("human caller %q should be gated, got %q", id, got)
		}
	}
}

func TestStuckThresholdHours(t *testing.T) {
	t.Setenv("PINDOC_STUCK_THRESHOLD_HOURS", "12")
	if got := stuckThresholdHours(); got != 12 {
		t.Fatalf("threshold = %d, want 12", got)
	}
	_ = os.Unsetenv("PINDOC_STUCK_THRESHOLD_HOURS")
	if got := stuckThresholdHours(); got != defaultStuckThresholdHours {
		t.Fatalf("default threshold = %d", got)
	}
}
