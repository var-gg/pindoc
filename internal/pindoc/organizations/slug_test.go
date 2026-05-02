package organizations

import (
	"errors"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	cases := []struct {
		name    string
		slug    string
		wantErr error // nil for accept; sentinel for reject
	}{
		{name: "accepts simple kebab", slug: "curioustore", wantErr: nil},
		{name: "accepts hyphenated", slug: "var-gg-team", wantErr: nil},
		{name: "accepts digits after letter", slug: "team-2026", wantErr: nil},
		{name: "accepts 2-char minimum", slug: "ab", wantErr: nil},
		{name: "rejects empty", slug: "", wantErr: ErrSlugInvalid},
		{name: "rejects single char", slug: "a", wantErr: ErrSlugInvalid},
		{name: "rejects leading digit", slug: "2026-team", wantErr: ErrSlugInvalid},
		{name: "rejects leading hyphen", slug: "-team", wantErr: ErrSlugInvalid},
		{name: "rejects underscore", slug: "my_team", wantErr: ErrSlugInvalid},
		{name: "accepts uppercase via normalization", slug: "MyTeam", wantErr: nil},
		{name: "rejects whitespace inside", slug: "my team", wantErr: ErrSlugInvalid},
		{name: "rejects 41+ chars", slug: "a23456789012345678901234567890123456789012", wantErr: ErrSlugInvalid},
		// Reserved set — system paths and routing collisions
		{name: "rejects login", slug: "login", wantErr: ErrSlugReserved},
		{name: "rejects api", slug: "api", wantErr: ErrSlugReserved},
		{name: "rejects settings", slug: "settings", wantErr: ErrSlugReserved},
		{name: "rejects pricing", slug: "pricing", wantErr: ErrSlugReserved},
		{name: "rejects p (project path prefix)", slug: "p", wantErr: ErrSlugInvalid},
		// "p" is len=1 so regex fails before reserved check; documented here
		// because it's the most important reserved value and we want to
		// confirm the dual-layer rejection still blocks it.
		{name: "rejects default sentinel", slug: "default", wantErr: ErrSlugReserved},
		{name: "rejects user", slug: "user", wantErr: ErrSlugReserved},
		{name: "rejects org", slug: "org", wantErr: ErrSlugReserved},
		// Case-insensitive: uppercase Login should fail regex first, but
		// the trimmed/lowered slug "login" should still hit reserved.
		{name: "trims whitespace then checks reserved", slug: "  login  ", wantErr: ErrSlugReserved},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSlug(tc.slug)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected accept, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected reject (%v), got accept", tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected err to wrap %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestIsReservedSlug(t *testing.T) {
	for _, slug := range []string{"login", "api", "settings", "default", "p", "showcase"} {
		if !IsReservedSlug(slug) {
			t.Errorf("expected %q to be reserved", slug)
		}
	}
	// Whitespace + case folding should still hit the reserved set.
	if !IsReservedSlug("  Login  ") {
		t.Errorf("expected '  Login  ' (case-folded) to be reserved")
	}
	for _, slug := range []string{"curioustore", "vargg", "pindoc", "var-gg-team"} {
		if IsReservedSlug(slug) {
			t.Errorf("expected %q to be allowed", slug)
		}
	}
}

func TestNormalizeSlug(t *testing.T) {
	cases := map[string]string{
		"":               "",
		"  curioustore ": "curioustore",
		"VAR-GG":         "var-gg",
		"Mixed-Case-42":  "mixed-case-42",
	}
	for in, want := range cases {
		if got := NormalizeSlug(in); got != want {
			t.Errorf("NormalizeSlug(%q) = %q, want %q", in, got, want)
		}
	}
}
