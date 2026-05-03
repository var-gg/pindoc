package config

import (
	"errors"
	"reflect"
	"testing"
)

// TestLoadDefaults locks the historical "single-user self-host" path:
// no env set means loopback bind, no IdP, public-without-auth disabled.
// Decision `decision-auth-model-loopback-and-providers` Case A1.
func TestLoadDefaults(t *testing.T) {
	t.Setenv("PINDOC_BIND_ADDR", "")
	t.Setenv("PINDOC_AUTH_PROVIDERS", "")
	t.Setenv("PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BindAddr != DefaultBindAddr {
		t.Fatalf("BindAddr = %q, want %q", cfg.BindAddr, DefaultBindAddr)
	}
	if len(cfg.AuthProviders) != 0 {
		t.Fatalf("AuthProviders = %v, want empty", cfg.AuthProviders)
	}
	if cfg.AllowPublicUnauthenticated {
		t.Fatalf("AllowPublicUnauthenticated should default false")
	}
	if !cfg.IsLoopbackBind() {
		t.Fatalf("default bind should be loopback")
	}
	if cfg.AssetRoot != "/var/lib/pindoc/assets" {
		t.Fatalf("AssetRoot = %q, want default LocalFS root", cfg.AssetRoot)
	}
}

// TestLoadProvidersCSVNormalizes verifies CSV parsing — trim, lower,
// dedupe — so `PINDOC_AUTH_PROVIDERS=GitHub , github,Google` boots a
// stable, deterministic provider list regardless of operator
// whitespace and case habits.
func TestLoadProvidersCSVNormalizes(t *testing.T) {
	t.Setenv("PINDOC_AUTH_PROVIDERS", "GitHub , github,Google ")
	t.Setenv("PINDOC_BIND_ADDR", DefaultBindAddr)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []string{AuthProviderGitHub, "google"}
	if !reflect.DeepEqual(cfg.AuthProviders, want) {
		t.Fatalf("AuthProviders = %#v, want %#v", cfg.AuthProviders, want)
	}
	if !cfg.HasAuthProvider("github") || !cfg.HasAuthProvider("Google") {
		t.Fatalf("HasAuthProvider should be case-insensitive")
	}
	if cfg.HasAuthProvider("gitlab") {
		t.Fatalf("HasAuthProvider should reject providers not in the list")
	}
}

func TestLoadWithSampleFlag(t *testing.T) {
	t.Setenv("PINDOC_WITH_SAMPLE", "true")
	t.Setenv("PINDOC_BIND_ADDR", DefaultBindAddr)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.WithSample {
		t.Fatal("PINDOC_WITH_SAMPLE=true should enable sample fixtures")
	}
}

func TestLoadAssetRoot(t *testing.T) {
	t.Setenv("PINDOC_ASSET_ROOT", "C:/pindoc-assets")
	t.Setenv("PINDOC_BIND_ADDR", DefaultBindAddr)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AssetRoot != "C:/pindoc-assets" {
		t.Fatalf("AssetRoot = %q", cfg.AssetRoot)
	}
}

// TestConfigPublicWithoutAuthRefused locks the boot-time refusal:
// non-loopback bind + empty providers + no explicit opt-in must fail
// fast with ErrPublicWithoutAuth so a misconfigured operator never
// silently exposes the daemon to the network.
func TestConfigPublicWithoutAuthRefused(t *testing.T) {
	t.Setenv("PINDOC_BIND_ADDR", "0.0.0.0:5830")
	t.Setenv("PINDOC_AUTH_PROVIDERS", "")
	t.Setenv("PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED", "false")
	_, err := Load()
	if !errors.Is(err, ErrPublicWithoutAuth) {
		t.Fatalf("Load() err = %v, want ErrPublicWithoutAuth", err)
	}
}

// TestConfigLoopbackProvidersEmptyOK verifies the default single-user
// path boots cleanly even when the operator never sets any
// PINDOC_AUTH_* env. Loopback bind makes the request-side trust
// boundary the OS process model.
func TestConfigLoopbackProvidersEmptyOK(t *testing.T) {
	t.Setenv("PINDOC_BIND_ADDR", "127.0.0.1:5830")
	t.Setenv("PINDOC_AUTH_PROVIDERS", "")
	t.Setenv("PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.IsLoopbackBind() {
		t.Fatalf("expected loopback bind")
	}
	if len(cfg.AuthProviders) != 0 {
		t.Fatalf("AuthProviders = %v, want empty", cfg.AuthProviders)
	}
}

// TestConfigPublicWithProvidersOK verifies the cross-device collaboration
// path: external bind + IdP active. Decision Case A3 (1-person cross-
// device) and Case D (friend joins a 1-person project).
func TestConfigPublicWithProvidersOK(t *testing.T) {
	t.Setenv("PINDOC_BIND_ADDR", "0.0.0.0:5830")
	t.Setenv("PINDOC_AUTH_PROVIDERS", "github")
	t.Setenv("PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.IsLoopbackBind() {
		t.Fatalf("0.0.0.0 bind should not register as loopback")
	}
	if !cfg.HasAuthProvider(AuthProviderGitHub) {
		t.Fatalf("expected github provider active")
	}
}

// TestConfigPublicAllowUnauthOK verifies the explicit "private LAN
// behind reverse proxy" opt-in. Operator acknowledges they trust the
// network even without an IdP — boot succeeds.
func TestConfigPublicAllowUnauthOK(t *testing.T) {
	t.Setenv("PINDOC_BIND_ADDR", "0.0.0.0:5830")
	t.Setenv("PINDOC_AUTH_PROVIDERS", "")
	t.Setenv("PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if !cfg.AllowPublicUnauthenticated {
		t.Fatalf("AllowPublicUnauthenticated should be true")
	}
}

// TestConfigIPv6LoopbackBind verifies the IPv6 loopback (`::1`) is
// recognised as loopback so `[::1]:5830` boots without tripping the
// Public-Without-Auth Refusal.
func TestConfigIPv6LoopbackBind(t *testing.T) {
	t.Setenv("PINDOC_BIND_ADDR", "[::1]:5830")
	t.Setenv("PINDOC_AUTH_PROVIDERS", "")
	t.Setenv("PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.IsLoopbackBind() {
		t.Fatalf("[::1] bind should register as loopback")
	}
}

// TestLoadOAuthDefaultsAndRedirectList preserves the existing OAuth
// env handling — the redirect list parsing and credential trimming
// are independent of the provider framing pivot.
func TestLoadOAuthDefaultsAndRedirectList(t *testing.T) {
	t.Setenv("PINDOC_OAUTH_REDIRECT_URIS", " http://127.0.0.1:1111/cb, http://localhost:2222/cb ;http://127.0.0.1:1111/cb ")
	t.Setenv("PINDOC_OAUTH_REDIRECT_BASE_URL", " http://127.0.0.1:5830 ")
	t.Setenv("PINDOC_OAUTH_CLIENT_ID", " test-client ")
	t.Setenv("PINDOC_OAUTH_CLIENT_SECRET", " secret ")
	t.Setenv("PINDOC_OAUTH_SIGNING_KEY_PATH", "C:/tmp/pindoc-oauth.pem")
	t.Setenv("PINDOC_GITHUB_CLIENT_ID", " github-client ")
	t.Setenv("PINDOC_GITHUB_CLIENT_SECRET", " github-secret ")
	t.Setenv("PINDOC_AUTH_PROVIDERS", "github")
	t.Setenv("PINDOC_BIND_ADDR", DefaultBindAddr)

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

func TestLoadForceOAuthLocal(t *testing.T) {
	t.Setenv("PINDOC_FORCE_OAUTH_LOCAL", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.ForceOAuthLocal {
		t.Fatal("ForceOAuthLocal = false, want true")
	}
}
