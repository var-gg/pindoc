package projects

import (
	"errors"
	"testing"
)

func TestNormalizeGitRemoteURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/var-gg/pindoc.git":      "github.com/var-gg/pindoc",
		"git@github.com:Var-GG/Pindoc.git":          "github.com/var-gg/pindoc",
		"ssh://git@github.com/var-gg/pindoc":        "github.com/var-gg/pindoc",
		"ssh://git@github.com:22/var-gg/pindoc.git": "github.com/var-gg/pindoc",
		"github.com/var-gg/pindoc.git":              "github.com/var-gg/pindoc",
	}
	for raw, want := range cases {
		got, err := NormalizeGitRemoteURL(raw)
		if err != nil {
			t.Fatalf("NormalizeGitRemoteURL(%q) returned error: %v", raw, err)
		}
		if got != want {
			t.Fatalf("NormalizeGitRemoteURL(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizeGitRemoteURLErrors(t *testing.T) {
	for _, raw := range []string{"github.com", "not-a-remote", "https://github.com/owner"} {
		_, err := NormalizeGitRemoteURL(raw)
		if err == nil {
			t.Fatalf("NormalizeGitRemoteURL(%q) returned nil, want error", raw)
		}
		if !errors.Is(err, ErrGitRemoteURLInvalid) {
			t.Fatalf("NormalizeGitRemoteURL(%q) error = %v, want ErrGitRemoteURLInvalid", raw, err)
		}
	}
}

func TestNormalizeRepoPathSetEmptyReturnsEmptySlice(t *testing.T) {
	got := normalizeRepoPathSet(nil)
	if got == nil {
		t.Fatal("empty local_paths must encode as an empty array, not NULL")
	}
	if len(got) != 0 {
		t.Fatalf("normalizeRepoPathSet(nil) = %v, want empty", got)
	}
}
