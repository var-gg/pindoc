package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinPathWarningsDistinguishRelativeAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	existing := filepath.Join(root, "docs", "existing.md")
	if err := os.WriteFile(existing, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	deps := Deps{RepoRoot: root}
	if got := pinPathWarnings(deps, []ArtifactPinInput{{Path: "docs/existing.md", Kind: "doc"}}); len(got) != 0 {
		t.Fatalf("relative existing path warned: %v", got)
	}
	if got := pinPathWarnings(deps, []ArtifactPinInput{{Path: existing, Kind: "doc"}}); len(got) != 0 {
		t.Fatalf("absolute path inside repo should normalize without warning: %v", got)
	}

	missing := pinPathWarnings(deps, []ArtifactPinInput{{Path: "docs/missing.md", Kind: "doc"}})
	if len(missing) != 1 || missing[0] != "PIN_PATH_NOT_FOUND:docs/missing.md" {
		t.Fatalf("missing path warning = %v", missing)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	got := pinPathWarnings(deps, []ArtifactPinInput{{Path: outside, Kind: "doc"}})
	if len(got) != 1 || !strings.HasPrefix(got[0], "PIN_PATH_OUTSIDE_REPO:absolute:secret.txt") {
		t.Fatalf("outside absolute path warning = %v", got)
	}
	if strings.Contains(got[0], filepath.Dir(outside)) {
		t.Fatalf("outside absolute path leaked directory: %v", got)
	}
}

func TestPinPathWarningsCollapseAllMissingAsUnobservable(t *testing.T) {
	root := t.TempDir()
	got := pinPathWarnings(Deps{RepoRoot: root}, []ArtifactPinInput{
		{Path: "external/a.go", Kind: "code"},
		{Path: "external/b.go", Kind: "code"},
	})
	if len(got) != 1 || got[0] != "PIN_PATH_UNOBSERVABLE:2" {
		t.Fatalf("all-missing external workspace warning = %v, want PIN_PATH_UNOBSERVABLE:2", got)
	}
}

func TestPinDiagnosticHintsPointAtWorkspaceDetect(t *testing.T) {
	warnings := []string{"PIN_REPO_NOT_REGISTERED:docs/a.md"}
	actions := strings.Join(pinDiagnosticSuggestedActions(warnings), "\n")
	if !strings.Contains(actions, "pindoc.workspace.detect") || !strings.Contains(actions, "project_repos") {
		t.Fatalf("pin diagnostic actions are not actionable: %q", actions)
	}
	tools := pinDiagnosticNextTools(warnings)
	if len(tools) != 1 || tools[0].Tool != "pindoc.workspace.detect" {
		t.Fatalf("pin diagnostic next_tools = %+v", tools)
	}
}
