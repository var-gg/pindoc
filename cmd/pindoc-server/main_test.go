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
