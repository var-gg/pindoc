package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	got, changed = toolsetDrift(fmt.Sprintf("%d:stale", len(RegisteredTools)), ToolsetVersion())
	if got == nil || !*got {
		t.Fatalf("same-count stale hash requires_resync = %v; want true", got)
	}
	if len(changed) != 0 {
		t.Fatalf("same-count schema drift should not invent new tools: %v", changed)
	}
}

func TestToolsetDriftClientActions(t *testing.T) {
	actions := toolsetDriftClientActions("0:stale")
	if len(actions) != 3 {
		t.Fatalf("actions len = %d, want 3: %+v", len(actions), actions)
	}
	wantIDs := []string{"call_runtime_status", "refresh_tool_search", "restart_mcp_session"}
	for i, want := range wantIDs {
		if actions[i].ID != want {
			t.Fatalf("action[%d].id = %q, want %q", i, actions[i].ID, want)
		}
	}
	if actions[0].Tool != "pindoc.runtime.status" || actions[0].Args["client_toolset_hash"] != "0:stale" {
		t.Fatalf("runtime.status action = %+v", actions[0])
	}
	if actions[1].Action != "tool_search" || !strings.Contains(actions[1].Reason, "deferred Pindoc tool metadata") {
		t.Fatalf("tool_search action = %+v", actions[1])
	}
	if actions[2].Action != "restart_session" || !strings.Contains(actions[2].Reason, "Reconnect") {
		t.Fatalf("restart action = %+v", actions[2])
	}
}

func TestDetectHarnessDrift(t *testing.T) {
	t.Run("missing PINDOC.md", func(t *testing.T) {
		dir := t.TempDir()
		hint := detectHarnessDrift(dir, "pindoc")
		if hint == nil || !hint.Detected {
			t.Fatalf("missing PINDOC.md should be detected: %+v", hint)
		}
		if hint.Code != harnessDriftCodeMissingPindoc {
			t.Fatalf("missing PINDOC.md code = %q, want %q", hint.Code, harnessDriftCodeMissingPindoc)
		}
		if hint.SuggestedCall != "pindoc.harness.install" {
			t.Fatalf("suggested_call = %q", hint.SuggestedCall)
		}
		if hint.Severity != "info" {
			t.Fatalf("missing PINDOC.md severity = %q, want info", hint.Severity)
		}
	})

	t.Run("matching frontmatter", func(t *testing.T) {
		dir := t.TempDir()
		writeTestPindoc(t, dir, "project_slug: pindoc\nschema_version: 1\n")
		hint := detectHarnessDrift(dir, "pindoc")
		if hint == nil || hint.Detected {
			t.Fatalf("matching PINDOC.md should not drift: %+v", hint)
		}
		if hint.SuggestedCall != harnessSuggestedNone {
			t.Fatalf("matching PINDOC.md suggested_call = %q, want none", hint.SuggestedCall)
		}
	})

	t.Run("project slug mismatch", func(t *testing.T) {
		dir := t.TempDir()
		writeTestPindoc(t, dir, "project_slug: other\nschema_version: 1\n")
		hint := detectHarnessDrift(dir, "pindoc")
		if hint == nil || !hint.Detected || hint.FoundProjectSlug != "other" {
			t.Fatalf("mismatch should be detected with found slug: %+v", hint)
		}
		if !strings.Contains(hint.Reason, "expected project_slug") {
			t.Fatalf("mismatch reason should mention expected project_slug: %q", hint.Reason)
		}
		if hint.Severity != "blocking" {
			t.Fatalf("mismatch severity = %q, want blocking", hint.Severity)
		}
		if hint.Code != harnessDriftCodeProjectSlugMismatch {
			t.Fatalf("mismatch code = %q, want %q", hint.Code, harnessDriftCodeProjectSlugMismatch)
		}
	})

	t.Run("missing schema version", func(t *testing.T) {
		dir := t.TempDir()
		writeTestPindoc(t, dir, "project_slug: pindoc\n")
		hint := detectHarnessDrift(dir, "pindoc")
		if hint == nil || !hint.Detected || !strings.Contains(hint.Reason, "schema_version") {
			t.Fatalf("missing schema_version should be detected: %+v", hint)
		}
	})
}

func TestEvaluateHarnessDriftClientInputs(t *testing.T) {
	t.Run("current PINDOC.md body wins over unobservable working_directory", func(t *testing.T) {
		unobservableDir := filepath.Join(t.TempDir(), "not-mounted", "project")
		hints, clean := evaluateHarnessDrift(harnessDriftProbe{
			WorkingDirectory:    unobservableDir,
			ExpectedProjectSlug: "pindoc",
			CurrentPindocMD:     testPindocBody("project_slug: pindoc\nschema_version: 1\n"),
			Frontmatter: &pingPindocFrontmatter{
				ProjectSlug:   "other",
				SchemaVersion: "1",
			},
		})
		if len(hints) != 0 {
			t.Fatalf("client body should avoid server filesystem drift hints: %+v", hints)
		}
		if clean.Detected || clean.Source != harnessDriftSourceClientBody {
			t.Fatalf("clean hint = %+v; want client body non-drift", clean)
		}
		if clean.FoundProjectSlug != "pindoc" || clean.SchemaVersion != "1" {
			t.Fatalf("clean frontmatter = slug %q schema %q", clean.FoundProjectSlug, clean.SchemaVersion)
		}
	})

	t.Run("frontmatter wins over unobservable working_directory", func(t *testing.T) {
		unobservableDir := filepath.Join(t.TempDir(), "not-mounted", "project")
		hints, clean := evaluateHarnessDrift(harnessDriftProbe{
			WorkingDirectory:    unobservableDir,
			ExpectedProjectSlug: "pindoc",
			Frontmatter: &pingPindocFrontmatter{
				ProjectSlug:   "pindoc",
				SchemaVersion: "1",
			},
		})
		if len(hints) != 0 {
			t.Fatalf("client frontmatter should avoid server filesystem drift hints: %+v", hints)
		}
		if clean.Detected || clean.Source != harnessDriftSourceClientFrontmatter {
			t.Fatalf("clean hint = %+v; want client frontmatter non-drift", clean)
		}
	})

	t.Run("frontmatter project slug mismatch is blocking", func(t *testing.T) {
		hints, _ := evaluateHarnessDrift(harnessDriftProbe{
			ExpectedProjectSlug: "pindoc",
			Frontmatter: &pingPindocFrontmatter{
				ProjectSlug:   "other",
				SchemaVersion: "1",
			},
		})
		if len(hints) != 1 {
			t.Fatalf("hints len = %d, want 1: %+v", len(hints), hints)
		}
		if hints[0].Severity != "blocking" || hints[0].Code != harnessDriftCodeProjectSlugMismatch {
			t.Fatalf("mismatch hint = %+v", hints[0])
		}
		if hints[0].SuggestedCall != harnessSuggestedHarnessInstallClient {
			t.Fatalf("suggested_call = %q, want %q", hints[0].SuggestedCall, harnessSuggestedHarnessInstallClient)
		}
	})
}

func TestEvaluateHarnessDriftServerObservation(t *testing.T) {
	t.Run("nonexistent working_directory is unobservable not missing PINDOC.md", func(t *testing.T) {
		unobservableDir := filepath.Join(t.TempDir(), "not-mounted", "project")
		hint := detectHarnessDrift(unobservableDir, "pindoc")
		if hint == nil || !hint.Detected {
			t.Fatalf("unobservable workspace should produce diagnostic hint: %+v", hint)
		}
		if hint.Code != harnessDriftCodeServerUnobservable {
			t.Fatalf("unobservable code = %q, want %q", hint.Code, harnessDriftCodeServerUnobservable)
		}
		if strings.Contains(hint.Reason, "missing in the workspace root") {
			t.Fatalf("unobservable reason must not claim PINDOC.md is missing: %q", hint.Reason)
		}
		if hint.SuggestedCall != harnessSuggestedWorkspaceDetect {
			t.Fatalf("suggested_call = %q, want %q", hint.SuggestedCall, harnessSuggestedWorkspaceDetect)
		}
	})

	t.Run("working_directory file is unreadable root diagnostic", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "not-a-dir")
		if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("write not-a-dir: %v", err)
		}
		hint := detectHarnessDrift(path, "pindoc")
		if hint == nil || !hint.Detected {
			t.Fatalf("file workspace should produce diagnostic hint: %+v", hint)
		}
		if hint.Code != harnessDriftCodeRootUnreadable {
			t.Fatalf("root unreadable code = %q, want %q", hint.Code, harnessDriftCodeRootUnreadable)
		}
		if hint.SuggestedCall != harnessSuggestedWorkspaceDetect {
			t.Fatalf("suggested_call = %q, want %q", hint.SuggestedCall, harnessSuggestedWorkspaceDetect)
		}
	})

	t.Run("unreadable PINDOC.md is not unobservable", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "PINDOC.md"), 0o700); err != nil {
			t.Fatalf("mkdir PINDOC.md: %v", err)
		}
		hint := detectHarnessDrift(dir, "pindoc")
		if hint == nil || !hint.Detected {
			t.Fatalf("directory PINDOC.md should produce diagnostic hint: %+v", hint)
		}
		if hint.Code != harnessDriftCodeUnreadablePindoc {
			t.Fatalf("unreadable PINDOC.md code = %q, want %q", hint.Code, harnessDriftCodeUnreadablePindoc)
		}
		if hint.SuggestedCall != harnessSuggestedHarnessInstall {
			t.Fatalf("suggested_call = %q, want %q", hint.SuggestedCall, harnessSuggestedHarnessInstall)
		}
	})
}

func TestPindocMDPathPreservesWindowsStyleSeparators(t *testing.T) {
	got := pindocMDPath(`A:\vargg-workspace\survival-manager`)
	want := `A:\vargg-workspace\survival-manager\PINDOC.md`
	if got != want {
		t.Fatalf("pindocMDPath windows = %q, want %q", got, want)
	}

	got = pindocMDPath(`A:\vargg-workspace/survival-manager/`)
	want = `A:\vargg-workspace\survival-manager\PINDOC.md`
	if got != want {
		t.Fatalf("pindocMDPath mixed = %q, want %q", got, want)
	}
}

func TestDetectHarnessDriftsSortsSeverity(t *testing.T) {
	dir := t.TempDir()
	writeTestPindoc(t, dir, "project_slug: other\n")
	hints := detectHarnessDrifts(dir, "pindoc")
	if len(hints) != 2 {
		t.Fatalf("hints len = %d, want 2: %+v", len(hints), hints)
	}
	if hints[0].Severity != "blocking" || hints[1].Severity != "info" {
		t.Fatalf("severity order = %q, %q; want blocking, info", hints[0].Severity, hints[1].Severity)
	}
	if !harnessDriftBlocked(hints) {
		t.Fatalf("blocking hint should set harness_blocked")
	}
}

func writeTestPindoc(t *testing.T, dir, frontmatter string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "PINDOC.md"), []byte(testPindocBody(frontmatter)), 0o600); err != nil {
		t.Fatalf("write PINDOC.md: %v", err)
	}
}

func testPindocBody(frontmatter string) string {
	return "---\n" + frontmatter + "---\n\n# PINDOC.md\n"
}
