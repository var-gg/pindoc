package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

	cappedPublicPatch := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPatch, "/api/p/"+slug+"/artifacts/"+artifactSlug, `{"visibility":"public"}`)
	if cappedPublicPatch.Code != http.StatusBadRequest {
		t.Fatalf("capped public patch status = %d, want 400; body=%s", cappedPublicPatch.Code, cappedPublicPatch.Body.String())
	}
	assertArtifactPatchErrorCode(t, cappedPublicPatch.Body.String(), "VISIBILITY_CAPPED_BY_PROJECT")
	assertArtifactVisibilityHTTP(t, ctx, pool, artifactID, projects.VisibilityOrg)

	if _, err := pool.Exec(ctx, `UPDATE projects SET visibility = $1 WHERE id = $2::uuid`, projects.VisibilityPublic, projectID); err != nil {
		t.Fatalf("set project public: %v", err)
	}
	ownerPublicPatch := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPatch, "/api/p/"+slug+"/artifacts/"+artifactSlug, `{"visibility":"public"}`)
	if ownerPublicPatch.Code != http.StatusOK {
		t.Fatalf("owner public patch status = %d, want 200; body=%s", ownerPublicPatch.Code, ownerPublicPatch.Body.String())
	}
	var publicPatchOut artifactPatchResp
	if err := json.NewDecoder(ownerPublicPatch.Body).Decode(&publicPatchOut); err != nil {
		t.Fatalf("decode owner public patch: %v", err)
	}
	if publicPatchOut.Status != "ok" || publicPatchOut.Visibility != projects.VisibilityPublic || publicPatchOut.Affected != 1 {
		t.Fatalf("owner public patch resp = %+v", publicPatchOut)
	}
	assertArtifactVisibilityHTTP(t, ctx, pool, artifactID, projects.VisibilityPublic)

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
	if patchOut.Code != "VISIBILITY_UPDATED" || patchOut.Affected != 1 || patchOut.RevisionNumber == 0 {
		t.Fatalf("owner patch audit fields = %+v", patchOut)
	}
	assertArtifactVisibilityHTTP(t, ctx, pool, artifactID, projects.VisibilityPrivate)
	assertArtifactRevisionNumberHTTP(t, ctx, pool, artifactID, patchOut.RevisionNumber)
	assertArtifactVisibilityEventHTTP(t, ctx, pool, artifactID, "public", "private")

	updatedAtAfterChange := selectArtifactUpdatedAtHTTP(t, ctx, pool, artifactID)
	noOpPatch := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPatch, "/api/p/"+slug+"/artifacts/"+artifactSlug, `{"visibility":"private"}`)
	if noOpPatch.Code != http.StatusOK {
		t.Fatalf("no-op patch status = %d, want 200; body=%s", noOpPatch.Code, noOpPatch.Body.String())
	}
	var noOpOut artifactPatchResp
	if err := json.NewDecoder(noOpPatch.Body).Decode(&noOpOut); err != nil {
		t.Fatalf("decode no-op patch: %v", err)
	}
	if noOpOut.Status != "informational" || noOpOut.Code != "VISIBILITY_NO_OP" || noOpOut.Affected != 0 {
		t.Fatalf("no-op patch resp = %+v", noOpOut)
	}
	assertArtifactRevisionNumberHTTP(t, ctx, pool, artifactID, patchOut.RevisionNumber)
	if got := selectArtifactUpdatedAtHTTP(t, ctx, pool, artifactID); !got.Equal(updatedAtAfterChange) {
		t.Fatalf("no-op updated_at changed: before=%s after=%s", updatedAtAfterChange, got)
	}

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

func TestLegacyArtifactRoutesApplyVisibilityIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run legacy artifact visibility HTTP DB integration")
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
	slug := "legacy-vis-" + suffix
	ownerEmail := "legacy-vis-owner-" + suffix + "@example.invalid"
	viewerEmail := "legacy-vis-viewer-" + suffix + "@example.invalid"
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Legacy Visibility Owner "+suffix, ownerEmail)
	viewerID := insertInviteHTTPUser(t, ctx, pool, "Legacy Visibility Viewer "+suffix, viewerEmail)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin create project tx: %v", err)
	}
	out, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            slug,
		Name:            "Legacy Visibility " + suffix,
		PrimaryLanguage: "en",
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
	publicSlug := "legacy-public-" + suffix
	orgSlug := "legacy-org-" + suffix
	privateSlug := "legacy-private-" + suffix
	privateMemberSlug := "legacy-private-member-" + suffix
	insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, publicSlug, projects.VisibilityPublic, ownerID)
	insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, orgSlug, projects.VisibilityOrg, ownerID)
	insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, privateSlug, projects.VisibilityPrivate, ownerID)
	insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, privateMemberSlug, projects.VisibilityPrivate, viewerID)

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
		ClientID:           "legacy-visibility-http-" + suffix,
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
		DefaultUserID:      ownerID,
		OAuth:              oauthSvc,
		AuthProviders:      []string{config.AuthProviderGitHub},
		BindAddr:           "0.0.0.0:5830",
	})

	viewerOrg := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodGet, "/api/p/"+slug+"/artifacts/"+orgSlug, "")
	if viewerOrg.Code != http.StatusOK {
		t.Fatalf("viewer org detail status = %d, want 200; body=%s", viewerOrg.Code, viewerOrg.Body.String())
	}
	viewerList := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodGet, "/api/p/"+slug+"/artifacts", "")
	if viewerList.Code != http.StatusOK {
		t.Fatalf("viewer list status = %d, want 200; body=%s", viewerList.Code, viewerList.Body.String())
	}
	var viewerListBody struct {
		Artifacts []struct {
			Slug string `json:"slug"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(viewerList.Body).Decode(&viewerListBody); err != nil {
		t.Fatalf("decode viewer list: %v", err)
	}
	viewerSlugs := map[string]bool{}
	for _, artifact := range viewerListBody.Artifacts {
		viewerSlugs[artifact.Slug] = true
	}
	if !viewerSlugs[publicSlug] || !viewerSlugs[orgSlug] || !viewerSlugs[privateMemberSlug] || viewerSlugs[privateSlug] {
		t.Fatalf("viewer artifacts = %+v, want public+org+self-private and no other private", viewerListBody.Artifacts)
	}

	ownerMemberPrivate := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodGet, "/api/p/"+slug+"/artifacts/"+privateMemberSlug, "")
	if ownerMemberPrivate.Code != http.StatusOK {
		t.Fatalf("owner member-authored private detail status = %d, want 200; body=%s", ownerMemberPrivate.Code, ownerMemberPrivate.Body.String())
	}

	viewerPrivate := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodGet, "/api/p/"+slug+"/artifacts/"+privateSlug, "")
	if viewerPrivate.Code != http.StatusNotFound {
		t.Fatalf("viewer private detail status = %d, want 404; body=%s", viewerPrivate.Code, viewerPrivate.Body.String())
	}
	if strings.Contains(viewerPrivate.Body.String(), "body_markdown") {
		t.Fatalf("viewer private detail leaked body_markdown: %s", viewerPrivate.Body.String())
	}

	viewerRevisions := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodGet, "/api/p/"+slug+"/artifacts/"+privateSlug+"/revisions", "")
	if viewerRevisions.Code != http.StatusNotFound {
		t.Fatalf("viewer private revisions status = %d, want 404; body=%s", viewerRevisions.Code, viewerRevisions.Body.String())
	}
	viewerDiff := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodGet, "/api/p/"+slug+"/artifacts/"+privateSlug+"/diff?from=1&to=1", "")
	if viewerDiff.Code != http.StatusNotFound {
		t.Fatalf("viewer private diff status = %d, want 404; body=%s", viewerDiff.Code, viewerDiff.Body.String())
	}
	if strings.Contains(viewerDiff.Body.String(), "unified_diff") {
		t.Fatalf("viewer private diff leaked unified_diff: %s", viewerDiff.Body.String())
	}

	anonymousList := doInviteRequest(t, handler, nil, "", http.MethodGet, "/api/p/"+slug+"/artifacts", "")
	if anonymousList.Code != http.StatusOK {
		t.Fatalf("anonymous list status = %d, want 200; body=%s", anonymousList.Code, anonymousList.Body.String())
	}
	var listBody struct {
		Artifacts []struct {
			Slug string `json:"slug"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(anonymousList.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode anonymous list: %v", err)
	}
	if len(listBody.Artifacts) != 1 || listBody.Artifacts[0].Slug != publicSlug {
		t.Fatalf("anonymous artifacts = %+v, want only public artifact %q", listBody.Artifacts, publicSlug)
	}

	loopbackList := doLoopbackVisibilityRequest(handler, http.MethodGet, "/api/p/"+slug+"/artifacts", "")
	if loopbackList.Code != http.StatusOK {
		t.Fatalf("loopback list status = %d, want 200; body=%s", loopbackList.Code, loopbackList.Body.String())
	}
	var loopbackBody struct {
		Artifacts []struct {
			Slug string `json:"slug"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(loopbackList.Body).Decode(&loopbackBody); err != nil {
		t.Fatalf("decode loopback list: %v", err)
	}
	if len(loopbackBody.Artifacts) != 4 {
		t.Fatalf("loopback artifacts count = %d, want 4; artifacts=%+v", len(loopbackBody.Artifacts), loopbackBody.Artifacts)
	}
}

func doLoopbackVisibilityRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:5830"
	req.Host = "127.0.0.1:5830"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
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

func insertArtifactVisibilityHTTPArtifact(t *testing.T, ctx context.Context, pool *db.Pool, projectID, areaID, slug, visibility string, authorUserID ...string) string {
	t.Helper()
	authorID := ""
	if len(authorUserID) > 0 {
		authorID = strings.TrimSpace(authorUserID[0])
	}
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (
			project_id, area_id, slug, type, title,
			body_markdown, body_locale, author_kind, author_id, author_user_id,
			completeness, status, review_state, visibility, published_at
		) VALUES (
			$1::uuid, $2::uuid, $3, 'Decision', $3,
			'body', 'en', 'agent', 'codex', NULLIF($5, '')::uuid,
			'partial', 'published', 'auto_published', $4, now()
		)
		RETURNING id::text
	`, projectID, areaID, slug, visibility, authorID).Scan(&id); err != nil {
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

func assertArtifactRevisionNumberHTTP(t *testing.T, ctx context.Context, pool *db.Pool, artifactID string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(max(revision_number), 0)
		  FROM artifact_revisions
		 WHERE artifact_id = $1::uuid
	`, artifactID).Scan(&got); err != nil {
		t.Fatalf("select artifact revision: %v", err)
	}
	if got != want {
		t.Fatalf("revision_number = %d, want %d", got, want)
	}
}

func assertArtifactVisibilityEventHTTP(t *testing.T, ctx context.Context, pool *db.Pool, artifactID, from, to string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM events
		 WHERE subject_id = $1::uuid
		   AND kind = 'artifact.visibility_changed'
		   AND payload->>'from' = $2
		   AND payload->>'to' = $3
	`, artifactID, from, to).Scan(&count); err != nil {
		t.Fatalf("select visibility event: %v", err)
	}
	if count != 1 {
		t.Fatalf("visibility event count = %d, want 1", count)
	}
}

func selectArtifactUpdatedAtHTTP(t *testing.T, ctx context.Context, pool *db.Pool, artifactID string) time.Time {
	t.Helper()
	var got time.Time
	if err := pool.QueryRow(ctx, `
		SELECT updated_at
		  FROM artifacts
		 WHERE id = $1::uuid
	`, artifactID).Scan(&got); err != nil {
		t.Fatalf("select artifact updated_at: %v", err)
	}
	return got
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
