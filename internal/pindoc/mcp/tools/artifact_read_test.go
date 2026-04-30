package tools

import (
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

func TestNormalizeArtifactReadRef(t *testing.T) {
	cases := []struct {
		name         string
		raw          string
		want         string
		wantShare    bool
		wantMismatch bool
		projectSlug  string
	}{
		{
			name:        "bare slug",
			raw:         "sidecar-taskcontrols-meta-patch-shape",
			want:        "sidecar-taskcontrols-meta-patch-shape",
			projectSlug: "pindoc",
		},
		{
			name:        "bare UUID",
			raw:         "0b8562d8-4f2d-46e2-9f9a-8b0a6e1592a1",
			want:        "0b8562d8-4f2d-46e2-9f9a-8b0a6e1592a1",
			projectSlug: "pindoc",
		},
		{
			name:        "pindoc URL",
			raw:         "pindoc://sidecar-taskcontrols-meta-patch-shape",
			want:        "sidecar-taskcontrols-meta-patch-shape",
			projectSlug: "pindoc",
		},
		{
			name:        "canonical reader share path",
			raw:         "/p/pindoc/wiki/sidecar-taskcontrols-meta-patch-shape",
			want:        "sidecar-taskcontrols-meta-patch-shape",
			wantShare:   true,
			projectSlug: "pindoc",
		},
		{
			name:        "absolute canonical reader share URL",
			raw:         "http://localhost:5830/p/pindoc/wiki/sidecar-taskcontrols-meta-patch-shape",
			want:        "sidecar-taskcontrols-meta-patch-shape",
			wantShare:   true,
			projectSlug: "pindoc",
		},
		{
			name:        "canonical reader history URL",
			raw:         "/p/pindoc/wiki/sidecar-taskcontrols-meta-patch-shape/history",
			want:        "sidecar-taskcontrols-meta-patch-shape",
			wantShare:   true,
			projectSlug: "pindoc",
		},
		{
			name:        "legacy locale reader share path",
			raw:         "/p/pindoc/ko/wiki/sidecar-taskcontrols-meta-patch-shape",
			want:        "sidecar-taskcontrols-meta-patch-shape",
			wantShare:   true,
			projectSlug: "pindoc",
		},
		{
			name:        "legacy /a URL",
			raw:         "https://example.test/a/0b8562d8-4f2d-46e2-9f9a-8b0a6e1592a1",
			want:        "0b8562d8-4f2d-46e2-9f9a-8b0a6e1592a1",
			wantShare:   true,
			projectSlug: "pindoc",
		},
		{
			name:         "other project scope mismatch",
			raw:          "/p/other/wiki/sidecar-taskcontrols-meta-patch-shape",
			want:         "sidecar-taskcontrols-meta-patch-shape",
			wantShare:    true,
			wantMismatch: true,
			projectSlug:  "pindoc",
		},
		{
			name:        "legacy other locale no longer mismatches scope",
			raw:         "/p/pindoc/en/wiki/sidecar-taskcontrols-meta-patch-shape",
			want:        "sidecar-taskcontrols-meta-patch-shape",
			wantShare:   true,
			projectSlug: "pindoc",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeArtifactReadRef(tc.raw, tc.projectSlug)
			if got.Value != tc.want {
				t.Fatalf("Value = %q; want %q", got.Value, tc.want)
			}
			if got.LooksLikeShareURL != tc.wantShare {
				t.Fatalf("LooksLikeShareURL = %v; want %v", got.LooksLikeShareURL, tc.wantShare)
			}
			if got.ScopeMismatch != tc.wantMismatch {
				t.Fatalf("ScopeMismatch = %v; want %v", got.ScopeMismatch, tc.wantMismatch)
			}
		})
	}
}

func TestArtifactReadNotFoundErrorHintsForShareURL(t *testing.T) {
	ref := normalizeArtifactReadRef(
		"/p/pindoc/wiki/missing-artifact",
		"pindoc",
	)

	err := artifactReadNotFoundError(
		"/p/pindoc/wiki/missing-artifact",
		&auth.ProjectScope{ProjectSlug: "pindoc", ProjectLocale: "ko"},
		ref,
	)
	msg := err.Error()
	for _, want := range []string{"share URL", "extracted slug", "missing-artifact"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

func TestBuildTaskAttentionNegativeTypeGateCoversNonTaskTypes(t *testing.T) {
	nonTaskTypes := []string{
		"Analysis",
		"Decision",
		"Glossary",
		"Feature",
		"APIEndpoint",
		"Screen",
		"DataModel",
		"Debug",
		"Flow",
		"TC",
	}
	for _, artifactType := range nonTaskTypes {
		t.Run(artifactType, func(t *testing.T) {
			got := buildTaskAttention(
				artifactType,
				"open",
				"agent:codex",
				"codex",
				&auth.Principal{AgentID: "codex"},
				"full",
				"ko",
				"pindoc",
				"some-artifact",
			)
			if got != nil {
				t.Fatalf("type=%s should not emit task_attention: %+v", artifactType, got)
			}
		})
	}
}

func TestBuildTaskAttentionGates(t *testing.T) {
	basePrincipal := &auth.Principal{AgentID: "codex"}

	t.Run("assignee affinity positive emits korean copy", func(t *testing.T) {
		got := buildTaskAttention("Task", "open", "agent:codex", "other-agent", basePrincipal, "full", "ko", "pindoc", "task-a")
		if got == nil {
			t.Fatal("expected task_attention")
		}
		if got.Code != "task_still_open" || got.Level != "info" {
			t.Fatalf("unexpected code/level: %+v", got)
		}
		want := "이 Task는 status=open. 작업이 끝났으면 acceptance 체크와 evidence를 정리한 뒤 pindoc.task.claim_done을 호출하고, 최종 응답 전 pindoc.task.done_check로 닫힘을 확인하세요."
		if got.Message != want {
			t.Fatalf("message = %q; want %q", got.Message, want)
		}
		if len(got.NextTools) != 2 ||
			got.NextTools[0].Tool != "pindoc.artifact.propose" ||
			got.NextTools[1].Tool != "pindoc.task.claim_done" {
			t.Fatalf("unexpected next_tools: %+v", got.NextTools)
		}
	})

	t.Run("latest revision author affinity positive emits english copy", func(t *testing.T) {
		got := buildTaskAttention("Task", "open", "agent:other", "codex", basePrincipal, "continuation", "en", "pindoc", "task-a")
		if got == nil {
			t.Fatal("expected task_attention")
		}
		want := "This Task is still open. If you're done, update acceptance/evidence, call pindoc.task.claim_done, then run pindoc.task.done_check before final handoff."
		if got.Message != want {
			t.Fatalf("message = %q; want %q", got.Message, want)
		}
	})

	t.Run("different caller has no attention", func(t *testing.T) {
		got := buildTaskAttention("Task", "open", "agent:claude-code", "claude-code", basePrincipal, "full", "ko", "pindoc", "task-a")
		if got != nil {
			t.Fatalf("different caller should not emit task_attention: %+v", got)
		}
	})

	t.Run("brief view has no attention", func(t *testing.T) {
		got := buildTaskAttention("Task", "open", "agent:codex", "other-agent", basePrincipal, "brief", "ko", "pindoc", "task-a")
		if got != nil {
			t.Fatalf("brief view should not emit task_attention: %+v", got)
		}
	})

	for _, status := range []string{"claimed_done", "blocked", "cancelled"} {
		t.Run("terminal status "+status, func(t *testing.T) {
			got := buildTaskAttention("Task", status, "agent:codex", "other-agent", basePrincipal, "full", "ko", "pindoc", "task-a")
			if got != nil {
				t.Fatalf("status=%s should not emit task_attention: %+v", status, got)
			}
		})
	}

	for _, caller := range []string{"", "user:alice", "@alice"} {
		t.Run("human caller "+caller, func(t *testing.T) {
			got := buildTaskAttention("Task", "open", caller, caller, &auth.Principal{AgentID: caller}, "full", "ko", "pindoc", "task-a")
			if got != nil {
				t.Fatalf("human caller=%q should not emit task_attention: %+v", caller, got)
			}
		})
	}
}

func TestTaskAttentionTaskMetaFields(t *testing.T) {
	status, assignee := taskAttentionTaskMetaFields([]byte(`{"status":" open ","assignee":" agent:codex "}`))
	if status != "open" || assignee != "agent:codex" {
		t.Fatalf("status/assignee = %q/%q", status, assignee)
	}

	status, assignee = taskAttentionTaskMetaFields([]byte(`{malformed`))
	if status != "" || assignee != "" {
		t.Fatalf("malformed task_meta should return empty fields, got %q/%q", status, assignee)
	}
}
