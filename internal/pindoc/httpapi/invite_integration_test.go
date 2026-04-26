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
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/invites"
)

func TestInviteIssueConsumeIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run invite HTTP DB integration")
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
	slug := "invite-http-" + suffix
	ownerID := insertInviteHTTPUser(t, ctx, pool, "Invite Owner "+suffix, "invite-owner-"+suffix+"@example.invalid")
	viewerID := insertInviteHTTPUser(t, ctx, pool, "Invite Viewer "+suffix, "invite-viewer-"+suffix+"@example.invalid")
	joinerID := insertInviteHTTPUser(t, ctx, pool, "Invite Joiner "+suffix, "invite-joiner-"+suffix+"@example.invalid")
	projectID := insertInviteHTTPProject(t, ctx, pool, slug, ownerID)
	insertInviteHTTPMember(t, ctx, pool, projectID, viewerID, invites.RoleViewer)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM users
			 WHERE lower(email) IN ($1, $2, $3)
		`, "invite-owner-"+suffix+"@example.invalid", "invite-viewer-"+suffix+"@example.invalid", "invite-joiner-"+suffix+"@example.invalid")
	})

	oauthSvc, err := pauth.NewOAuthService(ctx, pool, pauth.OAuthConfig{
		Issuer:             "http://127.0.0.1:5830",
		PublicBaseURL:      "http://127.0.0.1:5830",
		RedirectBaseURL:    "http://127.0.0.1:5830",
		SigningKeyPath:     t.TempDir() + "/oauth.pem",
		ClientID:           "invite-http-" + suffix,
		RedirectURIs:       []string{"http://127.0.0.1:3846/callback"},
		GitHubClientID:     "fake-gh-client",
		GitHubClientSecret: "fake-gh-secret",
	})
	if err != nil {
		t.Fatalf("NewOAuthService: %v", err)
	}
	handler := New(&config.Config{AuthMode: config.AuthModeOAuthGitHub}, Deps{
		DB:                 pool,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultProjectSlug: slug,
		OAuth:              oauthSvc,
		AuthMode:           config.AuthModeOAuthGitHub,
	})

	ownerProject := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodGet, "/api/p/"+slug, "")
	if ownerProject.Code != http.StatusOK {
		t.Fatalf("owner project status = %d, want 200; body=%s", ownerProject.Code, ownerProject.Body.String())
	}
	var ownerProjectBody projectInfo
	if err := json.NewDecoder(ownerProject.Body).Decode(&ownerProjectBody); err != nil {
		t.Fatalf("decode owner project: %v", err)
	}
	if ownerProjectBody.CurrentRole != pauth.RoleOwner {
		t.Fatalf("owner current_role = %q, want owner", ownerProjectBody.CurrentRole)
	}

	viewerProject := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodGet, "/api/p/"+slug, "")
	if viewerProject.Code != http.StatusOK {
		t.Fatalf("viewer project status = %d, want 200; body=%s", viewerProject.Code, viewerProject.Body.String())
	}
	var viewerProjectBody projectInfo
	if err := json.NewDecoder(viewerProject.Body).Decode(&viewerProjectBody); err != nil {
		t.Fatalf("decode viewer project: %v", err)
	}
	if viewerProjectBody.CurrentRole != invites.RoleViewer {
		t.Fatalf("viewer current_role = %q, want viewer", viewerProjectBody.CurrentRole)
	}

	anonymousProject := doInviteRequest(t, handler, nil, "", http.MethodGet, "/api/p/"+slug, "")
	if anonymousProject.Code != http.StatusOK {
		t.Fatalf("anonymous project status = %d, want 200; body=%s", anonymousProject.Code, anonymousProject.Body.String())
	}
	var anonymousProjectBody projectInfo
	if err := json.NewDecoder(anonymousProject.Body).Decode(&anonymousProjectBody); err != nil {
		t.Fatalf("decode anonymous project: %v", err)
	}
	if anonymousProjectBody.CurrentRole != "" {
		t.Fatalf("anonymous current_role = %q, want empty", anonymousProjectBody.CurrentRole)
	}

	ownerRoleResp := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPost, "/api/p/"+slug+"/invite", `{"role":"owner"}`)
	if ownerRoleResp.Code != http.StatusBadRequest {
		t.Fatalf("owner role issue status = %d, want 400; body=%s", ownerRoleResp.Code, ownerRoleResp.Body.String())
	}

	viewerIssue := doInviteRequest(t, handler, oauthSvc, viewerID, http.MethodPost, "/api/p/"+slug+"/invite", `{"role":"editor"}`)
	if viewerIssue.Code != http.StatusForbidden {
		t.Fatalf("non-owner issue status = %d, want 403; body=%s", viewerIssue.Code, viewerIssue.Body.String())
	}

	issue := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPost, "/api/p/"+slug+"/invite", `{"role":"editor"}`)
	if issue.Code != http.StatusOK {
		t.Fatalf("owner issue status = %d, want 200; body=%s", issue.Code, issue.Body.String())
	}
	var issueBody inviteIssueResponse
	if err := json.NewDecoder(issue.Body).Decode(&issueBody); err != nil {
		t.Fatalf("decode issue response: %v", err)
	}
	inviteURL, err := url.Parse(issueBody.InviteURL)
	if err != nil {
		t.Fatalf("parse invite URL: %v", err)
	}
	if inviteURL.Path != "/signup" {
		t.Fatalf("invite URL path = %q, want /signup", inviteURL.Path)
	}
	rawToken := inviteURL.Query().Get("invite")
	if !strings.HasPrefix(rawToken, "jt_") {
		t.Fatalf("invite token = %q, want jt_ prefix", rawToken)
	}

	info := doInviteRequest(t, handler, nil, "", http.MethodGet, "/join?invite="+url.QueryEscape(rawToken), "")
	if info.Code != http.StatusOK {
		t.Fatalf("join info status = %d, want 200; body=%s", info.Code, info.Body.String())
	}
	var infoBody inviteJoinInfoResponse
	if err := json.NewDecoder(info.Body).Decode(&infoBody); err != nil {
		t.Fatalf("decode join info: %v", err)
	}
	if infoBody.ProjectSlug != slug || infoBody.Role != invites.RoleEditor {
		t.Fatalf("join info = %+v", infoBody)
	}

	join := doInviteRequest(t, handler, oauthSvc, joinerID, http.MethodPost, "/join", `{"invite_token":"`+rawToken+`"}`)
	if join.Code != http.StatusOK {
		t.Fatalf("join status = %d, want 200; body=%s", join.Code, join.Body.String())
	}
	assertInviteHTTPMemberRole(t, ctx, pool, projectID, joinerID, invites.RoleEditor)

	consumedInfo := doInviteRequest(t, handler, nil, "", http.MethodGet, "/join?invite="+url.QueryEscape(rawToken), "")
	if consumedInfo.Code != http.StatusGone {
		t.Fatalf("consumed GET status = %d, want 410; body=%s", consumedInfo.Code, consumedInfo.Body.String())
	}
	consumedJoin := doInviteRequest(t, handler, oauthSvc, joinerID, http.MethodPost, "/join", `{"invite_token":"`+rawToken+`"}`)
	if consumedJoin.Code != http.StatusGone {
		t.Fatalf("consumed POST status = %d, want 410; body=%s", consumedJoin.Code, consumedJoin.Body.String())
	}

	viewerToken := issueInviteViaHTTP(t, handler, oauthSvc, ownerID, slug, invites.RoleViewer)
	viewerJoin := doInviteRequest(t, handler, oauthSvc, joinerID, http.MethodPost, "/join", `{"invite_token":"`+viewerToken+`"}`)
	if viewerJoin.Code != http.StatusOK {
		t.Fatalf("re-join status = %d, want 200; body=%s", viewerJoin.Code, viewerJoin.Body.String())
	}
	assertInviteHTTPMemberRole(t, ctx, pool, projectID, joinerID, invites.RoleViewer)

	expiredToken := insertExpiredInviteHTTPToken(t, ctx, pool, projectID)
	expiredInfo := doInviteRequest(t, handler, nil, "", http.MethodGet, "/join?invite="+url.QueryEscape(expiredToken), "")
	if expiredInfo.Code != http.StatusGone {
		t.Fatalf("expired GET status = %d, want 410; body=%s", expiredInfo.Code, expiredInfo.Body.String())
	}
	expiredJoin := doInviteRequest(t, handler, oauthSvc, joinerID, http.MethodPost, "/join", `{"invite_token":"`+expiredToken+`"}`)
	if expiredJoin.Code != http.StatusGone {
		t.Fatalf("expired POST status = %d, want 410; body=%s", expiredJoin.Code, expiredJoin.Body.String())
	}
}

func issueInviteViaHTTP(t *testing.T, handler http.Handler, oauthSvc *pauth.OAuthService, ownerID, slug, role string) string {
	t.Helper()
	rec := doInviteRequest(t, handler, oauthSvc, ownerID, http.MethodPost, "/api/p/"+slug+"/invite", `{"role":"`+role+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("issue %s invite status = %d, body=%s", role, rec.Code, rec.Body.String())
	}
	var out inviteIssueResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s invite: %v", role, err)
	}
	u, err := url.Parse(out.InviteURL)
	if err != nil {
		t.Fatalf("parse %s invite URL: %v", role, err)
	}
	return u.Query().Get("invite")
}

func doInviteRequest(t *testing.T, handler http.Handler, oauthSvc *pauth.OAuthService, userID, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Host = "reader.example.invalid"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if oauthSvc != nil && userID != "" {
		cookieRec := httptest.NewRecorder()
		if err := oauthSvc.SetBrowserSessionCookie(cookieRec, userID); err != nil {
			t.Fatalf("set session cookie: %v", err)
		}
		for _, cookie := range cookieRec.Result().Cookies() {
			req.AddCookie(cookie)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func insertInviteHTTPUser(t *testing.T, ctx context.Context, pool *db.Pool, name, email string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ($1, $2, 'pindoc_admin')
		RETURNING id::text
	`, name, email).Scan(&id); err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	return id
}

func insertInviteHTTPProject(t *testing.T, ctx context.Context, pool *db.Pool, slug, ownerID string) string {
	t.Helper()
	var projectID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (slug, name, owner_id, primary_language)
		VALUES ($1, $2, $3, 'en')
		RETURNING id::text
	`, slug, "Invite HTTP "+slug, "owner-"+slug).Scan(&projectID); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	insertInviteHTTPMember(t, ctx, pool, projectID, ownerID, pauth.RoleOwner)
	return projectID
}

func insertInviteHTTPMember(t *testing.T, ctx context.Context, pool *db.Pool, projectID, userID, role string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, $3)
		ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, projectID, userID, role); err != nil {
		t.Fatalf("insert member role %s: %v", role, err)
	}
}

func assertInviteHTTPMemberRole(t *testing.T, ctx context.Context, pool *db.Pool, projectID, userID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `
		SELECT role FROM project_members
		 WHERE project_id = $1::uuid AND user_id = $2::uuid
	`, projectID, userID).Scan(&got); err != nil {
		t.Fatalf("select member role: %v", err)
	}
	if got != want {
		t.Fatalf("member role = %q, want %q", got, want)
	}
}

func insertExpiredInviteHTTPToken(t *testing.T, ctx context.Context, pool *db.Pool, projectID string) string {
	t.Helper()
	raw, err := invites.GenerateToken()
	if err != nil {
		t.Fatalf("generate expired token: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO invite_tokens (token_hash, project_id, role, expires_at)
		VALUES ($1, $2::uuid, 'viewer', now() - interval '1 hour')
	`, invites.HashToken(raw), projectID); err != nil {
		t.Fatalf("insert expired invite: %v", err)
	}
	return raw
}
