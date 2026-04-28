package httpapi

import (
	"testing"

	pgit "github.com/var-gg/pindoc/internal/pindoc/git"
)

func TestGitHubFallbackURL(t *testing.T) {
	repo := pgit.Repo{
		GitRemoteOriginal: "git@github.com:var-gg/pindoc.git",
		DefaultBranch:     "main",
	}
	got := githubFallbackURL(repo, "abcdef1234567890", "internal/pindoc/git/provider.go")
	want := "https://github.com/var-gg/pindoc/blob/abcdef1234567890/internal/pindoc/git/provider.go"
	if got != want {
		t.Fatalf("fallback url = %q, want %q", got, want)
	}
}

func TestGitPreviewReason(t *testing.T) {
	if got := gitPreviewReason(pgit.ErrNoProviderForRepo); got != "no_provider_for_repo" {
		t.Fatalf("reason = %q", got)
	}
	if got := gitPreviewReason(pgit.ErrPathRejected); got != "path_rejected" {
		t.Fatalf("reason = %q", got)
	}
}
