package config

import (
	"reflect"
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

func TestLoadOAuthDefaultsAndRedirectList(t *testing.T) {
	t.Setenv("PINDOC_OAUTH_REDIRECT_URIS", " http://127.0.0.1:1111/cb, http://localhost:2222/cb ;http://127.0.0.1:1111/cb ")
	t.Setenv("PINDOC_OAUTH_REDIRECT_BASE_URL", " http://127.0.0.1:5830 ")
	t.Setenv("PINDOC_OAUTH_CLIENT_ID", " test-client ")
	t.Setenv("PINDOC_OAUTH_CLIENT_SECRET", " secret ")
	t.Setenv("PINDOC_OAUTH_SIGNING_KEY_PATH", "C:/tmp/pindoc-oauth.pem")
	t.Setenv("PINDOC_GITHUB_CLIENT_ID", " github-client ")
	t.Setenv("PINDOC_GITHUB_CLIENT_SECRET", " github-secret ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OAuthClientID != "test-client" {
		t.Fatalf("OAuthClientID = %q, want test-client", cfg.OAuthClientID)
	}
	if cfg.OAuthClientSecret != "secret" {
		t.Fatalf("OAuthClientSecret = %q, want secret", cfg.OAuthClientSecret)
	}
	if cfg.OAuthSigningKeyPath != "C:/tmp/pindoc-oauth.pem" {
		t.Fatalf("OAuthSigningKeyPath = %q", cfg.OAuthSigningKeyPath)
	}
	if cfg.OAuthRedirectBaseURL != "http://127.0.0.1:5830" {
		t.Fatalf("OAuthRedirectBaseURL = %q", cfg.OAuthRedirectBaseURL)
	}
	if cfg.GitHubClientID != "github-client" {
		t.Fatalf("GitHubClientID = %q", cfg.GitHubClientID)
	}
	if cfg.GitHubClientSecret != "github-secret" {
		t.Fatalf("GitHubClientSecret = %q", cfg.GitHubClientSecret)
	}
	want := []string{"http://127.0.0.1:1111/cb", "http://localhost:2222/cb"}
	if !reflect.DeepEqual(cfg.OAuthRedirectURIs, want) {
		t.Fatalf("OAuthRedirectURIs = %#v, want %#v", cfg.OAuthRedirectURIs, want)
	}
}
