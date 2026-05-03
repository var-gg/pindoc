package httpapi

import (
	"bytes"
	"context"
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
	"github.com/var-gg/pindoc/internal/pindoc/settings"
)

// TestOnboardingIdentityHandler_Loopback covers the agent-era first-
// time flow end-to-end: loopback caller posts a clean form, BE
// creates a users row, binds settings.default_loopback_user_id, and
// returns the three MCP-connect copy targets. Skipped without a
// PINDOC_TEST_DATABASE_URL because the whole point is to round-trip
// through real Postgres + settings reload.
func TestOnboardingIdentityHandler_Loopback(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run onboarding HTTP integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	store, err := settings.New(ctx, pool)
	if err != nil {
		t.Fatalf("settings new: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			UPDATE server_settings SET default_loopback_user_id = NULL WHERE id = 1
		`)
	})

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	projectSlug := "onb-" + suffix
	email := "onboarding-" + suffix + "@example.invalid"
	body := fmt.Sprintf(`{"display_name":"Onboard %s","email":"%s","github_handle":"@onb-%s"}`, suffix, email, suffix)

	handler := New(&config.Config{}, Deps{
		DB:                 pool,
		Settings:           store,
		DefaultProjectSlug: projectSlug,
		UserLanguage:       "ko",
	})

	rec := doOnboardingRequest(handler, http.MethodPost, body, "127.0.0.1:54321")
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback onboarding status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp onboardingIdentityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Status != "ok" || resp.UserID == "" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Project.Slug != projectSlug || resp.Project.Role != "owner" {
		t.Fatalf("project ref = %+v", resp.Project)
	}
	if !strings.Contains(resp.MCPConnect.AgentPrompt, `project_slug="`+projectSlug+`"`) {
		t.Fatalf("agent_prompt missing project slug: %q", resp.MCPConnect.AgentPrompt)
	}
	if !strings.Contains(resp.MCPConnect.MCPJSON, "/mcp") || resp.MCPConnect.URL == "" {
		t.Fatalf("mcp_connect = %+v", resp.MCPConnect)
	}
	if !strings.Contains(resp.MCPConnect.AgentPrompt, resp.MCPConnect.URL) {
		t.Fatalf("agent_prompt missing url: %q", resp.MCPConnect.AgentPrompt)
	}

	// Settings should reflect the new binding without a daemon restart.
	if uid := store.Get().DefaultLoopbackUserID; uid != resp.UserID {
		t.Fatalf("settings DefaultLoopbackUserID = %q, want %q", uid, resp.UserID)
	}
	var projectLang, memberRole string
	if err := pool.QueryRow(ctx, `
		SELECT p.primary_language, pm.role
		  FROM projects p
		  JOIN project_members pm ON pm.project_id = p.id
		 WHERE p.slug = $1 AND pm.user_id = $2::uuid
	`, projectSlug, resp.UserID).Scan(&projectLang, &memberRole); err != nil {
		t.Fatalf("default project owner lookup: %v", err)
	}
	if projectLang != "ko" || memberRole != "owner" {
		t.Fatalf("default project lang/role = %q/%q, want ko/owner", projectLang, memberRole)
	}

	// Cleanup the new user.
	_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lower(email) = $1`, strings.ToLower(email))
}

// TestOnboardingIdentityHandler_NonLoopbackForbidden locks the
// auth gate: any non-loopback caller must see INSTANCE_OWNER_REQUIRED
// regardless of payload. Decision § 2 Loopback Trust.
func TestOnboardingIdentityHandler_NonLoopbackForbidden(t *testing.T) {
	handler := New(&config.Config{}, Deps{})
	rec := doOnboardingRequest(handler, http.MethodPost,
		`{"display_name":"x","email":"y@z"}`, "10.0.0.5:54321")
	if rec.Code != http.StatusServiceUnavailable && rec.Code != http.StatusForbidden {
		t.Fatalf("non-loopback onboarding status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

// TestOnboardingIdentityHandler_RequiresFields covers the form
// validation surface so the FE can render specific error_codes for
// each missing field.
func TestOnboardingIdentityHandler_RequiresFields(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run validation surface tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("db migrate: %v", err)
	}
	store, err := settings.New(ctx, pool)
	if err != nil {
		t.Fatalf("settings new: %v", err)
	}
	handler := New(&config.Config{}, Deps{
		DB:       pool,
		Settings: store,
	})

	cases := []struct {
		name     string
		body     string
		wantCode string
	}{
		{name: "missing display_name", body: `{"display_name":"","email":"x@y"}`, wantCode: "DISPLAY_NAME_REQUIRED"},
		{name: "missing email", body: `{"display_name":"x","email":""}`, wantCode: "EMAIL_REQUIRED"},
		{name: "bad json", body: `{`, wantCode: "BAD_JSON"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := doOnboardingRequest(handler, http.MethodPost, c.body, "127.0.0.1:54321")
			if !strings.Contains(rec.Body.String(), c.wantCode) {
				t.Fatalf("body missing %q: %s", c.wantCode, rec.Body.String())
			}
		})
	}
}

func doOnboardingRequest(handler http.Handler, method, body, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/onboarding/identity", bytes.NewBufferString(body))
	req.RemoteAddr = remoteAddr
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
