package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/jackc/pgx/v5"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	foauth2 "github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/token/hmac"
	fjwt "github.com/ory/fosite/token/jwt"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

const (
	ScopePindoc        = "pindoc"
	ScopeOfflineAccess = "offline_access"
)

type OAuthConfig struct {
	Issuer             string
	PublicBaseURL      string
	RedirectBaseURL    string
	SigningKeyPath     string
	ClientID           string
	ClientSecret       string
	RedirectURIs       []string
	BootstrapUserID    string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubAuthURL      string
	GitHubTokenURL     string
	GitHubAPIBaseURL   string
	HTTPClient         *http.Client
}

type OAuthService struct {
	provider        fosite.OAuth2Provider
	store           *FositeStore
	strategy        *foauth2.DefaultJWTStrategy
	secretHasher    fosite.Hasher
	signingKey      *rsa.PrivateKey
	keyID           string
	issuer          string
	publicBaseURL   string
	clientID        string
	bootstrapUserID string
	cookieSecret    []byte

	// github is swapped in-place when the admin UI rotates credentials.
	// Read paths take a snapshot via currentGitHub() so a swap mid-
	// request never tears state. nil = github IdP not configured (404
	// on /auth/github/{login,callback}).
	githubMu sync.RWMutex
	github   *githubOAuth

	// redirectBaseURL is captured at boot so SetGitHubCredentials can
	// rebuild githubOAuth without re-resolving the public base URL on
	// every refresh.
	redirectBaseURL string
}

func NewOAuthService(ctx context.Context, pool *db.Pool, cfg OAuthConfig) (*OAuthService, error) {
	if pool == nil {
		return nil, errors.New("auth: nil DB pool")
	}
	issuer := normalizeBaseURL(firstNonEmpty(cfg.Issuer, cfg.PublicBaseURL))
	if issuer == "" {
		return nil, errors.New("auth: oauth issuer/public_base_url is required")
	}
	publicBaseURL := normalizeBaseURL(firstNonEmpty(cfg.PublicBaseURL, issuer))

	key, err := loadOrCreateRSAKey(strings.TrimSpace(cfg.SigningKeyPath))
	if err != nil {
		return nil, err
	}
	keyID, err := rsaKeyID(key)
	if err != nil {
		return nil, err
	}
	globalSecret := oauthGlobalSecret(key)
	cookieSecret := append([]byte(nil), globalSecret[:]...)

	fositeConfig := &fosite.Config{
		AccessTokenLifespan:            defaultAccessTokenTTL,
		RefreshTokenLifespan:           defaultRefreshTokenTTL,
		AuthorizeCodeLifespan:          defaultAuthorizeCodeTTL,
		AccessTokenIssuer:              issuer,
		TokenURL:                       issuer + "/oauth/token",
		ScopeStrategy:                  fosite.ExactScopeStrategy,
		AudienceMatchingStrategy:       fosite.DefaultAudienceMatchingStrategy,
		EnforcePKCE:                    true,
		EnablePKCEPlainChallengeMethod: false,
		RefreshTokenScopes:             []string{},
		RedirectSecureChecker:          allowHTTPSOrLoopbackRedirect,
		GlobalSecret:                   globalSecret[:],
		HashCost:                       10,
		SendDebugMessagesToClients:     false,
	}
	fositeConfig.ClientSecretsHasher = &fosite.BCrypt{Config: fositeConfig}

	store := NewFositeStore(pool)
	clientID := strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		clientID = "claude-desktop"
	}
	redirectURIs, err := validateRedirectURIs(cfg.RedirectURIs)
	if err != nil {
		return nil, err
	}
	var secretHash []byte
	if strings.TrimSpace(cfg.ClientSecret) != "" {
		secretHash, err = fositeConfig.ClientSecretsHasher.Hash(ctx, []byte(strings.TrimSpace(cfg.ClientSecret)))
		if err != nil {
			return nil, fmt.Errorf("hash oauth client secret: %w", err)
		}
	}
	if err := store.UpsertClient(ctx, OAuthClient{
		ID:              clientID,
		DisplayName:     clientID,
		SecretHash:      secretHash,
		RedirectURIs:    redirectURIs,
		GrantTypes:      []string{string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken)},
		ResponseTypes:   []string{"code"},
		Scopes:          SupportedOAuthScopes(),
		Public:          len(secretHash) == 0,
		CreatedByUserID: strings.TrimSpace(cfg.BootstrapUserID),
		CreatedVia:      OAuthClientCreatedViaEnvSeed,
	}); err != nil {
		return nil, err
	}

	hmacStrategy := foauth2.NewHMACSHAStrategy(&hmac.HMACStrategy{Config: fositeConfig}, fositeConfig)
	jwtStrategy := compose.NewOAuth2JWTStrategy(func(context.Context) (interface{}, error) {
		return key, nil
	}, hmacStrategy, fositeConfig)
	provider := compose.Compose(
		fositeConfig,
		store,
		jwtStrategy,
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2RefreshTokenGrantFactory,
		compose.OAuth2PKCEFactory,
		compose.OAuth2TokenIntrospectionFactory,
		compose.OAuth2TokenRevocationFactory,
	)
	redirectBaseURL := normalizeBaseURL(firstNonEmpty(cfg.RedirectBaseURL, cfg.PublicBaseURL, cfg.Issuer))
	githubOAuth, err := newGitHubOAuth(cfg, cookieSecret)
	if err != nil {
		return nil, err
	}

	return &OAuthService{
		provider:        provider,
		store:           store,
		strategy:        jwtStrategy,
		secretHasher:    fositeConfig.ClientSecretsHasher,
		signingKey:      key,
		keyID:           keyID,
		issuer:          issuer,
		publicBaseURL:   publicBaseURL,
		clientID:        clientID,
		bootstrapUserID: strings.TrimSpace(cfg.BootstrapUserID),
		cookieSecret:    cookieSecret,
		github:          githubOAuth,
		redirectBaseURL: redirectBaseURL,
	}, nil
}

// currentGitHub returns the active githubOAuth snapshot under a read
// lock. Callers receive a stable pointer for the duration of one
// request even if SetGitHubCredentials swaps the field mid-flight.
func (s *OAuthService) currentGitHub() *githubOAuth {
	if s == nil {
		return nil
	}
	s.githubMu.RLock()
	defer s.githubMu.RUnlock()
	return s.github
}

// SetGitHubCredentials swaps the active GitHub OAuth client at
// runtime. Empty client_id + secret unwires the IdP — /auth/github/*
// routes 404 on the next request. Returns an error when the inputs
// are inconsistent (one of the two missing). Decision decision-auth-
// model-loopback-and-providers § 3 — admin UI mutates credentials
// without restarting the daemon.
func (s *OAuthService) SetGitHubCredentials(clientID, clientSecret string) error {
	if s == nil {
		return errors.New("auth: nil OAuthService")
	}
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if clientID == "" && clientSecret == "" {
		s.githubMu.Lock()
		s.github = nil
		s.githubMu.Unlock()
		return nil
	}
	if clientID == "" || clientSecret == "" {
		return errors.New("auth: github client_id and client_secret must both be set")
	}
	next, err := newGitHubOAuth(OAuthConfig{
		PublicBaseURL:      s.publicBaseURL,
		RedirectBaseURL:    s.redirectBaseURL,
		Issuer:             s.issuer,
		GitHubClientID:     clientID,
		GitHubClientSecret: clientSecret,
	}, s.cookieSecret)
	if err != nil {
		return err
	}
	s.githubMu.Lock()
	s.github = next
	s.githubMu.Unlock()
	return nil
}

// HasGitHub reports whether the GitHub IdP is currently wired. Used
// by capability surfaces to advertise login affordance and by tests
// to assert hot-reload.
func (s *OAuthService) HasGitHub() bool {
	return s.currentGitHub() != nil
}

func (s *OAuthService) Store() *FositeStore {
	if s == nil {
		return nil
	}
	return s.store
}

func (s *OAuthService) ResourceMetadataURL() string {
	if s == nil {
		return ""
	}
	return s.publicBaseURL + "/.well-known/oauth-protected-resource"
}

func (s *OAuthService) TokenVerifier(ctx context.Context, token string, _ *http.Request) (*mcpauth.TokenInfo, error) {
	if s == nil || s.provider == nil || strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("%w: missing bearer token", mcpauth.ErrInvalidToken)
	}
	session := s.newJWTSession("")
	_, ar, err := s.provider.IntrospectToken(ctx, token, fosite.AccessToken, session, ScopePindoc)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", mcpauth.ErrInvalidToken, err)
	}
	jwtSession := cloneJWTSession(ar.GetSession())
	subject := strings.TrimSpace(jwtSession.GetSubject())
	if subject == "" && jwtSession.JWTClaims != nil {
		subject = strings.TrimSpace(jwtSession.JWTClaims.Subject)
	}
	if subject == "" {
		return nil, fmt.Errorf("%w: token has no subject", mcpauth.ErrInvalidToken)
	}
	scopes := append([]string(nil), []string(ar.GetGrantedScopes())...)
	expiresAt := jwtSession.GetExpiresAt(fosite.AccessToken)
	if expiresAt.IsZero() && jwtSession.JWTClaims != nil {
		expiresAt = jwtSession.JWTClaims.ExpiresAt
	}
	return &mcpauth.TokenInfo{
		UserID:     subject,
		Scopes:     scopes,
		Expiration: expiresAt,
		Extra: map[string]any{
			"source":    SourceOAuth,
			"token_id":  s.strategy.AccessTokenSignature(ctx, token),
			"client_id": clientIDFromRequester(ar),
		},
	}, nil
}

func (s *OAuthService) RegisterRoutes(mux *http.ServeMux) {
	if s == nil || mux == nil {
		return
	}
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.handleProtectedResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleAuthorizationServerMetadata)
	mux.HandleFunc("GET /.well-known/jwks.json", s.handleJWKS)
	mux.HandleFunc("GET /oauth/authorize", s.handleAuthorize)
	mux.HandleFunc("GET /oauth/consent", s.handleConsentInfo)
	mux.HandleFunc("POST /oauth/authorize/confirm", s.handleAuthorizeConfirm)
	mux.HandleFunc("POST /oauth/token", s.handleToken)
	mux.HandleFunc("POST /oauth/revoke", s.handleRevoke)
	mux.HandleFunc("POST /oauth/register", s.handleDynamicClientRegistration)
	// Always register the github routes so admin UI hot-reload works:
	// if no github IdP is currently configured, the handlers 404 via
	// currentGitHub() == nil. This way provider activation does not
	// require a daemon restart.
	mux.HandleFunc("GET /auth/github/login", s.handleGitHubLogin)
	mux.HandleFunc("GET /auth/github/callback", s.handleGitHubCallback)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
}

// RegisterUnavailableOAuthRoutes keeps OAuth-reserved paths from
// falling through to the Reader SPA when no identity provider is wired.
// MCP clients expect JSON OAuth metadata/errors, not an index.html shell.
func RegisterUnavailableOAuthRoutes(mux *http.ServeMux) {
	if mux == nil {
		return
	}
	for _, path := range []string{
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-authorization-server",
		"/.well-known/jwks.json",
		"/oauth/authorize",
		"/oauth/consent",
		"/oauth/authorize/confirm",
		"/oauth/token",
		"/oauth/revoke",
		"/oauth/register",
		"/auth/github/login",
		"/auth/github/callback",
		"/auth/logout",
	} {
		mux.HandleFunc(path, handleOAuthUnavailable)
	}
}

func handleOAuthUnavailable(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeOAuthJSON(w, http.StatusServiceUnavailable, map[string]string{
		"error": "auth_not_configured",
		"hint":  "set PINDOC_AUTH_PROVIDERS",
	})
}

func (s *OAuthService) handleProtectedResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	writeOAuthJSON(w, http.StatusOK, oauthex.ProtectedResourceMetadata{
		Resource:                          s.publicBaseURL + "/mcp",
		AuthorizationServers:              []string{s.issuer},
		JWKSURI:                           s.issuer + "/.well-known/jwks.json",
		ScopesSupported:                   SupportedOAuthScopes(),
		BearerMethodsSupported:            []string{"header"},
		ResourceSigningAlgValuesSupported: []string{"RS256"},
		ResourceName:                      "Pindoc MCP",
	})
}

func (s *OAuthService) handleAuthorizationServerMetadata(w http.ResponseWriter, _ *http.Request) {
	writeOAuthJSON(w, http.StatusOK, oauthex.AuthServerMeta{
		Issuer:                                 s.issuer,
		AuthorizationEndpoint:                  s.issuer + "/oauth/authorize",
		TokenEndpoint:                          s.issuer + "/oauth/token",
		RegistrationEndpoint:                   s.issuer + "/oauth/register",
		JWKSURI:                                s.issuer + "/.well-known/jwks.json",
		ScopesSupported:                        SupportedOAuthScopes(),
		ResponseTypesSupported:                 []string{"code"},
		GrantTypesSupported:                    []string{string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken)},
		TokenEndpointAuthMethodsSupported:      []string{"none", "client_secret_basic", "client_secret_post"},
		RevocationEndpoint:                     s.issuer + "/oauth/revoke",
		RevocationEndpointAuthMethodsSupported: []string{"none", "client_secret_basic", "client_secret_post"},
		IntrospectionEndpoint:                  "",
		CodeChallengeMethodsSupported:          []string{"S256"},
		ClientIDMetadataDocumentSupported:      true,
	})
}

func (s *OAuthService) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"keys": []jose.JSONWebKey{{
			Key:       &s.signingKey.PublicKey,
			KeyID:     s.keyID,
			Use:       "sig",
			Algorithm: string(jose.RS256),
		}},
	})
}

func (s *OAuthService) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ar, err := s.provider.NewAuthorizeRequest(ctx, r)
	if err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, err)
		return
	}
	grantRequestedScopes(ar)
	for _, audience := range ar.GetRequestedAudience() {
		ar.GrantAudience(audience)
	}
	subject := strings.TrimSpace(s.browserSessionUserID(r))
	fromBootstrap := false
	if subject == "" && IsLoopbackRequest(r) {
		subject = strings.TrimSpace(s.bootstrapUserID)
		fromBootstrap = subject != ""
	}
	if subject == "" {
		if gh := s.currentGitHub(); gh != nil {
			http.Redirect(w, r, gh.signupURLForAuthorize(r), http.StatusFound)
			return
		}
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrAccessDenied.WithHint("GitHub OAuth login is required before authorizing this client."))
		return
	}
	if !fromBootstrap {
		hasConsent, err := s.store.HasConsent(ctx, subject, clientIDFromRequester(ar), []string(ar.GetGrantedScopes()))
		if err != nil {
			s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrServerError.WithDebug(err.Error()))
			return
		}
		if !hasConsent {
			http.Redirect(w, r, "/authorize?"+r.URL.RawQuery, http.StatusFound)
			return
		}
	}
	resp, err := s.provider.NewAuthorizeResponse(ctx, ar, s.newJWTSession(subject))
	if err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, err)
		return
	}
	s.provider.WriteAuthorizeResponse(ctx, w, ar, resp)
}

func (s *OAuthService) handleToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accessRequest, err := s.provider.NewAccessRequest(ctx, r, s.newJWTSession(""))
	if err != nil {
		s.provider.WriteAccessError(ctx, w, accessRequest, err)
		return
	}
	session := cloneJWTSession(accessRequest.GetSession())
	s.prepareSession(session, session.GetSubject())
	accessRequest.SetSession(session)
	grantRequestedScopes(accessRequest)
	for _, audience := range accessRequest.GetRequestedAudience() {
		accessRequest.GrantAudience(audience)
	}
	response, err := s.provider.NewAccessResponse(ctx, accessRequest)
	if err != nil {
		s.provider.WriteAccessError(ctx, w, accessRequest, err)
		return
	}
	s.provider.WriteAccessResponse(ctx, w, accessRequest, response)
}

func (s *OAuthService) handleRevoke(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := s.provider.NewRevocationRequest(ctx, r)
	s.provider.WriteRevocationResponse(ctx, w, err)
}

func (s *OAuthService) newJWTSession(subject string) *foauth2.JWTSession {
	session := &foauth2.JWTSession{
		JWTClaims: &fjwt.JWTClaims{},
		JWTHeader: fjwt.NewHeaders(),
		ExpiresAt: map[fosite.TokenType]time.Time{},
		Subject:   strings.TrimSpace(subject),
		Username:  strings.TrimSpace(subject),
	}
	s.prepareSession(session, subject)
	return session
}

func (s *OAuthService) prepareSession(session *foauth2.JWTSession, subject string) {
	if session == nil {
		return
	}
	now := time.Now().UTC()
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = strings.TrimSpace(session.Subject)
	}
	if subject == "" && session.JWTClaims != nil {
		subject = strings.TrimSpace(session.JWTClaims.Subject)
	}
	if session.JWTClaims == nil {
		session.JWTClaims = &fjwt.JWTClaims{}
	}
	if session.JWTHeader == nil {
		session.JWTHeader = fjwt.NewHeaders()
	}
	if session.ExpiresAt == nil {
		session.ExpiresAt = map[fosite.TokenType]time.Time{}
	}
	if subject != "" {
		session.Subject = subject
		session.Username = subject
		session.JWTClaims.Subject = subject
	}
	session.JWTClaims.Issuer = s.issuer
	if len(session.JWTClaims.Audience) == 0 {
		session.JWTClaims.Audience = []string{s.publicBaseURL + "/mcp"}
	}
	if session.JWTClaims.IssuedAt.IsZero() {
		session.JWTClaims.IssuedAt = now
	}
	if session.JWTClaims.NotBefore.IsZero() {
		session.JWTClaims.NotBefore = now
	}
	if session.JWTClaims.Extra == nil {
		session.JWTClaims.Extra = map[string]interface{}{}
	}
	session.JWTClaims.Extra["source"] = SourceOAuth
	session.JWTHeader.Add("kid", s.keyID)
	if session.GetExpiresAt(fosite.AuthorizeCode).IsZero() {
		session.SetExpiresAt(fosite.AuthorizeCode, now.Add(defaultAuthorizeCodeTTL))
	}
	if session.GetExpiresAt(fosite.AccessToken).IsZero() {
		session.SetExpiresAt(fosite.AccessToken, now.Add(defaultAccessTokenTTL))
	}
	if session.GetExpiresAt(fosite.RefreshToken).IsZero() {
		session.SetExpiresAt(fosite.RefreshToken, now.Add(defaultRefreshTokenTTL))
	}
}

// EnsureBootstrapUser resolves or creates the boot-time owner identity used
// for local loopback convenience. Its returned ID must never authorize
// non-loopback OAuth requests; handleAuthorize gates bootstrap fallback with
// IsLoopbackRequest before using OAuthConfig.BootstrapUserID.
func EnsureBootstrapUser(ctx context.Context, pool *db.Pool, userName, userEmail string) (string, error) {
	if pool == nil {
		return "", errors.New("auth: nil DB pool")
	}
	name := strings.TrimSpace(userName)
	if name == "" {
		return "", nil
	}
	email := strings.ToLower(strings.TrimSpace(userEmail))
	var existingID string
	if email != "" {
		err := pool.QueryRow(ctx,
			`SELECT id::text FROM users WHERE lower(email) = $1 AND deleted_at IS NULL LIMIT 1`,
			email,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("lookup user by email: %w", err)
		}
	}
	if existingID == "" {
		err := pool.QueryRow(ctx,
			`SELECT id::text FROM users WHERE display_name = $1 AND email IS NOT DISTINCT FROM NULLIF($2, '') AND deleted_at IS NULL LIMIT 1`,
			name, email,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("lookup user by name: %w", err)
		}
	}
	if existingID != "" {
		_, err := pool.Exec(ctx,
			`UPDATE users SET display_name = $1, updated_at = now() WHERE id = $2 AND display_name <> $1`,
			name, existingID,
		)
		if err != nil {
			return "", fmt.Errorf("sync display_name: %w", err)
		}
		return existingID, nil
	}
	var newID string
	err := pool.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ($1, NULLIF($2, ''), 'harness_install')
		RETURNING id::text
	`, name, email).Scan(&newID)
	if err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}
	return newID, nil
}

func SupportedOAuthScopes() []string {
	return []string{ScopePindoc, ScopeOfflineAccess}
}

func grantRequestedScopes(req fosite.Requester) {
	if req == nil {
		return
	}
	for _, scope := range req.GetRequestedScopes() {
		if isSupportedOAuthScope(scope) {
			req.GrantScope(scope)
		}
	}
}

func isSupportedOAuthScope(scope string) bool {
	for _, supported := range SupportedOAuthScopes() {
		if scope == supported {
			return true
		}
	}
	return false
}

func validateRedirectURIs(raw []string) ([]string, error) {
	clean := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		u, err := url.Parse(item)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("auth: invalid oauth redirect_uri %q", item)
		}
		if !allowHTTPSOrLoopbackRedirect(context.Background(), u) {
			return nil, fmt.Errorf("auth: oauth redirect_uri must be https or loopback http: %q", item)
		}
		seen[item] = true
		clean = append(clean, item)
	}
	if len(clean) == 0 {
		return nil, errors.New("auth: at least one oauth redirect_uri is required")
	}
	return clean, nil
}

func allowHTTPSOrLoopbackRedirect(_ context.Context, u *url.URL) bool {
	if u == nil {
		return false
	}
	if strings.EqualFold(u.Scheme, "https") {
		return true
	}
	if !strings.EqualFold(u.Scheme, "http") {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loadOrCreateRSAKey(path string) (*rsa.PrivateKey, error) {
	if path == "" {
		return nil, errors.New("auth: oauth signing key path is required")
	}
	if data, err := os.ReadFile(path); err == nil {
		return parseRSAPrivateKey(data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read oauth signing key: %w", err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate oauth signing key: %w", err)
	}
	if err := key.Validate(); err != nil {
		return nil, fmt.Errorf("validate generated oauth signing key: %w", err)
	}
	data, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal oauth signing key: %w", err)
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: data})
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create oauth signing key dir: %w", err)
	}
	if err := os.WriteFile(path, block, 0o600); err != nil {
		return nil, fmt.Errorf("write oauth signing key: %w", err)
	}
	return key, nil
}

func parseRSAPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("auth: oauth signing key is not PEM")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS#1 oauth signing key: %w", err)
		}
		return key, key.Validate()
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS#8 oauth signing key: %w", err)
		}
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("auth: oauth signing key must be RSA")
		}
		return key, key.Validate()
	default:
		return nil, fmt.Errorf("auth: unsupported oauth signing key PEM type %q", block.Type)
	}
}

func rsaKeyID(key *rsa.PrivateKey) (string, error) {
	if key == nil {
		return "", errors.New("auth: nil oauth signing key")
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("marshal oauth public key: %w", err)
	}
	sum := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func oauthGlobalSecret(key *rsa.PrivateKey) [32]byte {
	if key == nil {
		return sha256.Sum256([]byte("pindoc-oauth-empty-key"))
	}
	return sha256.Sum256(x509.MarshalPKCS1PrivateKey(key))
}

func normalizeBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return "http://" + raw
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func clientIDFromRequester(req fosite.Requester) string {
	if req == nil || req.GetClient() == nil {
		return ""
	}
	return req.GetClient().GetID()
}

func writeOAuthJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
