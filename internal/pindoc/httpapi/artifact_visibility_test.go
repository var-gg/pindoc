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
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestDecodeArtifactPatch(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantTier  string
		wantError string
	}{
		{name: "public", body: `{"visibility":"public"}`, wantTier: projects.VisibilityPublic},
		{name: "trim and lower", body: `{"visibility":" PRIVATE "}`, wantTier: projects.VisibilityPrivate},
		{name: "empty", body: `{}`, wantError: "ARTIFACT_PATCH_EMPTY"},
		{name: "unsupported", body: `{"visibility":"org","title":"x"}`, wantError: "ARTIFACT_PATCH_FIELD_UNSUPPORTED"},
		{name: "invalid visibility", body: `{"visibility":"deleted"}`, wantError: "VISIBILITY_INVALID"},
		{name: "non string visibility", body: `{"visibility":42}`, wantError: "VISIBILITY_INVALID"},
		{name: "bad json", body: `{`, wantError: "BAD_JSON"},
		{name: "trailing json", body: `{"visibility":"org"} {}`, wantError: "BAD_JSON"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := decodeArtifactPatch(strings.NewReader(c.body))
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
			if got.Visibility == nil || *got.Visibility != c.wantTier {
				t.Fatalf("visibility = %v, want %q", got.Visibility, c.wantTier)
			}
		})
	}
}

func TestArtifactVisibilityPatchIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run artifact visibility HTTP DB integration")
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
	slug := "vis-http-" + suffix
	ownerEmail := "vis-owner-" + suffix + "@example.invalid"
	viewerEmail := "vis-viewer-" + suffix + "@example.invalid"
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Visibility Owner "+suffix, ownerEmail)
	viewerID := insertInviteHTTPUser(t, ctx, pool, "Visibility Viewer "+suffix, viewerEmail)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin create project tx: %v", err)
	}
	out, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            slug,
		Name:            "Visibility HTTP " + suffix,
		PrimaryLanguage: "en",
		OwnerID:         "vis-http-" + suffix,
		OwnerUserID:     ownerID,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create project: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit create project tx: %v", err)
	}
	projectID := out.ID
	insertInviteHTTPMember(t, ctx, pool, projectID, viewerID, pauth.RoleViewer)
	areaID := selectArtifactVisibilityHTTPArea(t, ctx, pool, projectID, "misc")
	artifactSlug := "vis-art-" + suffix
	artifactID := insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, artifactSlug, projects.VisibilityOrg)

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
		ClientID:           "visibility-http-" + suffix,
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

	ownerGet := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodGet, "/api/p/"+slug+"/artifacts/"+artifactSlug, "")
	if ownerGet.Code != http.StatusOK {
		t.Fatalf("owner get status = %d, want 200; body=%s", ownerGet.Code, ownerGet.Body.String())
	}
	var ownerDetail struct {
		Visibility        string `json:"visibility"`
		CanEditVisibility bool   `json:"can_edit_visibility"`
	}
	if err := json.NewDecoder(ownerGet.Body).Decode(&ownerDetail); err != nil {
		t.Fatalf("decode owner detail: %v", err)
	}
	if ownerDetail.Visibility != projects.VisibilityOrg || !ownerDetail.CanEditVisibility {
		t.Fatalf("owner detail = %+v, want visibility=org and can_edit_visibility=true", ownerDetail)
	}

	viewerGet := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodGet, "/api/p/"+slug+"/artifacts/"+artifactSlug, "")
	if viewerGet.Code != http.StatusOK {
		t.Fatalf("viewer get status = %d, want 200; body=%s", viewerGet.Code, viewerGet.Body.String())
	}
	var viewerDetail struct {
		CanEditVisibility bool `json:"can_edit_visibility"`
	}
	if err := json.NewDecoder(viewerGet.Body).Decode(&viewerDetail); err != nil {
		t.Fatalf("decode viewer detail: %v", err)
	}
	if viewerDetail.CanEditVisibility {
		t.Fatalf("viewer can_edit_visibility = true, want false")
	}

	ownerPatch := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPatch, "/api/p/"+slug+"/artifacts/"+artifactSlug, `{"visibility":"private"}`)
	if ownerPatch.Code != http.StatusOK {
		t.Fatalf("owner patch status = %d, want 200; body=%s", ownerPatch.Code, ownerPatch.Body.String())
	}
	var patchOut artifactPatchResp
	if err := json.NewDecoder(ownerPatch.Body).Decode(&patchOut); err != nil {
		t.Fatalf("decode owner patch: %v", err)
	}
	if patchOut.Status != "ok" || patchOut.ArtifactID != artifactID || patchOut.Visibility != projects.VisibilityPrivate {
		t.Fatalf("owner patch resp = %+v", patchOut)
	}
	assertArtifactVisibilityHTTP(t, ctx, pool, artifactID, projects.VisibilityPrivate)

	viewerPatch := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodPatch, "/api/p/"+slug+"/artifacts/"+artifactSlug, `{"visibility":"public"}`)
	if viewerPatch.Code != http.StatusForbidden {
		t.Fatalf("viewer patch status = %d, want 403; body=%s", viewerPatch.Code, viewerPatch.Body.String())
	}
	assertArtifactPatchErrorCode(t, viewerPatch.Body.String(), "PROJECT_OWNER_REQUIRED")
	assertArtifactVisibilityHTTP(t, ctx, pool, artifactID, projects.VisibilityPrivate)

	anonymousPatch := doInviteRequest(t, handler, nil, "", http.MethodPatch, "/api/p/"+slug+"/artifacts/"+artifactSlug, `{"visibility":"public"}`)
	if anonymousPatch.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous patch status = %d, want 401; body=%s", anonymousPatch.Code, anonymousPatch.Body.String())
	}
	assertArtifactPatchErrorCode(t, anonymousPatch.Body.String(), "AUTH_REQUIRED")

	invalidPatch := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPatch, "/api/p/"+slug+"/artifacts/"+artifactSlug, `{"visibility":"deleted"}`)
	if invalidPatch.Code != http.StatusBadRequest {
		t.Fatalf("invalid patch status = %d, want 400; body=%s", invalidPatch.Code, invalidPatch.Body.String())
	}
	assertArtifactPatchErrorCode(t, invalidPatch.Body.String(), "VISIBILITY_INVALID")

	missingPatch := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPatch, "/api/p/"+slug+"/artifacts/missing-artifact", `{"visibility":"public"}`)
	if missingPatch.Code != http.StatusNotFound {
		t.Fatalf("missing patch status = %d, want 404; body=%s", missingPatch.Code, missingPatch.Body.String())
	}
	assertArtifactPatchErrorCode(t, missingPatch.Body.String(), "ARTIFACT_NOT_FOUND")
}

func selectArtifactVisibilityHTTPArea(t *testing.T, ctx context.Context, pool *db.Pool, projectID, slug string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		SELECT id::text
		  FROM areas
		 WHERE project_id = $1::uuid AND slug = $2
	`, projectID, slug).Scan(&id); err != nil {
		t.Fatalf("select area %s: %v", slug, err)
	}
	return id
}

func insertArtifactVisibilityHTTPArtifact(t *testing.T, ctx context.Context, pool *db.Pool, projectID, areaID, slug, visibility string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (
			project_id, area_id, slug, type, title,
			body_markdown, body_locale, author_kind, author_id,
			completeness, status, review_state, visibility, published_at
		) VALUES (
			$1::uuid, $2::uuid, $3, 'Decision', $3,
			'body', 'en', 'agent', 'codex',
			'partial', 'published', 'auto_published', $4, now()
		)
		RETURNING id::text
	`, projectID, areaID, slug, visibility).Scan(&id); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash, tags,
			completeness, author_kind, author_id, author_version, commit_msg,
			revision_shape
		) VALUES (
			$1::uuid, 1, $2, 'body', encode(sha256(convert_to('body', 'UTF8')), 'hex'), ARRAY[]::text[],
			'partial', 'agent', 'codex', 'test', 'seed visibility test artifact',
			'body_patch'
		)
	`, id, slug); err != nil {
		t.Fatalf("insert artifact revision: %v", err)
	}
	return id
}

func assertArtifactVisibilityHTTP(t *testing.T, ctx context.Context, pool *db.Pool, artifactID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `
		SELECT visibility
		  FROM artifacts
		 WHERE id = $1::uuid
	`, artifactID).Scan(&got); err != nil {
		t.Fatalf("select artifact visibility: %v", err)
	}
	if got != want {
		t.Fatalf("visibility = %q, want %q", got, want)
	}
}

func assertArtifactPatchErrorCode(t *testing.T, body, want string) {
	t.Helper()
	var got artifactPatchError
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode error body %q: %v", body, err)
	}
	if got.ErrorCode != want {
		t.Fatalf("error_code = %q, want %q; body=%s", got.ErrorCode, want, body)
	}
}
