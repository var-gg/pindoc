package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/config"
)

// TestValidateServerConfig_LoopbackHappyPath locks the default boot
// path. Loopback bind, no IdP, no opt-in — Public-Without-Auth
// Refusal does not fire and the validator passes.
func TestValidateServerConfig_LoopbackHappyPath(t *testing.T) {
	if err := validateServerConfig(&config.Config{BindAddr: config.DefaultBindAddr}); err != nil {
		t.Fatalf("loopback default config error = %v", err)
	}
}

// TestValidateServerConfig_RejectsPublicWithoutAuth surfaces the boot-
// time refusal Decision § 4 enforces. The validator wraps
// config.Validate() so a misconfigured operator never silently boots
// in the wrong security model.
func TestValidateServerConfig_RejectsPublicWithoutAuth(t *testing.T) {
	err := validateServerConfig(&config.Config{BindAddr: "0.0.0.0:5830"})
	if !errors.Is(err, config.ErrPublicWithoutAuth) {
		t.Fatalf("validateServerConfig err = %v, want ErrPublicWithoutAuth", err)
	}
}

// TestValidateServerConfig_RequiresGitHubOAuthClient locks the
// follow-on invariant: opting into the github provider without
// supplying GitHub OAuth App credentials is also a boot-time error.
// The validator names both env vars so the operator knows which to
// set.
func TestValidateServerConfig_RequiresGitHubOAuthClient(t *testing.T) {
	err := validateServerConfig(&config.Config{
		BindAddr:      "0.0.0.0:5830",
		AuthProviders: []string{config.AuthProviderGitHub},
	})
	if err == nil {
		t.Fatalf("validateServerConfig(github without credentials) error = nil")
	}
	for _, want := range []string{"PINDOC_GITHUB_CLIENT_ID", "PINDOC_GITHUB_CLIENT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}

	err = validateServerConfig(&config.Config{
		BindAddr:            "0.0.0.0:5830",
		AuthProviders:       []string{config.AuthProviderGitHub},
		GitHubClientID:      "github-client",
		GitHubClientSecret:  "github-secret",
		OAuthSigningKeyPath: "unused",
	})
	if err != nil {
		t.Fatalf("validateServerConfig(github with credentials) error = %v", err)
	}
}
