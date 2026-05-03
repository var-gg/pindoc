package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/config"
)

func TestLegacyReaderLocaleRedirect(t *testing.T) {
	handler := New(&config.Config{}, Deps{})

	req := httptest.NewRequest(http.MethodGet, "/p/pindoc/ko/wiki/canonical-only-on-demand-translation?from=legacy", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d; want 301", rec.Code)
	}
	want := "/p/pindoc/wiki/canonical-only-on-demand-translation?from=legacy"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q; want %q", got, want)
	}
}

// TestConfigReportsProvidersAndBind locks the wire format the Reader
// reads from /api/config. Decision `decision-auth-model-loopback-and-
// providers` retired the auth_mode enum in favour of `providers` +
// `bind_addr`; FE keys "is the operator the calling principal" off
// the loopback judgement of the current request, not off this
// instance-wide config.
func TestConfigReportsProvidersAndBind(t *testing.T) {
	cfg := &config.Config{
		AuthProviders: []string{config.AuthProviderGitHub},
		BindAddr:      "0.0.0.0:5830",
	}
	handler := New(cfg, Deps{})

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, want := body["bind_addr"], "0.0.0.0:5830"; got != want {
		t.Fatalf("bind_addr = %v, want %q", got, want)
	}
	rawProviders, ok := body["providers"].([]any)
	if !ok {
		t.Fatalf("providers = %v (%T), want []any", body["providers"], body["providers"])
	}
	wantProviders := []any{config.AuthProviderGitHub}
	if !reflect.DeepEqual(rawProviders, wantProviders) {
		t.Fatalf("providers = %#v, want %#v", rawProviders, wantProviders)
	}
	if _, ok := body["auth_mode"]; ok {
		t.Fatalf("auth_mode should be retired from /api/config")
	}
	if got, want := body["multi_project_switching"], false; got != want {
		t.Fatalf("multi_project_switching = %v, want %v", got, want)
	}
	if got, want := body["project_create_allowed"], true; got != want {
		t.Fatalf("project_create_allowed = %v, want %v", got, want)
	}
	if got, want := body["multi_project"], body["multi_project_switching"]; got != want {
		t.Fatalf("multi_project legacy alias = %v, want %v", got, want)
	}
	if got, want := body["multi_project_deprecated"], "use multi_project_switching"; got != want {
		t.Fatalf("multi_project_deprecated = %v, want %q", got, want)
	}
}

func TestOAuthUnavailableRoutesDoNotFallThroughToSPA(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<!doctype html><title>Pindoc</title>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	handler := New(&config.Config{}, Deps{SPADistDir: dist})

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/.well-known/oauth-protected-resource"},
		{http.MethodGet, "/.well-known/oauth-authorization-server"},
		{http.MethodGet, "/.well-known/jwks.json"},
		{http.MethodGet, "/oauth/authorize"},
		{http.MethodGet, "/oauth/token"},
		{http.MethodPost, "/oauth/token"},
		{http.MethodPost, "/oauth/revoke"},
		{http.MethodGet, "/auth/github/login"},
		{http.MethodGet, "/auth/github/callback"},
		{http.MethodPost, "/auth/logout"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d; want 503; body=%s", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") || strings.Contains(ct, "text/html") {
				t.Fatalf("Content-Type = %q, want application/json and not text/html", ct)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode JSON: %v; body=%s", err, rec.Body.String())
			}
			if body["error"] != "auth_not_configured" || body["hint"] != "set PINDOC_AUTH_PROVIDERS" {
				t.Fatalf("body = %#v", body)
			}
		})
	}
}

func TestSecurityHeadersBaseline(t *testing.T) {
	handler := New(&config.Config{}, Deps{})

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	headers := rec.Header()
	if got := headers.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := headers.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
	if got := headers.Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("Referrer-Policy = %q", got)
	}
}

func TestSPAFallbackHasBaselineCSP(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<!doctype html><title>Pindoc</title>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	handler := New(&config.Config{}, Deps{SPADistDir: dist})

	req := httptest.NewRequest(http.MethodGet, "/p/pindoc/wiki", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"script-src 'self'", "style-src 'self' 'unsafe-inline'", "img-src 'self' data: blob:", "connect-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("Content-Security-Policy = %q, missing %q", csp, want)
		}
	}
}

func TestCORSAllowedOriginsMatrix(t *testing.T) {
	cases := []struct {
		name        string
		cfg         *config.Config
		origin      string
		wantOrigin  string
		wantVary    bool
		wantMethods bool
	}{
		{
			name:        "default denies cross origin",
			cfg:         &config.Config{},
			origin:      "https://app.example.test",
			wantOrigin:  "",
			wantMethods: false,
		},
		{
			name:        "configured origin echoes",
			cfg:         &config.Config{AllowedOrigins: []string{"https://app.example.test"}},
			origin:      "https://app.example.test",
			wantOrigin:  "https://app.example.test",
			wantVary:    true,
			wantMethods: true,
		},
		{
			name:        "unlisted origin denied",
			cfg:         &config.Config{AllowedOrigins: []string{"https://app.example.test"}},
			origin:      "https://evil.example.test",
			wantOrigin:  "",
			wantMethods: false,
		},
		{
			name:        "dev mode wildcard",
			cfg:         &config.Config{DevMode: true},
			origin:      "https://app.example.test",
			wantOrigin:  "*",
			wantMethods: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := New(tc.cfg, Deps{})
			req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
			req.Header.Set("Origin", tc.origin)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; want 200", rec.Code)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.wantOrigin {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, tc.wantOrigin)
			}
			if got := rec.Header().Get("Access-Control-Allow-Methods"); (got != "") != tc.wantMethods {
				t.Fatalf("Access-Control-Allow-Methods = %q, presence want %v", got, tc.wantMethods)
			}
			if got := rec.Header().Values("Vary"); tc.wantVary && !containsHeaderValue(got, "Origin") {
				t.Fatalf("Vary = %#v, want Origin", got)
			}
		})
	}
}

func TestTelemetryRequiresInstanceOwner(t *testing.T) {
	handler := New(&config.Config{}, Deps{})

	req := httptest.NewRequest(http.MethodGet, "/api/ops/telemetry", nil)
	req.RemoteAddr = "10.0.0.5:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "INSTANCE_OWNER_REQUIRED") {
		t.Fatalf("body missing INSTANCE_OWNER_REQUIRED: %s", rec.Body.String())
	}
}

func containsHeaderValue(values []string, want string) bool {
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if strings.TrimSpace(part) == want {
				return true
			}
		}
	}
	return false
}

// TestConfigDefaultBindReportsLoopback verifies the default boot path
// surfaces the loopback bind addr so the Reader can show "running on
// localhost" cues without the operator setting any env.
func TestConfigDefaultBindReportsLoopback(t *testing.T) {
	handler := New(&config.Config{}, Deps{})

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, want := body["bind_addr"], config.DefaultBindAddr; got != want {
		t.Fatalf("bind_addr = %v, want %q", got, want)
	}
	rawProviders, ok := body["providers"].([]any)
	if !ok {
		t.Fatalf("providers should always serialise as an array; got %T", body["providers"])
	}
	if len(rawProviders) != 0 {
		t.Fatalf("providers = %#v, want empty", rawProviders)
	}
}
