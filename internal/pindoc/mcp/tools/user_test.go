package tools

import (
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

func TestClassifyUserCurrentIdentity(t *testing.T) {
	t.Run("missing user identity is informational", func(t *testing.T) {
		got, handled := classifyUserCurrentIdentity(&auth.Principal{
			AgentID: "agent:codex",
		})
		if !handled {
			t.Fatal("expected missing user to be handled")
		}
		if got.Status != "informational" {
			t.Fatalf("status got=%q want=informational", got.Status)
		}
		if got.Code != "USER_NOT_SET" {
			t.Fatalf("code got=%q want=USER_NOT_SET", got.Code)
		}
		if len(got.Failed) != 0 || len(got.Checklist) != 0 || got.ErrorCode != "" {
			t.Fatalf("informational response should not use not_ready fields: %+v", got)
		}
		joined := strings.Join(got.Hints, " ")
		for _, want := range []string{"PINDOC_USER_NAME", "artifact.propose", "author_id"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("informational hints missing %q: %v", want, got.Hints)
			}
		}
	})

	t.Run("missing agent identity remains not_ready", func(t *testing.T) {
		got, handled := classifyUserCurrentIdentity(&auth.Principal{})
		if !handled {
			t.Fatal("expected missing agent to be handled")
		}
		if got.Status != "not_ready" || got.ErrorCode != "AGENT_NOT_SET" {
			t.Fatalf("got status/code %+v", got)
		}
		if len(got.Failed) != 1 || got.Failed[0] != "AGENT_NOT_SET" {
			t.Fatalf("failed[] got %v", got.Failed)
		}
	})

	t.Run("bound user falls through to db load", func(t *testing.T) {
		_, handled := classifyUserCurrentIdentity(&auth.Principal{
			UserID:  "user-1",
			AgentID: "agent:codex",
		})
		if handled {
			t.Fatal("bound user should fall through to loadUserByID")
		}
	})
}
