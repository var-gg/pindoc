package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/providers"
)

// TestInstanceProvidersHandlers covers the loopback-owner happy path
// (list / upsert / delete) plus the four refusal codes the admin UI
// keys off (PROVIDERS_UNAVAILABLE / INSTANCE_OWNER_REQUIRED /
// PROVIDER_UNSUPPORTED / CLIENT_ID_REQUIRED). Skipped when there is no
// PINDOC_TEST_DATABASE_URL because the store needs a real Postgres
// for crypto + uuid round-trips.
func TestInstanceProvidersHandlers(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run instance providers HTTP integration")
	}
	ctx := t.Context()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	cipher, err := providers.NewCipherFromBase64(base64.StdEncoding.EncodeToString(handlerFreshKey(t)))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	store := providers.New(pool, cipher)
	t.Cleanup(func() {
		_, _ = pool.Exec(t.Context(), `DELETE FROM instance_providers`)
	})

	handler := New(&config.Config{}, Deps{
		DB:        pool,
		Providers: store,
	})

	// 1. Loopback list returns empty.
	rec := doProvidersRequest(handler, http.MethodGet, "/api/instance/providers", "", "127.0.0.1:54321")
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback list status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// 2. External list refused with INSTANCE_OWNER_REQUIRED.
	rec = doProvidersRequest(handler, http.MethodGet, "/api/instance/providers", "", "10.0.0.5:54321")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("external list status = %d, want 403", rec.Code)
	}

	// 3. Unsupported provider rejected.
	rec = doProvidersRequest(handler, http.MethodPost, "/api/instance/providers",
		`{"provider_name":"google","client_id":"x","client_secret":"y"}`, "127.0.0.1:54321")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unsupported provider status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PROVIDER_UNSUPPORTED") {
		t.Fatalf("body missing PROVIDER_UNSUPPORTED: %s", rec.Body.String())
	}

	// 4. client_id required.
	rec = doProvidersRequest(handler, http.MethodPost, "/api/instance/providers",
		`{"provider_name":"github","client_id":""}`, "127.0.0.1:54321")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing client_id status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// 5. Happy path: upsert github + assert HasClientSecret reflects encryption.
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	body := fmt.Sprintf(`{"provider_name":"github","client_id":"client-%s","client_secret":"secret-%s"}`, suffix, suffix)
	rec = doProvidersRequest(handler, http.MethodPost, "/api/instance/providers", body, "127.0.0.1:54321")
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var upsertResp providerOpResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &upsertResp); err != nil {
		t.Fatalf("decode upsert: %v", err)
	}
	if upsertResp.Provider == nil || upsertResp.Provider.ProviderName != "github" {
		t.Fatalf("upsert provider = %+v", upsertResp.Provider)
	}
	if !upsertResp.Provider.HasClientSecret {
		t.Fatalf("HasClientSecret = false; want true")
	}

	// 6. List should now contain the row, secret stripped on the wire.
	rec = doProvidersRequest(handler, http.MethodGet, "/api/instance/providers", "", "127.0.0.1:54321")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var list providerListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Providers) != 1 || list.Providers[0].ClientID != "client-"+suffix {
		t.Fatalf("list providers = %+v", list.Providers)
	}
	for _, p := range list.Providers {
		if strings.Contains(rec.Body.String(), "secret-"+suffix) {
			t.Fatalf("list response leaked client_secret: %s", rec.Body.String())
		}
		if p.HasClientSecret == false {
			t.Fatalf("HasClientSecret = false in list; want true")
		}
	}

	// 7. Delete by name.
	rec = doProvidersRequest(handler, http.MethodDelete, "/api/instance/providers/github", "", "127.0.0.1:54321")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// 8. Subsequent delete returns 404.
	rec = doProvidersRequest(handler, http.MethodDelete, "/api/instance/providers/github", "", "127.0.0.1:54321")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("idempotent delete status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

// TestInstanceProvidersHandlers_NoStore covers the boot-skipped path:
// when Deps.Providers is nil the admin endpoints respond 503 with a
// stable code so the FE can grey out the panel rather than rendering
// blank.
func TestInstanceProvidersHandlers_NoStore(t *testing.T) {
	handler := New(&config.Config{}, Deps{})
	rec := doProvidersRequest(handler, http.MethodGet, "/api/instance/providers", "", "127.0.0.1:1234")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil store status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PROVIDERS_UNAVAILABLE") {
		t.Fatalf("body missing PROVIDERS_UNAVAILABLE: %s", rec.Body.String())
	}
}

func doProvidersRequest(handler http.Handler, method, path, body, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.RemoteAddr = remoteAddr
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func handlerFreshKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return key
}
