package mcp

import (
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/config"
)

// TestAuthChainForConfigLoopbackOnly locks the default boot path: no
// IdP configured → chain has only TrustedLocalResolver, which always
// matches (loopback fastpath). Decision `decision-auth-model-loopback-
// and-providers` § 2.
func TestAuthChainForConfigLoopbackOnly(t *testing.T) {
	chain := authChainForConfig(&config.Config{}, "user-id", "agent-id")
	if chain == nil {
		t.Fatalf("authChainForConfig() chain = nil")
	}
	if chain.Len() != 1 {
		t.Fatalf("Len = %d; want 1 (TrustedLocal only)", chain.Len())
	}
}

// TestAuthChainForConfigWithGitHubProvider verifies adding `github`
// to AuthProviders prepends a BearerTokenResolver so JWT-bearing
// requests are recognised before the loopback fallback.
func TestAuthChainForConfigWithGitHubProvider(t *testing.T) {
	chain := authChainForConfig(
		&config.Config{AuthProviders: []string{config.AuthProviderGitHub}},
		"user-id", "agent-id",
	)
	if chain == nil {
		t.Fatalf("authChainForConfig() chain = nil")
	}
	if chain.Len() != 2 {
		t.Fatalf("Len = %d; want 2 (Bearer + TrustedLocal)", chain.Len())
	}
}

// TestAuthChainForConfigNilSafe guards against panics when the boot
// wiring hasn't filled the Config pointer yet (test fixtures).
func TestAuthChainForConfigNilSafe(t *testing.T) {
	chain := authChainForConfig(nil, "user-id", "agent-id")
	if chain == nil {
		t.Fatalf("authChainForConfig(nil) chain = nil")
	}
	if chain.Len() != 1 {
		t.Fatalf("Len = %d; want 1 (TrustedLocal fallback even with nil cfg)", chain.Len())
	}
}
