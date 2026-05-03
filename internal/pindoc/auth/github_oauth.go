package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	oauth2github "golang.org/x/oauth2/github"

	"github.com/var-gg/pindoc/internal/pindoc/invites"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
	"github.com/var-gg/pindoc/internal/pindoc/users"
)

const (
	githubStateCookieName   = "pindoc_github_oauth_state"
	githubSessionCookieName = "pindoc_oauth_user"
	githubStateTTL          = 10 * time.Minute
	githubSessionTTL        = 30 * 24 * time.Hour
	defaultGitHubAPIBaseURL = "https://api.github.com"
)

type githubOAuth struct {
	config          oauth2.Config
	apiBaseURL      string
	httpClient      *http.Client
	redirectBaseURL string
	secureCookies   bool
}

type githubOAuthState struct {
	InviteToken string `json:"invite"`
	ReturnTo    string `json:"return_to"`
	Nonce       string `json:"nonce"`
	ExpiresAt   int64  `json:"exp"`
}

type browserSessionState struct {
	UserID    string `json:"uid"`
	ExpiresAt int64  `json:"exp"`
}

type githubUserResponse struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

type githubEmailResponse struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func newGitHubOAuth(cfg OAuthConfig, cookieSecret []byte) (*githubOAuth, error) {
	clientID := strings.TrimSpace(cfg.GitHubClientID)
	clientSecret := strings.TrimSpace(cfg.GitHubClientSecret)
	if clientID == "" && clientSecret == "" {
		return nil, nil
	}
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("auth: PINDOC_GITHUB_CLIENT_ID and PINDOC_GITHUB_CLIENT_SECRET are required together")
	}
	redirectBaseURL := normalizeBaseURL(firstNonEmpty(cfg.RedirectBaseURL, cfg.PublicBaseURL, cfg.Issuer))
	if redirectBaseURL == "" {
		return nil, errors.New("auth: oauth redirect base URL is required for GitHub login")
	}
	endpoint := oauth2github.Endpoint
	if strings.TrimSpace(cfg.GitHubAuthURL) != "" {
		endpoint.AuthURL = strings.TrimSpace(cfg.GitHubAuthURL)
	}
	if strings.TrimSpace(cfg.GitHubTokenURL) != "" {
		endpoint.TokenURL = strings.TrimSpace(cfg.GitHubTokenURL)
	}
	apiBaseURL := strings.TrimRight(strings.TrimSpace(cfg.GitHubAPIBaseURL), "/")
	if apiBaseURL == "" {
		apiBaseURL = defaultGitHubAPIBaseURL
	}
	return &githubOAuth{
		config: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     endpoint,
			RedirectURL:  redirectBaseURL + "/auth/github/callback",
			Scopes:       []string{"read:user", "user:email"},
		},
		apiBaseURL:      apiBaseURL,
		httpClient:      cfg.HTTPClient,
		redirectBaseURL: redirectBaseURL,
		secureCookies:   strings.HasPrefix(strings.ToLower(redirectBaseURL), "https://"),
	}, nil
}

func (s *OAuthService) handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	gh := s.currentGitHub()
	if gh == nil {
		http.NotFound(w, r)
		return
	}
	invite := strings.TrimSpace(r.URL.Query().Get("invite"))
	if invite == "" {
		allowed, err := s.allowFirstOwnerSelfSignup(r.Context())
		if err != nil {
			http.Error(w, "first owner signup check failed", http.StatusInternalServerError)
			return
		}
		if !allowed {
			http.Error(w, "invite token is required", http.StatusBadRequest)
			return
		}
	}
	nonce, err := randomHex(16)
	if err != nil {
		http.Error(w, "state generation failed", http.StatusInternalServerError)
		return
	}
	state := githubOAuthState{
		InviteToken: invite,
		ReturnTo:    gh.safeReturnTo(r.URL.Query().Get("return_to")),
		Nonce:       nonce,
		ExpiresAt:   time.Now().Add(githubStateTTL).Unix(),
	}
	signed, err := signJSON(s.cookieSecret, state)
	if err != nil {
		http.Error(w, "state signing failed", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, gh.cookie(githubStateCookieName, signed, int(githubStateTTL.Seconds())))
	http.Redirect(w, r, gh.config.AuthCodeURL(signed), http.StatusFound)
}

func (s *OAuthService) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	gh := s.currentGitHub()
	if gh == nil {
		http.NotFound(w, r)
		return
	}
	if msg := strings.TrimSpace(r.URL.Query().Get("error")); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	rawState := strings.TrimSpace(r.URL.Query().Get("state"))
	cookie, err := r.Cookie(githubStateCookieName)
	if err != nil || rawState == "" || cookie.Value == "" || subtle.ConstantTimeCompare([]byte(rawState), []byte(cookie.Value)) != 1 {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	clearCookie(w, gh.cookie(githubStateCookieName, "", -1))
	var state githubOAuthState
	if err := verifySignedJSON(s.cookieSecret, rawState, &state); err != nil {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	if time.Now().Unix() > state.ExpiresAt {
		http.Error(w, "expired oauth state", http.StatusBadRequest)
		return
	}
	selfSignup := strings.TrimSpace(state.InviteToken) == ""
	if selfSignup {
		allowed, err := s.allowFirstOwnerSelfSignup(r.Context())
		if err != nil {
			http.Error(w, "first owner signup check failed", http.StatusInternalServerError)
			return
		}
		if !allowed {
			http.Error(w, "invite token is required", http.StatusForbidden)
			return
		}
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		http.Error(w, "missing oauth code", http.StatusBadRequest)
		return
	}

	ctx := gh.oauthContext(r.Context())
	token, err := gh.config.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "github token exchange failed", http.StatusBadGateway)
		return
	}
	identity, err := gh.fetchIdentity(ctx, token)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errGitHubVerifiedEmailRequired) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	user, _, err := users.UpsertOAuthUser(r.Context(), s.store.pool, users.OAuthUserInput{
		Provider:     users.ProviderGitHub,
		ProviderUID:  identity.ProviderUID,
		Email:        identity.Email,
		DisplayName:  identity.DisplayName,
		GithubHandle: identity.Login,
	})
	if err != nil {
		http.Error(w, "user upsert failed", http.StatusInternalServerError)
		return
	}
	var joined *invites.Record
	if selfSignup {
		if err := s.ensureFirstOwnerMembership(r.Context(), user.ID); err != nil {
			http.Error(w, "first owner membership failed", http.StatusInternalServerError)
			return
		}
	} else {
		var err error
		joined, err = invites.Consume(r.Context(), s.store.pool, state.InviteToken, user.ID, time.Now().UTC())
		if err != nil {
			status := http.StatusGone
			if !errors.Is(err, invites.ErrTokenInactive) && !errors.Is(err, invites.ErrTokenNotFound) {
				status = http.StatusInternalServerError
			}
			http.Error(w, "invite consume failed", status)
			return
		}
	}
	if err := s.setBrowserSession(w, user.ID); err != nil {
		http.Error(w, "session signing failed", http.StatusInternalServerError)
		return
	}
	returnTo := gh.safeReturnTo(state.ReturnTo)
	if joined != nil && (returnTo == "/" || returnTo == "/signup" || strings.HasPrefix(returnTo, "/signup?")) && joined.ProjectSlug != "" {
		returnTo = "/p/" + url.PathEscape(joined.ProjectSlug) + "/today"
	}
	http.Redirect(w, r, returnTo, http.StatusFound)
}

func (s *OAuthService) allowFirstOwnerSelfSignup(ctx context.Context) (bool, error) {
	if s == nil || s.store == nil || s.store.pool == nil {
		return false, errors.New("auth: nil OAuthService")
	}
	projectSlug := strings.TrimSpace(s.defaultProjectSlug)
	if projectSlug != "" {
		var projectExists, ownerExists bool
		err := s.store.pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM projects WHERE slug = $1),
			       EXISTS (
			           SELECT 1
			             FROM projects p
			             JOIN project_members pm ON pm.project_id = p.id
			            WHERE p.slug = $1 AND pm.role = 'owner'
			       )
		`, projectSlug).Scan(&projectExists, &ownerExists)
		if err != nil {
			return false, fmt.Errorf("first owner project gate: %w", err)
		}
		if projectExists {
			return !ownerExists, nil
		}
	}
	var anyUsers bool
	if err := s.store.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE deleted_at IS NULL)
	`).Scan(&anyUsers); err != nil {
		return false, fmt.Errorf("first owner user gate: %w", err)
	}
	return !anyUsers, nil
}

func (s *OAuthService) ensureFirstOwnerMembership(ctx context.Context, userID string) error {
	if s == nil || s.store == nil || s.store.pool == nil {
		return errors.New("auth: nil OAuthService")
	}
	return projects.EnsureDefaultProjectOwnerMembership(ctx, s.store.pool, s.defaultProjectSlug, userID)
}

func (s *OAuthService) handleLogout(w http.ResponseWriter, r *http.Request) {
	gh := s.currentGitHub()
	if gh != nil {
		clearCookie(w, gh.cookie(githubSessionCookieName, "", -1))
		clearCookie(w, gh.cookie(githubStateCookieName, "", -1))
	}
	w.WriteHeader(http.StatusNoContent)
}

type githubIdentity struct {
	ProviderUID string
	Login       string
	DisplayName string
	Email       string
}

var errGitHubVerifiedEmailRequired = errors.New("github verified primary email is required")

func (g *githubOAuth) fetchIdentity(ctx context.Context, token *oauth2.Token) (githubIdentity, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	if g.httpClient != nil {
		client = oauth2.NewClient(g.oauthContext(ctx), oauth2.StaticTokenSource(token))
	}
	var user githubUserResponse
	if err := g.fetchJSON(ctx, client, g.apiBaseURL+"/user", &user); err != nil {
		return githubIdentity{}, err
	}
	var emails []githubEmailResponse
	if err := g.fetchJSON(ctx, client, g.apiBaseURL+"/user/emails", &emails); err != nil {
		return githubIdentity{}, err
	}
	email := selectPrimaryVerifiedGitHubEmail(emails)
	if email == "" {
		return githubIdentity{}, errGitHubVerifiedEmailRequired
	}
	login := strings.TrimSpace(user.Login)
	displayName := strings.TrimSpace(user.Name)
	if displayName == "" {
		displayName = login
	}
	if displayName == "" {
		displayName = email[:strings.Index(email, "@")]
	}
	providerUID := strconv.FormatInt(user.ID, 10)
	if providerUID == "0" && login != "" {
		providerUID = login
	}
	if providerUID == "0" {
		return githubIdentity{}, errors.New("github user id is required")
	}
	return githubIdentity{
		ProviderUID: providerUID,
		Login:       login,
		DisplayName: displayName,
		Email:       email,
	}, nil
}

func (g *githubOAuth) fetchJSON(ctx context.Context, client *http.Client, endpoint string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("github api %s returned %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode github api response: %w", err)
	}
	return nil
}

func selectPrimaryVerifiedGitHubEmail(emails []githubEmailResponse) string {
	for _, item := range emails {
		email := strings.ToLower(strings.TrimSpace(item.Email))
		if item.Primary && item.Verified && strings.Contains(email, "@") {
			return email
		}
	}
	return ""
}

func (g *githubOAuth) oauthContext(ctx context.Context) context.Context {
	if g.httpClient == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, g.httpClient)
}

func (g *githubOAuth) signupURLForAuthorize(r *http.Request) string {
	returnTo := "/"
	if r != nil && r.URL != nil {
		returnTo = r.URL.RequestURI()
	}
	q := url.Values{}
	q.Set("return_to", returnTo)
	return "/signup?" + q.Encode()
}

func (g *githubOAuth) safeReturnTo(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/"
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "/"
	}
	base, err := url.Parse(g.redirectBaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "/"
	}
	if !strings.EqualFold(u.Scheme, base.Scheme) || !strings.EqualFold(u.Host, base.Host) {
		return "/"
	}
	out := u.EscapedPath()
	if out == "" {
		out = "/"
	}
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		out += "#" + u.EscapedFragment()
	}
	return out
}

func (g *githubOAuth) cookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   g.secureCookies,
	}
}

func clearCookie(w http.ResponseWriter, cookie *http.Cookie) {
	cookie.Value = ""
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
}

func (s *OAuthService) setBrowserSession(w http.ResponseWriter, userID string) error {
	gh := s.currentGitHub()
	if gh == nil {
		return errors.New("auth: github oauth is not configured")
	}
	value, err := signJSON(s.cookieSecret, browserSessionState{
		UserID:    strings.TrimSpace(userID),
		ExpiresAt: time.Now().Add(githubSessionTTL).Unix(),
	})
	if err != nil {
		return err
	}
	http.SetCookie(w, gh.cookie(githubSessionCookieName, value, int(githubSessionTTL.Seconds())))
	return nil
}

func (s *OAuthService) SetBrowserSessionCookie(w http.ResponseWriter, userID string) error {
	return s.setBrowserSession(w, userID)
}

func (s *OAuthService) browserSessionUserID(r *http.Request) string {
	if s == nil || r == nil || len(s.cookieSecret) == 0 {
		return ""
	}
	cookie, err := r.Cookie(githubSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return ""
	}
	var state browserSessionState
	if err := verifySignedJSON(s.cookieSecret, cookie.Value, &state); err != nil {
		return ""
	}
	if strings.TrimSpace(state.UserID) == "" || time.Now().Unix() > state.ExpiresAt {
		return ""
	}
	return strings.TrimSpace(state.UserID)
}

func (s *OAuthService) BrowserSessionUserID(r *http.Request) string {
	return s.browserSessionUserID(r)
}

func signJSON(secret []byte, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	body64 := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(body64))
	sig64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body64 + "." + sig64, nil
}

func verifySignedJSON(secret []byte, value string, dest any) error {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("invalid signed payload")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(parts[0]))
	want := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	if !hmac.Equal(got, want) {
		return errors.New("invalid signed payload signature")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dest)
}

func randomHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
