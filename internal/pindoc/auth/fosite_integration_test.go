package auth

import (
	"bytes"
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
	"time"

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

func TestAuthorizeBootstrapFallbackRequiresLoopback(t *testing.T) {
	ctx, pool := openOAuthIntegrationDB(t)
	suffix := uniqueOAuthSuffix()
	userID := insertOAuthTestUser(t, ctx, pool, suffix)

	clientID := "bootstrap-client-" + suffix
	redirectURI := "http://127.0.0.1:3846/callback"
	svc, err := NewOAuthService(ctx, pool, OAuthConfig{
		Issuer:          "http://pindoc.example.test",
		PublicBaseURL:   "http://pindoc.example.test",
		SigningKeyPath:  t.TempDir() + "/oauth.pem",
		ClientID:        clientID,
		RedirectURIs:    []string{redirectURI},
		BootstrapUserID: userID,
	})
	if err != nil {
		t.Fatalf("NewOAuthService: %v", err)
	}

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	verifier := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJK"
	authURL := buildAuthorizeURL(t, "http://pindoc.example.test", clientID, redirectURI, verifier)

	loopbackLoc := authorizeLocationFromHandler(t, mux, authURL, "127.0.0.1:51234")
	if code := loopbackLoc.Query().Get("code"); code == "" {
		t.Fatalf("loopback authorize missing code: %s", loopbackLoc.String())
	}
	if errCode := loopbackLoc.Query().Get("error"); errCode != "" {
		t.Fatalf("loopback authorize error = %q; location=%s", errCode, loopbackLoc.String())
	}

	nonLoopbackLoc := authorizeLocationFromHandler(t, mux, authURL, "203.0.113.10:51234")
	if code := nonLoopbackLoc.Query().Get("code"); code != "" {
		t.Fatalf("non-loopback authorize minted code %q; location=%s", code, nonLoopbackLoc.String())
	}
	if errCode := nonLoopbackLoc.Query().Get("error"); errCode != "access_denied" {
		t.Fatalf("non-loopback error = %q, want access_denied; location=%s", errCode, nonLoopbackLoc.String())
	}
}

func TestDynamicClientRegistrationIntegration(t *testing.T) {
	ctx, pool := openOAuthIntegrationDB(t)
	suffix := uniqueOAuthSuffix()
	userID := insertOAuthTestUser(t, ctx, pool, suffix)
	svc, err := NewOAuthService(ctx, pool, OAuthConfig{
		Issuer:          "http://127.0.0.1:5830",
		PublicBaseURL:   "http://127.0.0.1:5830",
		SigningKeyPath:  t.TempDir() + "/oauth.pem",
		ClientID:        "seed-client-" + suffix,
		RedirectURIs:    []string{"http://127.0.0.1:3846/callback"},
		BootstrapUserID: userID,
	})
	if err != nil {
		t.Fatalf("NewOAuthService: %v", err)
	}

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	body := []byte(`{"client_name":"Codex","redirect_uris":["http://127.0.0.1:3846/callback"],"token_endpoint_auth_method":"none","scope":"pindoc offline_access"}`)
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("closed DCR status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
	if _, err := svc.SetDCRMode(ctx, DCRModeOpen); err != nil {
		t.Fatalf("SetDCRMode(open): %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/oauth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:49000"
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("DCR status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode DCR response: %v", err)
	}
	clientID := requireString(t, out, "client_id")
	if got := requireString(t, out, "client_name"); got != "Codex" {
		t.Fatalf("client_name = %q, want Codex", got)
	}
	issued, ok := out["client_id_issued_at"].(float64)
	if !ok || issued <= 0 {
		t.Fatalf("client_id_issued_at = %#v, want unix timestamp", out["client_id_issued_at"])
	}
	expires, ok := out["client_secret_expires_at"].(float64)
	if !ok || expires <= issued {
		t.Fatalf("client_secret_expires_at = %#v, want timestamp after issued_at %.0f", out["client_secret_expires_at"], issued)
	}
	if got := int64(expires) - int64(issued); got != int64(dcrClientLifetime/time.Second) {
		t.Fatalf("client_secret_expires_at delta = %d, want %d", got, int64(dcrClientLifetime/time.Second))
	}
	recClient, err := svc.Store().ClientRecord(ctx, clientID)
	if err != nil {
		t.Fatalf("ClientRecord(%q): %v", clientID, err)
	}
	if recClient.CreatedVia != OAuthClientCreatedViaDCR || !recClient.Public {
		t.Fatalf("client record = %+v", recClient)
	}
	if recClient.CreatedRemoteAddr != "203.0.113.10:49000" {
		t.Fatalf("CreatedRemoteAddr = %q, want request RemoteAddr", recClient.CreatedRemoteAddr)
	}
	if recClient.ExpiresAt == nil {
		t.Fatal("DCR client ExpiresAt is nil")
	}
	if got := int64(recClient.ExpiresAt.Sub(recClient.CreatedAt).Seconds()); got != int64(dcrClientLifetime/time.Second) {
		t.Fatalf("DCR expires_at delta = %d, want %d", got, int64(dcrClientLifetime/time.Second))
	}
}

func TestConsentFlowIntegration(t *testing.T) {
	ctx, pool := openOAuthIntegrationDB(t)
	suffix := uniqueOAuthSuffix()
	userID := insertOAuthTestUser(t, ctx, pool, suffix)

	redirectURI := "http://127.0.0.1:3846/callback"
	svc, err := NewOAuthService(ctx, pool, OAuthConfig{
		Issuer:         "http://127.0.0.1:5830",
		PublicBaseURL:  "http://127.0.0.1:5830",
		SigningKeyPath: t.TempDir() + "/oauth.pem",
		ClientID:       "seed-consent-client-" + suffix,
		RedirectURIs:   []string{redirectURI},
	})
	if err != nil {
		t.Fatalf("NewOAuthService: %v", err)
	}

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	if _, err := svc.SetDCRMode(ctx, DCRModeOpen); err != nil {
		t.Fatalf("SetDCRMode(open): %v", err)
	}
	clientID := registerPublicDCRClient(t, mux)
	cookies := browserSessionCookies(t, svc, userID)
	verifier := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJK"
	authURL := buildAuthorizeURL(t, "http://127.0.0.1:5830", clientID, redirectURI, verifier)

	first := authorizeLocationFromHandlerWithCookies(t, mux, authURL, "203.0.113.10:51234", cookies)
	if first.Path != "/authorize" {
		t.Fatalf("first authorize path = %q, want /authorize; location=%s", first.Path, first.String())
	}

	info := consentInfoFromHandler(t, mux, first.RawQuery, cookies)
	if info.ConsentNonce == "" {
		t.Fatal("consent info missing consent_nonce")
	}
	if info.CreatedVia != OAuthClientCreatedViaDCR {
		t.Fatalf("consent info created_via = %q, want dcr", info.CreatedVia)
	}
	if info.CreatedAt == "" {
		t.Fatal("consent info missing created_at")
	}
	if len(info.RedirectURIs) != 1 || info.RedirectURIs[0] != redirectURI {
		t.Fatalf("consent info redirect_uris = %#v, want %q", info.RedirectURIs, redirectURI)
	}

	rejectConfirmAuthorize(t, mux, "approve", first.RawQuery, "", "http://127.0.0.1:5830", cookies, http.StatusForbidden)
	rejectConfirmAuthorize(t, mux, "approve", first.RawQuery, info.ConsentNonce, "https://attacker.example", cookies, http.StatusForbidden)

	approve := confirmAuthorize(t, mux, "approve", first.RawQuery, info.ConsentNonce, cookies)
	code := approve.Query().Get("code")
	if code == "" {
		t.Fatalf("approve redirect missing code: %s", approve.String())
	}
	afterAuthorize, err := svc.Store().ClientRecord(ctx, clientID)
	if err != nil {
		t.Fatalf("ClientRecord after authorize: %v", err)
	}
	if afterAuthorize.LastUsedAt == nil {
		t.Fatal("DCR client last_used_at was not touched by authorize success")
	}
	rejectConfirmAuthorize(t, mux, "approve", first.RawQuery, info.ConsentNonce, "http://127.0.0.1:5830", cookies, http.StatusForbidden)
	token := tokenRequestFromHandler(t, mux, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}, http.StatusOK)
	accessToken := requireString(t, token, "access_token")
	if _, err := pool.Exec(ctx, `UPDATE oauth_clients SET last_used_at = NULL WHERE client_id = $1`, clientID); err != nil {
		t.Fatalf("clear last_used_at: %v", err)
	}
	tokenInfo, err := svc.TokenVerifier(ctx, accessToken, nil)
	if err != nil {
		t.Fatalf("TokenVerifier after consent: %v", err)
	}
	if tokenInfo.UserID != userID || !contains(tokenInfo.Scopes, ScopePindoc) {
		t.Fatalf("TokenInfo after consent = %+v", tokenInfo)
	}
	afterVerify, err := svc.Store().ClientRecord(ctx, clientID)
	if err != nil {
		t.Fatalf("ClientRecord after TokenVerifier: %v", err)
	}
	if afterVerify.LastUsedAt == nil {
		t.Fatal("DCR client last_used_at was not touched by token verification")
	}
	granted, err := svc.Store().HasConsent(ctx, userID, clientID, SupportedOAuthScopes())
	if err != nil {
		t.Fatalf("HasConsent: %v", err)
	}
	if !granted {
		t.Fatal("consent grant was not stored")
	}

	second := authorizeLocationFromHandlerWithCookies(t, mux, authURL, "203.0.113.10:51234", cookies)
	if code := second.Query().Get("code"); code == "" {
		t.Fatalf("cached consent authorize missing code: %s", second.String())
	}

	denyClientID := "deny-client-" + suffix
	if err := svc.Store().UpsertClient(ctx, OAuthClient{
		ID:            denyClientID,
		DisplayName:   "Deny Client",
		RedirectURIs:  []string{redirectURI},
		GrantTypes:    []string{string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken)},
		ResponseTypes: []string{"code"},
		Scopes:        SupportedOAuthScopes(),
		Public:        true,
		CreatedVia:    OAuthClientCreatedViaDCR,
	}); err != nil {
		t.Fatalf("Upsert deny client: %v", err)
	}
	denyURL := buildAuthorizeURL(t, "http://127.0.0.1:5830", denyClientID, redirectURI, verifier)
	denyConsent := authorizeLocationFromHandlerWithCookies(t, mux, denyURL, "203.0.113.10:51234", cookies)
	denyInfo := consentInfoFromHandler(t, mux, denyConsent.RawQuery, cookies)
	denied := confirmAuthorize(t, mux, "deny", denyConsent.RawQuery, denyInfo.ConsentNonce, cookies)
	if got := denied.Query().Get("error"); got != "access_denied" {
		t.Fatalf("deny error = %q, want access_denied; location=%s", got, denied.String())
	}
}

func authorizeCode(t *testing.T, serverURL, clientID, redirectURI, verifier string) string {
	t.Helper()
	authURL := buildAuthorizeURL(t, serverURL, clientID, redirectURI, verifier)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(authURL)
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer resp.Body.Close()
	return requireAuthorizeCode(t, resp)
}

func buildAuthorizeURL(t *testing.T, serverURL, clientID, redirectURI, verifier string) string {
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
	return authURL.String()
}

func authorizeLocationFromHandler(t *testing.T, handler http.Handler, target, remoteAddr string) *url.URL {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()
	return requireAuthorizeLocation(t, resp)
}

func authorizeLocationFromHandlerWithCookies(t *testing.T, handler http.Handler, target, remoteAddr string, cookies []*http.Cookie) *url.URL {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RemoteAddr = remoteAddr
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()
	return requireAuthorizeLocation(t, resp)
}

func registerPublicDCRClient(t *testing.T, handler http.Handler) string {
	t.Helper()
	body := []byte(`{"client_name":"Codex","redirect_uris":["http://127.0.0.1:3846/callback"],"token_endpoint_auth_method":"none","scope":"pindoc offline_access"}`)
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("DCR status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode DCR response: %v", err)
	}
	return requireString(t, out, "client_id")
}

func browserSessionCookies(t *testing.T, svc *OAuthService, userID string) []*http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := svc.SetBrowserSessionCookie(rec, userID); err != nil {
		t.Fatalf("SetBrowserSessionCookie: %v", err)
	}
	resp := rec.Result()
	defer resp.Body.Close()
	return resp.Cookies()
}

func consentInfoFromHandler(t *testing.T, handler http.Handler, rawQuery string, cookies []*http.Cookie) consentInfoResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/oauth/consent?"+rawQuery, nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("consent info status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var out consentInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode consent info: %v; body=%s", err, rec.Body.String())
	}
	return out
}

func confirmAuthorize(t *testing.T, handler http.Handler, action, rawQuery, nonce string, cookies []*http.Cookie) *url.URL {
	t.Helper()
	resp := postConfirmAuthorize(t, handler, action, rawQuery, nonce, "http://127.0.0.1:5830", cookies)
	defer resp.Body.Close()
	return requireAuthorizeLocation(t, resp)
}

func rejectConfirmAuthorize(t *testing.T, handler http.Handler, action, rawQuery, nonce, origin string, cookies []*http.Cookie, wantStatus int) {
	t.Helper()
	resp := postConfirmAuthorize(t, handler, action, rawQuery, nonce, origin, cookies)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("confirm status = %d, want %d, body=%s", resp.StatusCode, wantStatus, string(body))
	}
}

func postConfirmAuthorize(t *testing.T, handler http.Handler, action, rawQuery, nonce, origin string, cookies []*http.Cookie) *http.Response {
	t.Helper()
	form := url.Values{
		"action": {action},
		"query":  {rawQuery},
	}
	if strings.TrimSpace(nonce) != "" {
		form.Set("consent_nonce", nonce)
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize/confirm", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if strings.TrimSpace(origin) != "" {
		req.Header.Set("Origin", origin)
	}
	req.RemoteAddr = "203.0.113.10:51234"
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Result()
}

func tokenRequestFromHandler(t *testing.T, handler http.Handler, form url.Values, wantStatus int) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
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

func requireAuthorizeCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	location := requireAuthorizeLocation(t, resp)
	code := location.Query().Get("code")
	if code == "" {
		t.Fatalf("authorize redirect has no code: %s", location.String())
	}
	if state := location.Query().Get("state"); state != "state-0123456789" {
		t.Fatalf("state = %q, want state-0123456789", state)
	}
	return code
}

func requireAuthorizeLocation(t *testing.T, resp *http.Response) *url.URL {
	t.Helper()
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("authorize status = %d, want redirect, body=%s", resp.StatusCode, string(body))
	}
	location, err := resp.Location()
	if err != nil {
		t.Fatalf("authorize Location: %v", err)
	}
	return location
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
