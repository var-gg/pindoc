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
