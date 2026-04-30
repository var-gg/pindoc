package tools

import (
	"reflect"
	"strings"
	"testing"
)

// TestDefaultHarnessSessionBootstrapContract pins the machine-readable
// bootstrap contract clients depend on. Bumping any of these strings
// breaks claude-code / codex installs that have already shipped, so a
// regression test catches accidental drift before it lands.
func TestDefaultHarnessSessionBootstrapContract(t *testing.T) {
	got := defaultHarnessSessionBootstrap()
	if got == nil {
		t.Fatal("defaultHarnessSessionBootstrap returned nil")
	}

	wantAuto := []string{"pindoc.workspace.detect", "pindoc.task.queue"}
	if !reflect.DeepEqual(got.AutoCall, wantAuto) {
		t.Fatalf("AutoCall: got %v, want %v", got.AutoCall, wantAuto)
	}

	if got.CacheKeyForWorkspaceDetect != "pindoc.session.default_project_slug" {
		t.Fatalf("cache key drift: %q", got.CacheKeyForWorkspaceDetect)
	}

	wantSignals := []string{"pindoc_md_frontmatter", "workspace_path", "git_remote_url"}
	if !reflect.DeepEqual(got.SignalsFromClient, wantSignals) {
		t.Fatalf("SignalsFromClient: got %v, want %v", got.SignalsFromClient, wantSignals)
	}

	wantRerun := []string{"user_switched_workspace", "tool_returned_PROJECT_SLUG_REQUIRED"}
	if !reflect.DeepEqual(got.RerunOn, wantRerun) {
		t.Fatalf("RerunOn: got %v, want %v", got.RerunOn, wantRerun)
	}

	if got.ToolsetVersionCacheKey != "pindoc.session.toolset_version" {
		t.Fatalf("toolset cache key drift: %q", got.ToolsetVersionCacheKey)
	}
	if got.DriftCheckTool != "pindoc.runtime.status" {
		t.Fatalf("drift check tool = %q", got.DriftCheckTool)
	}
	if len(got.DriftActions) != 2 {
		t.Fatalf("DriftActions len = %d, want 2", len(got.DriftActions))
	}
	if got.SessionHandoffTemplate != "_template_session_handoff" {
		t.Fatalf("handoff template = %q", got.SessionHandoffTemplate)
	}

	if got.Notes == "" {
		t.Fatal("Notes should be a non-empty hint for human reviewers")
	}
}

// TestRenderPindocMDSessionBootstrapSection asserts the prose mirror of
// the contract is present and consistent with the machine-readable
// values. If a future edit drops one without the other, this test
// catches the divergence.
func TestRenderPindocMDSessionBootstrapSection(t *testing.T) {
	body := renderPindocMD("Pindoc", "00000000-0000-0000-0000-000000000000", "pindoc", "en", "en", "test", true)

	for _, want := range []string{
		"## Session bootstrap (workspace.detect + task.queue sweep)",
		"pindoc.workspace.detect",
		"pindoc.task.queue",
		"across_projects=true",
		"projects[slug].items",
		"total_assignee_open_count",
		"MULTI_PROJECT_WORKSPACE",
		"pin the specific project_slug",
		"PINDOC.md frontmatter",
		"workspace_path",
		"git_remote_url",
		"pindoc.session.default_project_slug",
		"PROJECT_SLUG_REQUIRED",
		"pindoc.session.toolset_version",
		"pindoc.runtime.status",
		"stale tool descriptions",
		"_template_session_handoff",
		"pindoc.task.done_check",
		"### When to re-run",
		"### Fallback when bootstrap is missing",
		"session_bootstrap object",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered PINDOC.md missing session bootstrap fragment %q", want)
		}
	}

	// Sanity check: the Pre-flight Check protocol still references the
	// auto-bootstrap so older Pre-flight readers don't try to re-detect.
	if !strings.Contains(body, "Pre-flight Check protocol") {
		t.Fatal("Pre-flight Check section should still exist")
	}
	if !strings.Contains(body, "Session bootstrap") {
		t.Fatal("Pre-flight should cross-reference Session bootstrap")
	}
}
