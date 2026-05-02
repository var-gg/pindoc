package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ory/fosite"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestFositePKCEFlowIntegration(t *testing.T) {
	ctx, pool := openOAuthIntegrationDB(t)
	suffix := uniqueOAuthSuffix()
	userID := insertOAuthTestUser(t, ctx, pool, suffix)
	projectSlug := insertOAuthTestProject(t, ctx, pool, suffix, userID)

	clientID := "pkce-client-" + suffix
	svc, err := NewOAuthService(ctx, pool, OAuthConfig{
		Issuer:          "http://127.0.0.1:5830",
		PublicBaseURL:   "http://127.0.0.1:5830",
		SigningKeyPath:  t.TempDir() + "/oauth.pem",
		ClientID:        clientID,
		RedirectURIs:    []string{"http://127.0.0.1:3846/callback"},
		BootstrapUserID: userID,
	})
	if err != nil {
		t.Fatalf("NewOAuthService: %v", err)
	}

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	redirectURI := ts.URL + "/callback"
	if err := svc.Store().UpsertClient(ctx, OAuthClient{
		ID:            clientID,
		RedirectURIs:  []string{redirectURI},
		GrantTypes:    []string{string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken)},
		ResponseTypes: []string{"code"},
		Scopes:        SupportedOAuthScopes(),
		Public:        true,
	}); err != nil {
		t.Fatalf("UpsertClient redirect: %v", err)
	}

	verifier := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJK"
	code := authorizeCode(t, ts.URL, clientID, redirectURI, verifier)
	token := tokenRequest(t, ts.URL, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}, http.StatusOK)
	accessToken := requireString(t, token, "access_token")
	refreshToken := requireString(t, token, "refresh_token")
	if strings.Count(accessToken, ".") != 2 {
		t.Fatalf("access_token should be JWT, got %q", accessToken)
	}

	info, err := svc.TokenVerifier(ctx, accessToken, nil)
	if err != nil {
		t.Fatalf("TokenVerifier: %v", err)
	}
	if info.UserID != userID || !contains(info.Scopes, ScopePindoc) || info.Extra["token_id"] == "" {
		t.Fatalf("TokenInfo = %+v", info)
	}
	scope, err := ResolveProject(ctx, pool, &Principal{UserID: userID, Source: SourceOAuth}, projectSlug)
	if err != nil {
		t.Fatalf("ResolveProject oauth: %v", err)
	}
	if scope.Role != RoleOwner {
		t.Fatalf("oauth project role = %q, want owner", scope.Role)
	}

	rotated := tokenRequest(t, ts.URL, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
	}, http.StatusOK)
	newRefreshToken := requireString(t, rotated, "refresh_token")
	if newRefreshToken == "" || newRefreshToken == refreshToken {
		t.Fatalf("refresh token was not rotated")
	}
	reuse := tokenRequest(t, ts.URL, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
	}, http.StatusBadRequest)
	if got := requireString(t, reuse, "error"); got != "invalid_grant" {
		t.Fatalf("refresh reuse error = %q, want invalid_grant; body=%#v", got, reuse)
	}

	badCode := authorizeCode(t, ts.URL, clientID, redirectURI, verifier)
	badPKCE := tokenRequest(t, ts.URL, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {badCode},
		"redirect_uri":  {redirectURI},
		"code_verifier": {"bad-bad-bad-bad-bad-bad-bad-bad-bad-bad-bad-bad"},
	}, http.StatusBadRequest)
	if got := requireString(t, badPKCE, "error"); got != "invalid_grant" {
		t.Fatalf("bad PKCE error = %q, want invalid_grant; body=%#v", got, badPKCE)
	}
}

func authorizeCode(t *testing.T, serverURL, clientID, redirectURI, verifier string) string {
	t.Helper()
	challenge := pkceChallenge(verifier)
	authURL, err := url.Parse(serverURL + "/oauth/authorize")
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}
	q := authURL.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", ScopePindoc+" "+ScopeOfflineAccess)
	q.Set("state", "state-0123456789")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	authURL.RawQuery = q.Encode()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(authURL.String())
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("authorize status = %d, want redirect, body=%s", resp.StatusCode, string(body))
	}
	location, err := resp.Location()
	if err != nil {
		t.Fatalf("authorize Location: %v", err)
	}
	code := location.Query().Get("code")
	if code == "" {
		t.Fatalf("authorize redirect has no code: %s", location.String())
	}
	if state := location.Query().Get("state"); state != "state-0123456789" {
		t.Fatalf("state = %q, want state-0123456789", state)
	}
	return code
}

func tokenRequest(t *testing.T, serverURL string, form url.Values, wantStatus int) map[string]any {
	t.Helper()
	resp, err := http.PostForm(serverURL+"/oauth/token", form)
	if err != nil {
		t.Fatalf("POST token: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("token status = %d, want %d, body=%s", resp.StatusCode, wantStatus, string(body))
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("token JSON: %v; body=%s", err, string(body))
	}
	return out
}

func insertOAuthTestUser(t *testing.T, ctx context.Context, pool *db.Pool, suffix string) string {
	t.Helper()
	var userID string
	err := pool.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ($1, $2, 'harness_install')
		RETURNING id::text
	`, "OAuth Test "+suffix, fmt.Sprintf("oauth-%s@example.invalid", suffix)).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return userID
}

func insertOAuthTestProject(t *testing.T, ctx context.Context, pool *db.Pool, suffix, userID string) string {
	t.Helper()
	slug := "oauth-it-" + suffix
	var projectID string
	err := pool.QueryRow(ctx, `
		INSERT INTO projects (slug, name, organization_id, primary_language)
		VALUES ($1, $2, (SELECT id FROM organizations WHERE slug = 'default' LIMIT 1), 'en')
		RETURNING id::text
	`, slug, "OAuth IT "+suffix).Scan(&projectID)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, 'owner')
	`, projectID, userID); err != nil {
		t.Fatalf("insert membership: %v", err)
	}
	return slug
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func requireString(t *testing.T, body map[string]any, key string) string {
	t.Helper()
	value, ok := body[key].(string)
	if !ok || value == "" {
		t.Fatalf("%s missing in %#v", key, body)
	}
	return value
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
