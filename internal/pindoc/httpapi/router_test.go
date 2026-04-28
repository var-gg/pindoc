package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
