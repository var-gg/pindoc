package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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

// TestMapProjectCreateError locks the contract between projects sentinel
// errors and the REST envelope's (status, error_code) pair. UI / CLI /
// curl callers all switch on error_code, so a typo or missing case here
// silently breaks every entrypoint at once. SLUG_TAKEN gets 409
// (resource conflict) — everything else is a 400 except the catchall
// 500 INTERNAL_ERROR for unwrapped errors.
func TestMapProjectCreateError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "slug invalid",
			err:        fmt.Errorf("%w: bad shape", projects.ErrSlugInvalid),
			wantStatus: http.StatusBadRequest,
			wantCode:   "SLUG_INVALID",
		},
		{
			name:       "slug reserved",
			err:        fmt.Errorf("%w: collides", projects.ErrSlugReserved),
			wantStatus: http.StatusBadRequest,
			wantCode:   "SLUG_RESERVED",
		},
		{
			name:       "slug already taken (409)",
			err:        fmt.Errorf("%w: dup", projects.ErrSlugTaken),
			wantStatus: http.StatusConflict,
			wantCode:   "SLUG_TAKEN",
		},
		{
			name:       "name required",
			err:        fmt.Errorf("%w: empty", projects.ErrNameRequired),
			wantStatus: http.StatusBadRequest,
			wantCode:   "NAME_REQUIRED",
		},
		{
			name:       "language required",
			err:        fmt.Errorf("%w: empty", projects.ErrLangRequired),
			wantStatus: http.StatusBadRequest,
			wantCode:   "LANG_REQUIRED",
		},
		{
			name:       "language invalid",
			err:        fmt.Errorf("%w: fr", projects.ErrLangInvalid),
			wantStatus: http.StatusBadRequest,
			wantCode:   "LANG_INVALID",
		},
		{
			name:       "unwrapped DB error → INTERNAL_ERROR 500",
			err:        errors.New("connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotStatus, gotCode := mapProjectCreateError(c.err)
			if gotStatus != c.wantStatus {
				t.Errorf("status = %d, want %d", gotStatus, c.wantStatus)
			}
			if gotCode != c.wantCode {
				t.Errorf("code = %q, want %q", gotCode, c.wantCode)
			}
		})
	}
}

func TestReaderHiddenProjectSlug(t *testing.T) {
	cases := []struct {
		slug string
		want bool
	}{
		{"oauth-it-abc123", true},
		{"invite-http-abc123", true},
		{"workspace-detect-abc123", true},
		{"vis-http-18abc7f4129be000", true},
		{"vis-mcp-1777735890813002700", true},
		{"artifact-audit-1777735957821357800", true},
		{"task-flow-a-1777735961285390100", true},
		{"task-flow-b-1777735961285390100", true},
		{"task-queue-across-a-1777735962378049400", true},
		{"task-queue-across-b-1777735962378049400", true},
		{"pindoc-18abd57be67af9f8", true},
		{"PINDOC-18ABD57BE67AF9F8", true},
		{"OAuth-IT-ABC123", true},
		{"pindoc", false},
		{"pindoc-tour", false},
		{"pindoc-18abd57be67af9f", false},
		{"pindoc-18abd57be67af9fg", false},
		{"pindoc-18abd57be67af9f8-extra", false},
		{"customer-docs", false},
	}
	for _, c := range cases {
		t.Run(c.slug, func(t *testing.T) {
			if got := readerHiddenProjectSlug(c.slug); got != c.want {
				t.Fatalf("readerHiddenProjectSlug(%q) = %v, want %v", c.slug, got, c.want)
			}
		})
	}
}

func TestIncludeReaderHiddenProjects(t *testing.T) {
	cases := []struct {
		name  string
		query string
		role  string
		want  bool
	}{
		{name: "no query", query: "", role: pauth.RoleOwner, want: false},
		{name: "owner include_hidden", query: "include_hidden=true", role: pauth.RoleOwner, want: true},
		{name: "owner include_internal", query: "include_internal=true", role: pauth.RoleOwner, want: true},
		{name: "owner ops", query: "ops=1", role: pauth.RoleOwner, want: true},
		{name: "owner debug", query: "debug=ops", role: pauth.RoleOwner, want: true},
		{name: "viewer ignored", query: "ops=1", role: pauth.RoleViewer, want: false},
		{name: "editor ignored", query: "include_hidden=true", role: pauth.RoleEditor, want: false},
		{name: "missing scope ignored", query: "debug=ops", role: "", want: false},
		{name: "false ignored", query: "include_hidden=false", role: pauth.RoleOwner, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/projects?"+c.query, nil)
			var scope *pauth.ProjectScope
			if c.role != "" {
				scope = &pauth.ProjectScope{Role: c.role}
			}
			if got := includeReaderHiddenProjects(req, scope); got != c.want {
				t.Fatalf("includeReaderHiddenProjects(%q, role=%q) = %v, want %v", c.query, c.role, got, c.want)
			}
		})
	}
}

func TestProjectListReaderHiddenQueryRequiresOwnerIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run project list reader-hidden HTTP DB integration")
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
	hiddenSlug := "oauth-it-" + suffix
	ownerEmail := "project-list-owner-" + suffix + "@example.invalid"
	viewerEmail := "project-list-viewer-" + suffix + "@example.invalid"
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Project List Owner "+suffix, ownerEmail)
	viewerID := insertInviteHTTPUser(t, ctx, pool, "Project List Viewer "+suffix, viewerEmail)
	projectID := insertInviteHTTPProject(t, ctx, pool, hiddenSlug, ownerID)
	insertInviteHTTPMember(t, ctx, pool, projectID, viewerID, pauth.RoleViewer)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, hiddenSlug)
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
		ClientID:           "project-list-" + suffix,
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
		DefaultProjectSlug: hiddenSlug,
		OAuth:              oauthSvc,
		AuthProviders:      []string{config.AuthProviderGitHub},
		BindAddr:           "0.0.0.0:5830",
	})

	viewerOps := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodGet, "/api/projects?ops=1", "")
	if viewerOps.Code != http.StatusOK {
		t.Fatalf("viewer project list status = %d, want 200; body=%s", viewerOps.Code, viewerOps.Body.String())
	}
	if projectListContainsSlug(t, viewerOps, hiddenSlug) {
		t.Fatalf("viewer ?ops=1 response exposed hidden project %q", hiddenSlug)
	}

	ownerOps := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodGet, "/api/projects?ops=1", "")
	if ownerOps.Code != http.StatusOK {
		t.Fatalf("owner project list status = %d, want 200; body=%s", ownerOps.Code, ownerOps.Body.String())
	}
	if !projectListContainsSlug(t, ownerOps, hiddenSlug) {
		t.Fatalf("owner ?ops=1 response did not include hidden project %q", hiddenSlug)
	}
}

func TestHandleProjectListVisibilityMatrixIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run project list visibility HTTP DB integration")
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
	publicSlug := "project-list-public-" + suffix
	orgSlug := "project-list-org-" + suffix
	privateSlug := "project-list-private-" + suffix
	ownerEmail := "project-list-vis-owner-" + suffix + "@example.invalid"
	orgMemberEmail := "project-list-vis-org-" + suffix + "@example.invalid"
	nonMemberEmail := "project-list-vis-non-" + suffix + "@example.invalid"
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Project List Visibility Owner "+suffix, ownerEmail)
	orgMemberID := insertInviteHTTPUser(t, ctx, pool, "Project List Visibility Org "+suffix, orgMemberEmail)
	nonMemberID := insertInviteHTTPUser(t, ctx, pool, "Project List Visibility Non "+suffix, nonMemberEmail)

	insertProjectListVisibilityProject(t, ctx, pool, publicSlug, projects.VisibilityPublic, ownerID)
	insertProjectListVisibilityProject(t, ctx, pool, orgSlug, projects.VisibilityOrg, ownerID)
	insertProjectListVisibilityProject(t, ctx, pool, privateSlug, projects.VisibilityPrivate, ownerID)
	insertProjectListDefaultOrgMember(t, ctx, pool, orgMemberID)
	t.Cleanup(func() {
		for _, slug := range []string{publicSlug, orgSlug, privateSlug} {
			_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
		}
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM users
			 WHERE lower(email) IN ($1, $2, $3)
		`, strings.ToLower(ownerEmail), strings.ToLower(orgMemberEmail), strings.ToLower(nonMemberEmail))
	})

	oauthSvc, err := pauth.NewOAuthService(ctx, pool, pauth.OAuthConfig{
		Issuer:             "http://127.0.0.1:5830",
		PublicBaseURL:      "http://127.0.0.1:5830",
		RedirectBaseURL:    "http://127.0.0.1:5830",
		SigningKeyPath:     t.TempDir() + "/oauth.pem",
		ClientID:           "project-list-vis-" + suffix,
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
		DefaultProjectSlug: publicSlug,
		OAuth:              oauthSvc,
		AuthProviders:      []string{config.AuthProviderGitHub},
		BindAddr:           "0.0.0.0:5830",
	})

	cases := []struct {
		name     string
		userID   string
		loopback bool
		want     []string
		notWant  []string
	}{
		{
			name:   "owner sees all tiers by project membership",
			userID: ownerID,
			want:   []string{publicSlug, orgSlug, privateSlug},
		},
		{
			name:    "org member sees public and org only",
			userID:  orgMemberID,
			want:    []string{publicSlug, orgSlug},
			notWant: []string{privateSlug},
		},
		{
			name:    "non member sees public only",
			userID:  nonMemberID,
			want:    []string{publicSlug},
			notWant: []string{orgSlug, privateSlug},
		},
		{
			name:    "anonymous sees public only",
			want:    []string{publicSlug},
			notWant: []string{orgSlug, privateSlug},
		},
		{
			name:     "trusted local sees all tiers",
			loopback: true,
			want:     []string{publicSlug, orgSlug, privateSlug},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			if c.loopback {
				rec = doLoopbackVisibilityRequest(handler, http.MethodGet, "/api/projects", "")
			} else {
				rec = doInviteRequest(t, handler, oauthSvc, c.userID, http.MethodGet, "/api/projects", "")
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("project list status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
			out := decodeProjectListResponse(t, rec)
			for _, slug := range c.want {
				if _, ok := out.bySlug[slug]; !ok {
					t.Fatalf("response missing visible project %q; got slugs=%v", slug, projectListSlugs(out.projects))
				}
			}
			for _, slug := range c.notWant {
				if row, ok := out.bySlug[slug]; ok {
					t.Fatalf("response exposed non-visible project %q with name=%q artifacts_count=%d", slug, row.Name, row.ArtifactsCount)
				}
			}
			if got, want := out.multiProjectSwitching, len(out.projects) > 1; got != want {
				t.Fatalf("multi_project_switching = %v, want %v for %d returned rows", got, want, len(out.projects))
			}
		})
	}
}

type decodedProjectList struct {
	projects              []projectListRow
	bySlug                map[string]projectListRow
	multiProjectSwitching bool
}

func decodeProjectListResponse(t *testing.T, rec *httptest.ResponseRecorder) decodedProjectList {
	t.Helper()
	var raw struct {
		Projects              []projectListRow `json:"projects"`
		MultiProjectSwitching bool             `json:"multi_project_switching"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode project list: %v", err)
	}
	bySlug := make(map[string]projectListRow, len(raw.Projects))
	for _, project := range raw.Projects {
		bySlug[project.Slug] = project
	}
	return decodedProjectList{
		projects:              raw.Projects,
		bySlug:                bySlug,
		multiProjectSwitching: raw.MultiProjectSwitching,
	}
}

func projectListSlugs(projects []projectListRow) []string {
	out := make([]string, 0, len(projects))
	for _, project := range projects {
		out = append(out, project.Slug)
	}
	return out
}

func insertProjectListVisibilityProject(t *testing.T, ctx context.Context, pool *db.Pool, slug, visibility, ownerID string) string {
	t.Helper()
	var projectID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (slug, name, description, organization_id, primary_language, visibility)
		VALUES ($1, $2, $3, (SELECT id FROM organizations WHERE slug = 'default' LIMIT 1), 'en', $4)
		RETURNING id::text
	`, slug, "Project List "+slug, "private metadata "+slug, visibility).Scan(&projectID); err != nil {
		t.Fatalf("insert visibility project %s: %v", slug, err)
	}
	insertInviteHTTPMember(t, ctx, pool, projectID, ownerID, pauth.RoleOwner)
	return projectID
}

func insertProjectListDefaultOrgMember(t *testing.T, ctx context.Context, pool *db.Pool, userID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role)
		SELECT id, $1::uuid, 'member'
		  FROM organizations
		 WHERE slug = 'default'
		ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, userID); err != nil {
		t.Fatalf("insert default org member: %v", err)
	}
}

func projectListContainsSlug(t *testing.T, rec *httptest.ResponseRecorder, slug string) bool {
	t.Helper()
	var out struct {
		Projects []projectListRow `json:"projects"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode project list: %v", err)
	}
	for _, project := range out.Projects {
		if project.Slug == slug {
			if !project.ReaderHidden {
				t.Fatalf("project %q is present but reader_hidden=false", slug)
			}
			return true
		}
	}
	return false
}

func TestProjectCreateDefaultURLUsesToday(t *testing.T) {
	if got := projectCreateDefaultURL("shop-fe"); got != "/p/shop-fe/today" {
		t.Fatalf("projectCreateDefaultURL() = %q, want /p/shop-fe/today", got)
	}
}

func TestProjectCreateResponseIncludesMCPConnect(t *testing.T) {
	resp := projectCreateResponse{
		ProjectID:        "project-id",
		Slug:             "shop-fe",
		Name:             "Shop Frontend",
		PrimaryLanguage:  "en",
		URL:              projectCreateDefaultURL("shop-fe"),
		DefaultArea:      "misc",
		AreasCreated:     24,
		TemplatesCreated: 4,
		MCPConnect: onboardingMCPConnect{
			URL:         "http://127.0.0.1:5832/mcp",
			MCPJSON:     onboardingMCPJSON("http://127.0.0.1:5832/mcp"),
			AgentPrompt: onboardingAgentPrompt("http://127.0.0.1:5832/mcp", "shop-fe"),
		},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	s := string(body)
	for _, want := range []string{
		`"mcp_connect"`,
		`"url":"http://127.0.0.1:5832/mcp"`,
		`"mcp_json"`,
		`"agent_prompt"`,
		`project_slug=\"shop-fe\"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("project create response JSON missing %s: %s", want, s)
		}
	}
}

func TestProjectCreateRESTBindsOwnerMembershipIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run REST project create owner integration")
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
	ownerEmail := "rest-create-owner-" + suffix + "@example.invalid"
	ownerID := insertInviteHTTPUser(t, ctx, pool, "REST Create Owner "+suffix, ownerEmail)
	projectSlug := "rest-owner-" + suffix
	anonSlug := "rest-anon-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug IN ($1, $2)`, projectSlug, anonSlug)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lower(email) = $1`, strings.ToLower(ownerEmail))
	})

	handler := New(&config.Config{BindAddr: "0.0.0.0:5830"}, Deps{
		DB:                 pool,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultProjectSlug: "pindoc",
		DefaultUserID:      ownerID,
		BindAddr:           "0.0.0.0:5830",
	})

	body := fmt.Sprintf(`{"slug":"%s","name":"REST Owner %s","primary_language":"en"}`, projectSlug, suffix)
	rec := doLoopbackVisibilityRequest(handler, http.MethodPost, "/api/projects", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("project create status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var resp projectCreateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode project create: %v", err)
	}
	if resp.ProjectID == "" || resp.Slug != projectSlug {
		t.Fatalf("project create response = %+v", resp)
	}
	var role string
	if err := pool.QueryRow(ctx, `
		SELECT role
		  FROM project_members
		 WHERE project_id = $1::uuid
		   AND user_id = $2::uuid
	`, resp.ProjectID, ownerID).Scan(&role); err != nil {
		t.Fatalf("select owner membership: %v", err)
	}
	if role != pauth.RoleOwner {
		t.Fatalf("project member role = %q, want owner", role)
	}

	anonBody := fmt.Sprintf(`{"slug":"%s","name":"REST Anonymous %s","primary_language":"en"}`, anonSlug, suffix)
	anonReq := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(anonBody))
	anonReq.RemoteAddr = "203.0.113.10:45678"
	anonReq.Host = "example.test"
	anonReq.Header.Set("Content-Type", "application/json")
	anonRec := httptest.NewRecorder()
	handler.ServeHTTP(anonRec, anonReq)
	if anonRec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous project create status = %d, want 401; body=%s", anonRec.Code, anonRec.Body.String())
	}
	var errResp projectCreateError
	if err := json.NewDecoder(anonRec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode anonymous error: %v", err)
	}
	if errResp.ErrorCode != "INSTANCE_OWNER_REQUIRED" {
		t.Fatalf("anonymous error_code = %q, want INSTANCE_OWNER_REQUIRED", errResp.ErrorCode)
	}
	var anonCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM projects WHERE slug = $1`, anonSlug).Scan(&anonCount); err != nil {
		t.Fatalf("count anonymous project inserts: %v", err)
	}
	if anonCount != 0 {
		t.Fatalf("anonymous project inserts = %d, want 0", anonCount)
	}
}
