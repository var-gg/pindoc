package main

import (
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/config"
)

func TestValidateServerAuthMode(t *testing.T) {
	if err := validateServerAuthMode(config.AuthModeTrustedLocal); err != nil {
		t.Fatalf("trusted_local error = %v", err)
	}
	if err := validateServerAuthMode(config.AuthModeOAuthGitHub); err != nil {
		t.Fatalf("oauth_github error = %v", err)
	}

	for _, mode := range []config.AuthMode{
		config.AuthModePublicReadonly,
		config.AuthModeSingleUser,
	} {
		t.Run(string(mode), func(t *testing.T) {
			err := validateServerAuthMode(mode)
			if err == nil {
				t.Fatalf("validateServerAuthMode(%q) error = nil", mode)
			}
			if !strings.Contains(err.Error(), "PINDOC_AUTH_MODE="+string(mode)) {
				t.Fatalf("error %q does not name mode %q", err.Error(), mode)
			}
		})
	}
}

func TestValidateServerConfigRequiresGitHubOAuthClient(t *testing.T) {
	err := validateServerConfig(&config.Config{AuthMode: config.AuthModeOAuthGitHub})
	if err == nil {
		t.Fatalf("validateServerConfig(oauth_github without GitHub client) error = nil")
	}
	for _, want := range []string{"PINDOC_GITHUB_CLIENT_ID", "PINDOC_GITHUB_CLIENT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}

	err = validateServerConfig(&config.Config{
		AuthMode:            config.AuthModeOAuthGitHub,
		GitHubClientID:      "github-client",
		GitHubClientSecret:  "github-secret",
		OAuthSigningKeyPath: "unused",
	})
	if err != nil {
		t.Fatalf("validateServerConfig(oauth_github with GitHub client) error = %v", err)
	}
}
