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

func TestOrgProjectArtifactRoutesIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run org-scoped HTTP DB integration")
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
	orgSlug := "curioustore-" + suffix
	projectSlug := "pindoc-" + suffix
	ownerEmail := "org-route-owner-" + suffix + "@example.invalid"
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Org Route Owner "+suffix, ownerEmail)
	orgID := insertOrgRouteHTTPOrg(t, ctx, pool, orgSlug)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin create project tx: %v", err)
	}
	out, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            projectSlug,
		Name:            "Org Route " + suffix,
		PrimaryLanguage: "en",
		OrganizationID:  orgID,
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
	if _, err := pool.Exec(ctx, `UPDATE projects SET visibility = $1 WHERE id = $2::uuid`, projects.VisibilityPublic, projectID); err != nil {
		t.Fatalf("mark project public: %v", err)
	}
	areaID := selectArtifactVisibilityHTTPArea(t, ctx, pool, projectID, "misc")
	publicSlug := "public-" + suffix
	orgArtifactSlug := "org-" + suffix
	privateSlug := "private-" + suffix
	insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, publicSlug, projects.VisibilityPublic)
	insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, orgArtifactSlug, projects.VisibilityOrg)
	insertArtifactVisibilityHTTPArtifact(t, ctx, pool, projectID, areaID, privateSlug, projects.VisibilityPrivate)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, projectSlug)
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE slug = $1`, orgSlug)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lower(email) = $1`, strings.ToLower(ownerEmail))
	})

	handler := New(&config.Config{BindAddr: "0.0.0.0:5830"}, Deps{
		DB:                 pool,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultProjectSlug: projectSlug,
		BindAddr:           "0.0.0.0:5830",
	})

	current := doInviteRequest(t, handler, nil, "", http.MethodGet, "/api/orgs/"+orgSlug+"/p/"+projectSlug, "")
	if current.Code != http.StatusOK {
		t.Fatalf("org project current status = %d, want 200; body=%s", current.Code, current.Body.String())
	}
	var projectBody projectInfo
	if err := json.NewDecoder(current.Body).Decode(&projectBody); err != nil {
		t.Fatalf("decode project current: %v", err)
	}
	if projectBody.Slug != projectSlug || projectBody.OrgSlug != orgSlug || projectBody.OrganizationSlug != orgSlug {
		t.Fatalf("project current = %+v, want project=%q org=%q", projectBody, projectSlug, orgSlug)
	}

	list := doInviteRequest(t, handler, nil, "", http.MethodGet, "/api/orgs/"+orgSlug+"/p/"+projectSlug+"/artifacts", "")
	if list.Code != http.StatusOK {
		t.Fatalf("org artifact list status = %d, want 200; body=%s", list.Code, list.Body.String())
	}
	var listBody struct {
		ProjectSlug      string `json:"project_slug"`
		OrgSlug          string `json:"org_slug"`
		OrganizationSlug string `json:"organization_slug"`
		Artifacts        []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode artifact list: %v", err)
	}
	if listBody.ProjectSlug != projectSlug || listBody.OrgSlug != orgSlug || listBody.OrganizationSlug != orgSlug {
		t.Fatalf("list scope = %+v, want project=%q org=%q", listBody, projectSlug, orgSlug)
	}
	if len(listBody.Artifacts) != 1 || listBody.Artifacts[0].Slug != publicSlug || listBody.Artifacts[0].Visibility != projects.VisibilityPublic {
		t.Fatalf("anonymous org artifacts = %+v, want only public artifact %q", listBody.Artifacts, publicSlug)
	}

	publicDetail := doInviteRequest(t, handler, nil, "", http.MethodGet, "/api/orgs/"+orgSlug+"/p/"+projectSlug+"/artifacts/"+publicSlug, "")
	if publicDetail.Code != http.StatusOK {
		t.Fatalf("public detail status = %d, want 200; body=%s", publicDetail.Code, publicDetail.Body.String())
	}
	orgDetail := doInviteRequest(t, handler, nil, "", http.MethodGet, "/api/orgs/"+orgSlug+"/p/"+projectSlug+"/artifacts/"+orgArtifactSlug, "")
	if orgDetail.Code != http.StatusNotFound {
		t.Fatalf("org-only detail status = %d, want 404; body=%s", orgDetail.Code, orgDetail.Body.String())
	}

	unknownOrg := doInviteRequest(t, handler, nil, "", http.MethodGet, "/api/orgs/unknown-org/p/"+projectSlug+"/artifacts", "")
	if unknownOrg.Code != http.StatusNotFound {
		t.Fatalf("unknown org status = %d, want 404; body=%s", unknownOrg.Code, unknownOrg.Body.String())
	}
	unknownProject := doInviteRequest(t, handler, nil, "", http.MethodGet, "/api/orgs/"+orgSlug+"/p/unknown-prj/artifacts", "")
	if unknownProject.Code != http.StatusNotFound {
		t.Fatalf("unknown project status = %d, want 404; body=%s", unknownProject.Code, unknownProject.Body.String())
	}

	legacy := doInviteRequest(t, handler, nil, "", http.MethodGet, "/api/p/"+projectSlug+"/artifacts", "")
	if legacy.Code != http.StatusOK {
		t.Fatalf("legacy artifact list status = %d, want 200; body=%s", legacy.Code, legacy.Body.String())
	}
	var legacyBody struct {
		Artifacts []struct {
			Slug string `json:"slug"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(legacy.Body).Decode(&legacyBody); err != nil {
		t.Fatalf("decode legacy artifact list: %v", err)
	}
	if len(legacyBody.Artifacts) != 1 || legacyBody.Artifacts[0].Slug != publicSlug {
		t.Fatalf("legacy anonymous artifacts = %+v, want only public artifact %q", legacyBody.Artifacts, publicSlug)
	}
}

func TestOrgProjectInboxRoutesShareLegacyAccessMatrixIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run org-scoped inbox HTTP DB integration")
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
	orgSlug := "inbox-org-" + suffix
	projectSlug := "inbox-route-" + suffix
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Inbox Route Owner "+suffix, "inbox-route-owner-"+suffix+"@example.invalid")
	outsiderID := insertInviteHTTPUser(t, ctx, pool, "Inbox Route Outsider "+suffix, "inbox-route-outsider-"+suffix+"@example.invalid")
	orgID := insertOrgRouteHTTPOrg(t, ctx, pool, orgSlug)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin create project tx: %v", err)
	}
	out, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            projectSlug,
		Name:            "Inbox Route " + suffix,
		PrimaryLanguage: "en",
		OrganizationID:  orgID,
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
	if _, err := pool.Exec(ctx, `UPDATE projects SET visibility = $1 WHERE id = $2::uuid`, projects.VisibilityPublic, projectID); err != nil {
		t.Fatalf("mark project public: %v", err)
	}
	areaID := selectArtifactVisibilityHTTPArea(t, ctx, pool, projectID, "misc")
	legacyReviewSlug := "legacy-review-" + suffix
	orgReviewSlug := "org-review-" + suffix
	insertInboxPendingArtifact(t, ctx, pool, projectID, areaID, legacyReviewSlug, projects.VisibilityPublic, ownerID)
	insertInboxPendingArtifact(t, ctx, pool, projectID, areaID, orgReviewSlug, projects.VisibilityPublic, ownerID)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, projectSlug)
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE slug = $1`, orgSlug)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM users
			 WHERE lower(email) IN ($1, $2)
		`, "inbox-route-owner-"+suffix+"@example.invalid", "inbox-route-outsider-"+suffix+"@example.invalid")
	})

	oauthSvc, err := pauth.NewOAuthService(ctx, pool, pauth.OAuthConfig{
		Issuer:             "http://127.0.0.1:5830",
		PublicBaseURL:      "http://127.0.0.1:5830",
		RedirectBaseURL:    "http://127.0.0.1:5830",
		SigningKeyPath:     t.TempDir() + "/oauth.pem",
		ClientID:           "org-inbox-route-" + suffix,
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
		DefaultProjectSlug: projectSlug,
		OAuth:              oauthSvc,
		AuthProviders:      []string{config.AuthProviderGitHub},
		BindAddr:           "0.0.0.0:5830",
	})

	cases := []struct {
		name   string
		userID string
		method string
		path   string
		body   string
		want   int
	}{
		{"legacy anonymous get", "", http.MethodGet, "/api/p/" + projectSlug + "/inbox", "", http.StatusNotFound},
		{"org anonymous get", "", http.MethodGet, "/api/orgs/" + orgSlug + "/p/" + projectSlug + "/inbox", "", http.StatusNotFound},
		{"legacy outsider get", outsiderID, http.MethodGet, "/api/p/" + projectSlug + "/inbox", "", http.StatusNotFound},
		{"org outsider get", outsiderID, http.MethodGet, "/api/orgs/" + orgSlug + "/p/" + projectSlug + "/inbox", "", http.StatusNotFound},
		{"legacy owner get", ownerID, http.MethodGet, "/api/p/" + projectSlug + "/inbox", "", http.StatusOK},
		{"org owner get", ownerID, http.MethodGet, "/api/orgs/" + orgSlug + "/p/" + projectSlug + "/inbox", "", http.StatusOK},
		{"legacy owner post", ownerID, http.MethodPost, "/api/p/" + projectSlug + "/inbox/" + legacyReviewSlug + "/review", `{"decision":"approve","commit_msg":"ok"}`, http.StatusOK},
		{"org owner post", ownerID, http.MethodPost, "/api/orgs/" + orgSlug + "/p/" + projectSlug + "/inbox/" + orgReviewSlug + "/review", `{"decision":"approve","commit_msg":"ok"}`, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := doInviteRequest(t, handler, oauthSvc, c.userID, c.method, c.path, c.body)
			if rec.Code != c.want {
				t.Fatalf("%s %s status = %d, want %d; body=%s", c.method, c.path, rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

func insertOrgRouteHTTPOrg(t *testing.T, ctx context.Context, pool *db.Pool, slug string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO organizations (slug, name, kind)
		VALUES ($1, $2, 'team')
		RETURNING id::text
	`, slug, "Org Route "+slug).Scan(&id); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	return id
}
