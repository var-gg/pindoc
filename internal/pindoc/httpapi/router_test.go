package httpapi

import (
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
