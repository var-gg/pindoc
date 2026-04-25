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
