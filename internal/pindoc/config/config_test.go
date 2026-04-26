package config

import (
	"strings"
	"testing"
)

func TestAuthModeValid(t *testing.T) {
	valid := []AuthMode{
		AuthModeTrustedLocal,
		AuthModePublicReadonly,
		AuthModeSingleUser,
		AuthModeOAuthGitHub,
	}
	for _, mode := range valid {
		if !mode.Valid() {
			t.Fatalf("%q should be valid", mode)
		}
	}
	if AuthMode("project_token").Valid() {
		t.Fatalf("project_token should not be valid until it is added to the enum")
	}
}

func TestLoadAuthMode(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want AuthMode
	}{
		{"default", "", AuthModeTrustedLocal},
		{"trusted_local", "trusted_local", AuthModeTrustedLocal},
		{"public_readonly", "public_readonly", AuthModePublicReadonly},
		{"single_user", "single_user", AuthModeSingleUser},
		{"oauth_github", "oauth_github", AuthModeOAuthGitHub},
		{"trim and lowercase", " OAUTH_GITHUB ", AuthModeOAuthGitHub},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("PINDOC_AUTH_MODE", c.env)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.AuthMode != c.want {
				t.Fatalf("AuthMode = %q, want %q", cfg.AuthMode, c.want)
			}
		})
	}
}

func TestLoadInvalidAuthMode(t *testing.T) {
	t.Setenv("PINDOC_AUTH_MODE", "foobar")
	cfg, err := Load()
	if err == nil {
		t.Fatalf("Load() error = nil, cfg = %#v", cfg)
	}
	msg := err.Error()
	for _, want := range []string{
		"invalid PINDOC_AUTH_MODE: 'foobar'",
		"trusted_local|public_readonly|single_user|oauth_github",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
}
