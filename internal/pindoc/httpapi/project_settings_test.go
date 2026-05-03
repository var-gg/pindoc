package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestDecodeProjectSettingsPatch(t *testing.T) {
	cases := []struct {
		name             string
		body             string
		wantSensitiveOps string // empty means unset
		wantDefaultVis   string // empty means unset
		wantError        string
	}{
		{name: "confirm", body: `{"sensitive_ops":"confirm"}`, wantSensitiveOps: "confirm"},
		{name: "trim and lower", body: `{"sensitive_ops":" AUTO "}`, wantSensitiveOps: "auto"},
		{name: "default visibility public", body: `{"default_artifact_visibility":"public"}`, wantDefaultVis: "public"},
		{name: "default visibility lower", body: `{"default_artifact_visibility":" PRIVATE "}`, wantDefaultVis: "private"},
		{name: "both fields", body: `{"sensitive_ops":"confirm","default_artifact_visibility":"public"}`, wantSensitiveOps: "confirm", wantDefaultVis: "public"},
		{name: "empty", body: `{}`, wantError: "PROJECT_SETTINGS_EMPTY"},
		{name: "unsupported field", body: `{"sensitive_ops":"auto","name":"x"}`, wantError: "PROJECT_SETTINGS_FIELD_UNSUPPORTED"},
		{name: "invalid sensitive ops", body: `{"sensitive_ops":"manual"}`, wantError: "SENSITIVE_OPS_INVALID"},
		{name: "non string sensitive ops", body: `{"sensitive_ops":true}`, wantError: "SENSITIVE_OPS_INVALID"},
		{name: "invalid visibility", body: `{"default_artifact_visibility":"deleted"}`, wantError: "DEFAULT_VISIBILITY_INVALID"},
		{name: "non string visibility", body: `{"default_artifact_visibility":42}`, wantError: "DEFAULT_VISIBILITY_INVALID"},
		{name: "bad json", body: `{`, wantError: "BAD_JSON"},
		{name: "trailing json", body: `{"sensitive_ops":"auto"} {}`, wantError: "BAD_JSON"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := decodeProjectSettingsPatch(strings.NewReader(c.body))
			if c.wantError != "" {
				if err == nil {
					t.Fatalf("decode error = nil, want %s", c.wantError)
				}
				if err.ErrorCode != c.wantError {
					t.Fatalf("error_code = %q, want %q", err.ErrorCode, c.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("decode error = %+v", err)
			}
			gotSensitive := ""
			if got.SensitiveOps != nil {
				gotSensitive = *got.SensitiveOps
			}
			if gotSensitive != c.wantSensitiveOps {
				t.Errorf("sensitive_ops = %q, want %q", gotSensitive, c.wantSensitiveOps)
			}
			gotVis := ""
			if got.DefaultArtifactVisibility != nil {
				gotVis = *got.DefaultArtifactVisibility
			}
			if gotVis != c.wantDefaultVis {
				t.Errorf("default_artifact_visibility = %q, want %q", gotVis, c.wantDefaultVis)
			}
		})
	}
}

func TestProjectSettingsPatchIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run project settings HTTP DB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	slug := "settings-http-" + suffix
	ownerEmail := "settings-owner-" + suffix + "@example.invalid"
	viewerEmail := "settings-viewer-" + suffix + "@example.invalid"
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Settings Owner "+suffix, ownerEmail)
	viewerID := insertInviteHTTPUser(t, ctx, pool, "Settings Viewer "+suffix, viewerEmail)
	projectID := insertInviteHTTPProject(t, ctx, pool, slug, ownerID)
	insertInviteHTTPMember(t, ctx, pool, projectID, viewerID, "viewer")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM users
			 WHERE lower(email) IN ($1, $2)
		`, strings.ToLower(ownerEmail), strings.ToLower(viewerEmail))
	})

	oauthSvc, err := pauth.NewOAuthService(ctx, pool, pauth.OAuthConfig{
		Issuer:             "http://127.0.0.1:5830",
		PublicBaseURL:      "http://127.0.0.1:5830",
		RedirectBaseURL:    "http://127.0.0.1:5830",
		SigningKeyPath:     t.TempDir() + "/oauth.pem",
		ClientID:           "settings-http-" + suffix,
		RedirectURIs:       []string{"http://127.0.0.1:3846/callback"},
		GitHubClientID:     "fake-gh-client",
		GitHubClientSecret: "fake-gh-secret",
	})
	if err != nil {
		t.Fatalf("NewOAuthService: %v", err)
	}
	handler := New(&config.Config{
		AuthProviders: []string{config.AuthProviderGitHub},
		BindAddr:      "0.0.0.0:5830",
	}, Deps{
		DB:                 pool,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultProjectSlug: slug,
		OAuth:              oauthSvc,
		AuthProviders:      []string{config.AuthProviderGitHub},
		BindAddr:           "0.0.0.0:5830",
	})

	ownerConfirm := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPatch, "/api/p/"+slug+"/settings", `{"sensitive_ops":"confirm"}`)
	if ownerConfirm.Code != http.StatusOK {
		t.Fatalf("owner confirm status = %d, want 200; body=%s", ownerConfirm.Code, ownerConfirm.Body.String())
	}
	var out projectSettingsPatchResp
	if err := json.NewDecoder(ownerConfirm.Body).Decode(&out); err != nil {
		t.Fatalf("decode owner confirm: %v", err)
	}
	if out.Status != "ok" || out.SensitiveOps != "confirm" {
		t.Fatalf("owner confirm resp = %+v", out)
	}
	assertProjectSensitiveOps(t, ctx, pool, projectID, "confirm")

	ownerDefaultPrivate := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPatch, "/api/p/"+slug+"/settings", `{"default_artifact_visibility":"private"}`)
	if ownerDefaultPrivate.Code != http.StatusOK {
		t.Fatalf("owner default visibility status = %d, want 200; body=%s", ownerDefaultPrivate.Code, ownerDefaultPrivate.Body.String())
	}
	var defaultOut projectSettingsPatchResp
	if err := json.NewDecoder(ownerDefaultPrivate.Body).Decode(&defaultOut); err != nil {
		t.Fatalf("decode owner default visibility: %v", err)
	}
	if defaultOut.Status != "ok" || defaultOut.DefaultArtifactVisibility != "private" {
		t.Fatalf("owner default visibility resp = %+v", defaultOut)
	}
	assertProjectDefaultVisibility(t, ctx, pool, projectID, "private")

	ownerCurrent := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodGet, "/api/p/"+slug, "")
	if ownerCurrent.Code != http.StatusOK {
		t.Fatalf("owner project current status = %d, want 200; body=%s", ownerCurrent.Code, ownerCurrent.Body.String())
	}
	var current projectInfo
	if err := json.NewDecoder(ownerCurrent.Body).Decode(&current); err != nil {
		t.Fatalf("decode owner project current: %v", err)
	}
	if current.DefaultArtifactVisibility != "private" {
		t.Fatalf("project current default_artifact_visibility = %q, want private", current.DefaultArtifactVisibility)
	}
	// Pin the LEFT JOIN organizations + role/sensitive_ops projection so the
	// 0055 owner_id drop regression cannot reappear silently. organization_id
	// is NOT NULL with FK ON DELETE RESTRICT (migration 0049), so a non-empty
	// organization_slug stays the contract.
	if current.OrganizationSlug == "" {
		t.Fatalf("project current organization_slug = empty, want non-empty after LEFT JOIN organizations")
	}
	if current.SensitiveOps != "confirm" {
		t.Fatalf("project current sensitive_ops = %q, want confirm", current.SensitiveOps)
	}
	if current.CurrentRole != pauth.RoleOwner {
		t.Fatalf("project current current_role = %q, want owner", current.CurrentRole)
	}

	viewerAuto := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodPatch, "/api/p/"+slug+"/settings", `{"sensitive_ops":"auto"}`)
	if viewerAuto.Code != http.StatusForbidden {
		t.Fatalf("viewer patch status = %d, want 403; body=%s", viewerAuto.Code, viewerAuto.Body.String())
	}
	assertProjectSensitiveOps(t, ctx, pool, projectID, "confirm")

	unsupported := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPatch, "/api/p/"+slug+"/settings", `{"description":"x"}`)
	if unsupported.Code != http.StatusBadRequest {
		t.Fatalf("unsupported field status = %d, want 400; body=%s", unsupported.Code, unsupported.Body.String())
	}

	ownerAuto := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPatch, "/api/p/"+slug+"/settings", `{"sensitive_ops":"auto"}`)
	if ownerAuto.Code != http.StatusOK {
		t.Fatalf("owner auto status = %d, want 200; body=%s", ownerAuto.Code, ownerAuto.Body.String())
	}
	assertProjectSensitiveOps(t, ctx, pool, projectID, "auto")
}

func assertProjectSensitiveOps(t *testing.T, ctx context.Context, pool *db.Pool, projectID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `
		SELECT sensitive_ops
		  FROM projects
		 WHERE id = $1::uuid
	`, projectID).Scan(&got); err != nil {
		t.Fatalf("select sensitive_ops: %v", err)
	}
	if got != want {
		t.Fatalf("sensitive_ops = %q, want %q", got, want)
	}
}

func assertProjectDefaultVisibility(t *testing.T, ctx context.Context, pool *db.Pool, projectID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `
		SELECT default_artifact_visibility
		  FROM projects
		 WHERE id = $1::uuid
	`, projectID).Scan(&got); err != nil {
		t.Fatalf("select default_artifact_visibility: %v", err)
	}
	if got != want {
		t.Fatalf("default_artifact_visibility = %q, want %q", got, want)
	}
}
