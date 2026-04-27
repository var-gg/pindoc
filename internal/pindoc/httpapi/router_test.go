package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestConfigReportsAuthMode(t *testing.T) {
	handler := New(&config.Config{AuthMode: config.AuthModeSingleUser}, Deps{})

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
	if got := body["auth_mode"]; got != string(config.AuthModeSingleUser) {
		t.Fatalf("auth_mode = %v, want %q", got, config.AuthModeSingleUser)
	}
}
