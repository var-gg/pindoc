package git

import "testing"

func TestCleanRepoPathRejectsTraversal(t *testing.T) {
	for _, path := range []string{"../secret", "/abs/path", "docs/../../secret"} {
		if _, err := cleanRepoPath(path); err != ErrPathRejected {
			t.Fatalf("cleanRepoPath(%q) err=%v, want ErrPathRejected", path, err)
		}
	}
}

func TestCleanRepoPathNormalizes(t *testing.T) {
	got, err := cleanRepoPath("docs\\guide.md")
	if err != nil {
		t.Fatalf("cleanRepoPath returned error: %v", err)
	}
	if got != "docs/guide.md" {
		t.Fatalf("cleanRepoPath = %q", got)
	}
}
