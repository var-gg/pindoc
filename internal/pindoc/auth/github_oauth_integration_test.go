package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/invites"
)

func TestGitHubOAuthCallbackIntegration(t *testing.T) {
	ctx, pool := openOAuthIntegrationDB(t)
	suffix := uniqueOAuthSuffix()
	providerUID := fmt.Sprintf("%d", time.Now().UnixNano())
	email := fmt.Sprintf("github-%s@example.invalid", suffix)
	existingID := insertGitHubTrustedLocalUser(t, ctx, pool, "Existing GitHub "+suffix, strings.ToUpper(email))
	ownerID := insertOAuthTestUser(t, ctx, pool, "invite-owner-"+suffix)
	projectSlug, projectID := insertGitHubInviteProject(t, ctx, pool, suffix, ownerID)
	rawInvite, _, err := invites.Issue(ctx, pool, invites.IssueInput{
		ProjectID: projectID,
		Role:      invites.RoleEditor,
		IssuedBy:  ownerID,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("issue invite: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, projectSlug)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM users
			 WHERE lower(email) IN ($1, $2) OR provider_uid = $3
		`, strings.ToLower(email), "oauth-invite-owner-"+suffix+"@example.invalid", providerUID)
	})

	fakeGitHub := fakeGitHubServer(t, fakeGitHubIdentity{
		ID:       providerUID,
		Login:    "octo-" + suffix,
		Name:     "Octo Codex",
		Email:    email,
		Verified: true,
	})
	defer fakeGitHub.Close()

	var mux *http.ServeMux
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mux == nil {
			http.Error(w, "test mux not ready", http.StatusServiceUnavailable)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	defer ts.Close()

	clientID := "github-client-" + suffix
	redirectURI := ts.URL + "/client/callback"
	svc, err := NewOAuthService(ctx, pool, OAuthConfig{
		Issuer:             ts.URL,
		PublicBaseURL:      ts.URL,
		RedirectBaseURL:    ts.URL,
		SigningKeyPath:     t.TempDir() + "/oauth.pem",
		ClientID:           clientID,
		RedirectURIs:       []string{redirectURI},
		GitHubClientID:     "fake-gh-client",
		GitHubClientSecret: "fake-gh-secret",
		GitHubAuthURL:      fakeGitHub.URL + "/login",
		GitHubTokenURL:     fakeGitHub.URL + "/token",
		GitHubAPIBaseURL:   fakeGitHub.URL,
	})
	if err != nil {
		t.Fatalf("NewOAuthService: %v", err)
	}
	mux = http.NewServeMux()
	svc.RegisterRoutes(mux)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	httpClient := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := httpClient.Get(ts.URL + "/auth/github/login")
	if err != nil {
		t.Fatalf("missing-invite login request: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("missing-invite login status = %d, want 400, body=%s", resp.StatusCode, string(body))
	}
	_ = resp.Body.Close()

	verifier := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJK"
	authURL := "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {ScopePindoc + " " + ScopeOfflineAccess},
		"state":                 {"client-state-" + suffix},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}.Encode()
	loginURL := ts.URL + "/auth/github/login?" + url.Values{
		"invite":    {rawInvite},
		"return_to": {authURL},
	}.Encode()

	loc := redirectLocation(t, httpClient, loginURL)
	fakeGitHubURL, err := url.Parse(fakeGitHub.URL)
	if err != nil {
		t.Fatalf("parse fake GitHub URL: %v", err)
	}
	if loc.Host != fakeGitHubURL.Host {
		t.Fatalf("login redirected to %s, want fake GitHub %s", loc.String(), fakeGitHub.URL)
	}
	loc = redirectLocation(t, httpClient, loc.String())
	if loc.Path != "/auth/github/callback" {
		t.Fatalf("GitHub redirected to %s, want callback", loc.String())
	}
	loc = redirectLocation(t, httpClient, loc.String())
	if loc.Path != "/oauth/authorize" {
		t.Fatalf("callback redirected to %s, want authorize", loc.String())
	}
	loc = redirectLocation(t, httpClient, loc.String())
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatalf("authorize redirect missing code: %s", loc.String())
	}

	token := tokenRequest(t, ts.URL, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}, http.StatusOK)
	accessToken := requireString(t, token, "access_token")
	info, err := svc.TokenVerifier(ctx, accessToken, nil)
	if err != nil {
		t.Fatalf("TokenVerifier: %v", err)
	}
	if info.UserID != existingID || !contains(info.Scopes, ScopePindoc) {
		t.Fatalf("TokenInfo = %+v, want user %s with pindoc scope", info, existingID)
	}

	var source, provider, storedProviderUID, handle, storedEmail string
	if err := pool.QueryRow(ctx, `
		SELECT source, provider, provider_uid, github_handle, email
		  FROM users
		 WHERE id = $1::uuid
	`, existingID).Scan(&source, &provider, &storedProviderUID, &handle, &storedEmail); err != nil {
		t.Fatalf("select linked user: %v", err)
	}
	if source != "github_oauth" || provider != "github" || storedProviderUID != providerUID || handle != "octo-"+suffix {
		t.Fatalf("linked fields = source=%q provider=%q provider_uid=%q handle=%q", source, provider, storedProviderUID, handle)
	}
	if storedEmail != strings.ToLower(email) {
		t.Fatalf("stored email = %q, want lowercase", storedEmail)
	}
	var role string
	if err := pool.QueryRow(ctx, `
		SELECT role FROM project_members
		 WHERE project_id = $1::uuid AND user_id = $2::uuid
	`, projectID, existingID).Scan(&role); err != nil {
		t.Fatalf("select joined membership: %v", err)
	}
	if role != invites.RoleEditor {
		t.Fatalf("joined role = %q, want editor", role)
	}
}

func TestSelectPrimaryVerifiedGitHubEmail(t *testing.T) {
	got := selectPrimaryVerifiedGitHubEmail([]githubEmailResponse{
		{Email: "secondary@example.invalid", Primary: false, Verified: true},
		{Email: "primary@example.invalid", Primary: true, Verified: false},
		{Email: "Verified@Example.Invalid", Primary: true, Verified: true},
	})
	if got != "verified@example.invalid" {
		t.Fatalf("selectPrimaryVerifiedGitHubEmail = %q", got)
	}
	if got := selectPrimaryVerifiedGitHubEmail([]githubEmailResponse{{Email: "primary@example.invalid", Primary: true}}); got != "" {
		t.Fatalf("unverified primary email = %q, want empty", got)
	}
}

type fakeGitHubIdentity struct {
	ID       string
	Login    string
	Name     string
	Email    string
	Verified bool
}

func fakeGitHubServer(t *testing.T, identity fakeGitHubIdentity) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		callback := r.URL.Query().Get("redirect_uri")
		if callback == "" {
			http.Error(w, "missing redirect_uri", http.StatusBadRequest)
			return
		}
		u, err := url.Parse(callback)
		if err != nil {
			http.Error(w, "bad redirect_uri", http.StatusBadRequest)
			return
		}
		q := u.Query()
		q.Set("code", "fake-code")
		q.Set("state", r.URL.Query().Get("state"))
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, _ *http.Request) {
		writeOAuthJSON(w, http.StatusOK, map[string]any{
			"access_token": "fake-token",
			"token_type":   "bearer",
		})
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, _ *http.Request) {
		var id int64
		_, _ = fmt.Sscanf(identity.ID, "%d", &id)
		writeOAuthJSON(w, http.StatusOK, githubUserResponse{
			ID:    id,
			Login: identity.Login,
			Name:  identity.Name,
		})
	})
	mux.HandleFunc("GET /user/emails", func(w http.ResponseWriter, _ *http.Request) {
		writeOAuthJSON(w, http.StatusOK, []githubEmailResponse{{
			Email:    identity.Email,
			Primary:  true,
			Verified: identity.Verified,
		}})
	})
	return httptest.NewServer(mux)
}

func insertGitHubTrustedLocalUser(t *testing.T, ctx context.Context, pool *db.Pool, name, email string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ($1, $2, 'harness_install')
		RETURNING id::text
	`, name, email).Scan(&id); err != nil {
		t.Fatalf("insert trusted local user: %v", err)
	}
	return id
}

func insertGitHubInviteProject(t *testing.T, ctx context.Context, pool *db.Pool, suffix, ownerID string) (string, string) {
	t.Helper()
	slug := "github-invite-" + suffix
	var projectID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (slug, name, owner_id, primary_language)
		VALUES ($1, $2, $3, 'en')
		RETURNING id::text
	`, slug, "GitHub Invite "+suffix, "owner-"+suffix).Scan(&projectID); err != nil {
		t.Fatalf("insert invite project: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, 'owner')
	`, projectID, ownerID); err != nil {
		t.Fatalf("insert invite owner membership: %v", err)
	}
	return slug, projectID
}

func redirectLocation(t *testing.T, client *http.Client, target string) *url.URL {
	t.Helper()
	resp, err := client.Get(target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s status = %d, want redirect, body=%s", target, resp.StatusCode, string(body))
	}
	loc, err := resp.Location()
	if err != nil {
		t.Fatalf("redirect Location for %s: %v", target, err)
	}
	if !loc.IsAbs() {
		loc = resp.Request.URL.ResolveReference(loc)
	}
	return loc
}
