package tools

import (
	"strings"
	"testing"
)

func TestNormalizeArtifactReadRef(t *testing.T) {
	cases := []struct {
		name          string
		raw           string
		want          string
		wantShare     bool
		wantMismatch  bool
		projectSlug   string
		projectLocale string
	}{
		{
			name:          "bare slug",
			raw:           "sidecar-taskcontrols-meta-patch-shape",
			want:          "sidecar-taskcontrols-meta-patch-shape",
			projectSlug:   "pindoc",
			projectLocale: "ko",
		},
		{
			name:          "bare UUID",
			raw:           "0b8562d8-4f2d-46e2-9f9a-8b0a6e1592a1",
			want:          "0b8562d8-4f2d-46e2-9f9a-8b0a6e1592a1",
			projectSlug:   "pindoc",
			projectLocale: "ko",
		},
		{
			name:          "pindoc URL",
			raw:           "pindoc://sidecar-taskcontrols-meta-patch-shape",
			want:          "sidecar-taskcontrols-meta-patch-shape",
			projectSlug:   "pindoc",
			projectLocale: "ko",
		},
		{
			name:          "relative reader share path",
			raw:           "/p/pindoc/ko/wiki/sidecar-taskcontrols-meta-patch-shape",
			want:          "sidecar-taskcontrols-meta-patch-shape",
			wantShare:     true,
			projectSlug:   "pindoc",
			projectLocale: "ko",
		},
		{
			name:          "absolute reader share URL",
			raw:           "http://localhost:5830/p/pindoc/ko/wiki/sidecar-taskcontrols-meta-patch-shape",
			want:          "sidecar-taskcontrols-meta-patch-shape",
			wantShare:     true,
			projectSlug:   "pindoc",
			projectLocale: "ko",
		},
		{
			name:          "reader history URL",
			raw:           "/p/pindoc/ko/wiki/sidecar-taskcontrols-meta-patch-shape/history",
			want:          "sidecar-taskcontrols-meta-patch-shape",
			wantShare:     true,
			projectSlug:   "pindoc",
			projectLocale: "ko",
		},
		{
			name:          "legacy /a URL",
			raw:           "https://example.test/a/0b8562d8-4f2d-46e2-9f9a-8b0a6e1592a1",
			want:          "0b8562d8-4f2d-46e2-9f9a-8b0a6e1592a1",
			wantShare:     true,
			projectSlug:   "pindoc",
			projectLocale: "ko",
		},
		{
			name:          "other project scope mismatch",
			raw:           "/p/other/ko/wiki/sidecar-taskcontrols-meta-patch-shape",
			want:          "sidecar-taskcontrols-meta-patch-shape",
			wantShare:     true,
			wantMismatch:  true,
			projectSlug:   "pindoc",
			projectLocale: "ko",
		},
		{
			name:          "other locale scope mismatch",
			raw:           "/p/pindoc/en/wiki/sidecar-taskcontrols-meta-patch-shape",
			want:          "sidecar-taskcontrols-meta-patch-shape",
			wantShare:     true,
			wantMismatch:  true,
			projectSlug:   "pindoc",
			projectLocale: "ko",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeArtifactReadRef(tc.raw, tc.projectSlug, tc.projectLocale)
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
		"/p/pindoc/ko/wiki/missing-artifact",
		"pindoc",
		"ko",
	)

	err := artifactReadNotFoundError(
		"/p/pindoc/ko/wiki/missing-artifact",
		Deps{ProjectSlug: "pindoc", ProjectLocale: "ko"},
		ref,
	)
	msg := err.Error()
	for _, want := range []string{"share URL", "extracted slug", "missing-artifact"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}
