package main

import (
	"errors"
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

// TestValidateServerConfig_GitHubCredentialsAreOptional locks the
// post-providers-admin-ui contract: validateServerConfig no longer
// requires env GitHub credentials when AuthProviders includes github,
// because the admin UI can supply them via instance_providers at
// runtime. cmd/pindoc-server's main() still fails loud at OAuth init
// time when neither env nor DB carries credentials — that path needs
// the open DB pool, which the validator does not have.
func TestValidateServerConfig_GitHubCredentialsAreOptional(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
	}{
		{
			name: "github provider without env credentials",
			cfg: config.Config{
				BindAddr:      "0.0.0.0:5830",
				AuthProviders: []string{config.AuthProviderGitHub},
			},
		},
		{
			name: "github provider with env credentials",
			cfg: config.Config{
				BindAddr:           "0.0.0.0:5830",
				AuthProviders:      []string{config.AuthProviderGitHub},
				GitHubClientID:     "github-client",
				GitHubClientSecret: "github-secret",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := validateServerConfig(&c.cfg); err != nil {
				t.Fatalf("validateServerConfig: %v", err)
			}
		})
	}
}
