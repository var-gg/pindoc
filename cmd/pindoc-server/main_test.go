package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
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

func TestValidateOAuthBootPosture_ForceOAuthRequiresProvider(t *testing.T) {
	err := validateOAuthBootPosture(&config.Config{ForceOAuthLocal: true}, false)
	if !errors.Is(err, errForceOAuthLocalRequiresProvider) {
		t.Fatalf("validateOAuthBootPosture err = %v, want errForceOAuthLocalRequiresProvider", err)
	}
	if err := validateOAuthBootPosture(&config.Config{ForceOAuthLocal: true}, true); err != nil {
		t.Fatalf("active provider should satisfy force-oauth prerequisite: %v", err)
	}
	if err := validateOAuthBootPosture(&config.Config{}, false); err != nil {
		t.Fatalf("force off should not require provider: %v", err)
	}
}

func TestShouldBypassMCPBearerForceOAuthLocal(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5830/mcp", nil)
	req.RemoteAddr = "127.0.0.1:51234"

	if !shouldBypassMCPBearer(&config.Config{}, req, false) {
		t.Fatal("default loopback request should bypass bearer middleware")
	}
	if shouldBypassMCPBearer(&config.Config{ForceOAuthLocal: true}, req, false) {
		t.Fatal("ForceOAuthLocal should route loopback request through bearer middleware")
	}
}

func TestShouldBypassMCPBearerTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5830/mcp", nil)
	req.RemoteAddr = "172.18.0.1:51234"

	if !shouldBypassMCPBearer(&config.Config{}, req, true) {
		t.Fatal("trusted same-host proxy should bypass bearer middleware")
	}
	if shouldBypassMCPBearer(&config.Config{ForceOAuthLocal: true}, req, true) {
		t.Fatal("ForceOAuthLocal should keep trusted-proxy request on bearer middleware")
	}
	if shouldBypassMCPBearer(&config.Config{}, req, false) {
		t.Fatal("non-loopback request without trusted proxy should require bearer")
	}
}

func TestWrapMCPBearerForLoopbackForceOAuthLocal(t *testing.T) {
	streamHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("tool-ok"))
	})
	verifier := func(_ context.Context, token string, _ *http.Request) (*mcpauth.TokenInfo, error) {
		if token != "valid" {
			return nil, mcpauth.ErrInvalidToken
		}
		return &mcpauth.TokenInfo{
			Scopes:     []string{pauth.ScopePindoc},
			Expiration: time.Now().Add(time.Hour),
		}, nil
	}
	bearer := mcpauth.RequireBearerToken(verifier, &mcpauth.RequireBearerTokenOptions{
		ResourceMetadataURL: "http://127.0.0.1:5830/.well-known/oauth-protected-resource",
		Scopes:              []string{pauth.ScopePindoc},
	})(streamHandler)

	t.Run("force on requires bearer for loopback", func(t *testing.T) {
		handler := wrapMCPBearerForLoopback(streamHandler, bearer, &config.Config{ForceOAuthLocal: true}, false)
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5830/mcp", nil)
		req.RemoteAddr = "127.0.0.1:51234"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		header := rec.Header().Get("WWW-Authenticate")
		if !strings.Contains(header, "Bearer ") || !strings.Contains(header, "resource_metadata=") || !strings.Contains(header, pauth.ScopePindoc) {
			t.Fatalf("WWW-Authenticate = %q, want bearer challenge with PRM and scope", header)
		}
	})

	t.Run("force on accepts valid bearer for loopback", func(t *testing.T) {
		handler := wrapMCPBearerForLoopback(streamHandler, bearer, &config.Config{ForceOAuthLocal: true}, false)
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5830/mcp", nil)
		req.RemoteAddr = "127.0.0.1:51234"
		req.Header.Set("Authorization", "Bearer valid")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
		}
		if got := rec.Body.String(); got != "tool-ok" {
			t.Fatalf("body = %q, want tool-ok", got)
		}
	})

	t.Run("default preserves loopback bypass", func(t *testing.T) {
		handler := wrapMCPBearerForLoopback(streamHandler, bearer, &config.Config{}, false)
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5830/mcp", nil)
		req.RemoteAddr = "127.0.0.1:51234"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
		}
		if got := rec.Body.String(); got != "tool-ok" {
			t.Fatalf("body = %q, want tool-ok", got)
		}
	})

	t.Run("trusted proxy bypasses non-loopback bridge address", func(t *testing.T) {
		handler := wrapMCPBearerForLoopback(streamHandler, bearer, &config.Config{}, true)
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5830/mcp", nil)
		req.RemoteAddr = "172.18.0.1:51234"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
		}
		if got := rec.Body.String(); got != "tool-ok" {
			t.Fatalf("body = %q, want tool-ok", got)
		}
	})
}
