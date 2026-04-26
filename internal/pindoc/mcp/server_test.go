package mcp

import (
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/config"
)

func TestAuthChainForModeTrustedLocal(t *testing.T) {
	chain, err := authChainForMode(config.AuthModeTrustedLocal, "user-id", "agent-id")
	if err != nil {
		t.Fatalf("authChainForMode() error = %v", err)
	}
	if chain == nil {
		t.Fatalf("authChainForMode() chain = nil")
	}
}

func TestAuthChainForModeUnsupported(t *testing.T) {
	cases := []config.AuthMode{
		config.AuthModePublicReadonly,
		config.AuthModeSingleUser,
		config.AuthModeOAuthGitHub,
	}
	for _, mode := range cases {
		t.Run(string(mode), func(t *testing.T) {
			chain, err := authChainForMode(mode, "user-id", "agent-id")
			if err == nil {
				t.Fatalf("authChainForMode() error = nil, chain = %#v", chain)
			}
			if !strings.Contains(err.Error(), "PINDOC_AUTH_MODE="+string(mode)) {
				t.Fatalf("error %q does not name mode %q", err.Error(), mode)
			}
		})
	}
}
