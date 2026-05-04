package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/ory/fosite"
)

type OAuthClientCreateInput struct {
	ClientID          string
	DisplayName       string
	RedirectURIs      []string
	Public            bool
	CreatedByUserID   string
	CreatedVia        string
	CreatedRemoteAddr string
}

type OAuthClientCreateResult struct {
	Client       OAuthClientRecord
	ClientSecret string
}

func (s *OAuthService) CreateClient(ctx context.Context, in OAuthClientCreateInput) (OAuthClientCreateResult, error) {
	if s == nil || s.store == nil {
		return OAuthClientCreateResult{}, errors.New("auth: nil OAuthService")
	}
	clientID := strings.TrimSpace(in.ClientID)
	if clientID == "" {
		var err error
		clientID, err = randomOAuthID("pindoc-client")
		if err != nil {
			return OAuthClientCreateResult{}, err
		}
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		displayName = clientID
	}
	redirectURIs, err := validateRedirectURIs(in.RedirectURIs)
	if err != nil {
		return OAuthClientCreateResult{}, err
	}
	var (
		secret     string
		secretHash []byte
	)
	if !in.Public {
		secret, err = randomOAuthSecret()
		if err != nil {
			return OAuthClientCreateResult{}, err
		}
		secretHash, err = s.secretHasher.Hash(ctx, []byte(secret))
		if err != nil {
			return OAuthClientCreateResult{}, fmt.Errorf("hash oauth client secret: %w", err)
		}
	}
	if err := s.store.UpsertClient(ctx, OAuthClient{
		ID:                clientID,
		DisplayName:       displayName,
		SecretHash:        secretHash,
		RedirectURIs:      redirectURIs,
		GrantTypes:        []string{string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken)},
		ResponseTypes:     []string{"code"},
		Scopes:            SupportedOAuthScopes(),
		Public:            in.Public,
		CreatedByUserID:   strings.TrimSpace(in.CreatedByUserID),
		CreatedVia:        in.CreatedVia,
		CreatedRemoteAddr: strings.TrimSpace(in.CreatedRemoteAddr),
	}); err != nil {
		return OAuthClientCreateResult{}, err
	}
	rec, err := s.store.ClientRecord(ctx, clientID)
	if err != nil {
		return OAuthClientCreateResult{}, err
	}
	return OAuthClientCreateResult{Client: rec, ClientSecret: secret}, nil
}

func (s *OAuthService) DeleteClient(ctx context.Context, clientID string, suppressEnvSeed bool) error {
	if s == nil || s.store == nil {
		return errors.New("auth: nil OAuthService")
	}
	return s.store.DeleteClient(ctx, clientID, suppressEnvSeed)
}

type dcrError struct {
	ErrorCode        string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

const (
	DCRModeClosed = "closed"
	DCRModeOpen   = "open"

	dcrRateLimitPerIP  = 5
	dcrRateLimitWindow = time.Hour
	dcrMaxClients      = 100
	dcrClientLifetime  = 90 * 24 * time.Hour
	dcrClientIdleTTL   = 180 * 24 * time.Hour
	dcrPruneInterval   = 24 * time.Hour
)

type dcrRateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newDCRRateLimiter() *dcrRateLimiter {
	return &dcrRateLimiter{hits: map[string][]time.Time{}}
}

func (l *dcrRateLimiter) Allow(key string, now time.Time, limit int, window time.Duration) bool {
	if l == nil || limit <= 0 || window <= 0 {
		return true
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = "unknown"
	}
	cutoff := now.Add(-window)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneExpiredLocked(cutoff)
	recent := l.hits[key][:0]
	for _, hit := range l.hits[key] {
		if hit.After(cutoff) {
			recent = append(recent, hit)
		}
	}
	if len(recent) == 0 {
		delete(l.hits, key)
	}
	if len(recent) >= limit {
		l.hits[key] = recent
		return false
	}
	recent = append(recent, now)
	l.hits[key] = recent
	return true
}

func (l *dcrRateLimiter) PruneExpired(now time.Time, window time.Duration) int {
	if l == nil || window <= 0 {
		return 0
	}
	cutoff := now.Add(-window)
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.pruneExpiredLocked(cutoff)
}

func (l *dcrRateLimiter) Len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.hits)
}

func (l *dcrRateLimiter) pruneExpiredLocked(cutoff time.Time) int {
	pruned := 0
	for key, hits := range l.hits {
		recent := hits[:0]
		for _, hit := range hits {
			if hit.After(cutoff) {
				recent = append(recent, hit)
			}
		}
		if len(recent) == 0 {
			delete(l.hits, key)
			pruned++
			continue
		}
		l.hits[key] = recent
	}
	return pruned
}

type dcrRegistrationResponse struct {
	oauthex.ClientRegistrationMetadata
	ClientID              string `json:"client_id"`
	ClientSecret          string `json:"client_secret,omitempty"`
	ClientIDIssuedAt      int64  `json:"client_id_issued_at,omitempty"`
	ClientSecretExpiresAt int64  `json:"client_secret_expires_at"`
}

func (s *OAuthService) handleDynamicClientRegistration(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	mode, err := s.DCRMode(r.Context())
	if err != nil {
		writeOAuthJSON(w, http.StatusInternalServerError, dcrError{ErrorCode: "server_error", ErrorDescription: "could not read DCR mode"})
		return
	}
	if mode != DCRModeOpen {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="dynamic client registration is closed"`)
		writeOAuthJSON(w, http.StatusUnauthorized, dcrError{ErrorCode: "invalid_token", ErrorDescription: "dynamic client registration is closed; enable it from the OAuth clients admin panel"})
		return
	}
	remoteAddr := requestRemoteAddr(r)
	if !s.allowDCRRegistration(remoteAddr, now) {
		writeOAuthJSON(w, http.StatusTooManyRequests, dcrError{ErrorCode: "slow_down", ErrorDescription: "dynamic client registration rate limit exceeded"})
		return
	}
	if err := s.ensureDCRCapacity(r.Context(), now); err != nil {
		if errors.Is(err, errDCRClientLimitReached) {
			writeOAuthJSON(w, http.StatusServiceUnavailable, dcrError{ErrorCode: "server_error", ErrorDescription: "dynamic client registration client limit reached"})
			return
		}
		writeOAuthJSON(w, http.StatusInternalServerError, dcrError{ErrorCode: "server_error", ErrorDescription: "could not count DCR clients"})
		return
	}

	var meta oauthex.ClientRegistrationMetadata
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		writeOAuthJSON(w, http.StatusBadRequest, dcrError{ErrorCode: "invalid_client_metadata", ErrorDescription: "could not parse request body as JSON"})
		return
	}
	scopes, err := dcrScopes(meta.Scope)
	if err != nil {
		writeOAuthJSON(w, http.StatusBadRequest, dcrError{ErrorCode: "invalid_client_metadata", ErrorDescription: err.Error()})
		return
	}
	grantTypes, err := dcrGrantTypes(meta.GrantTypes)
	if err != nil {
		writeOAuthJSON(w, http.StatusBadRequest, dcrError{ErrorCode: "invalid_client_metadata", ErrorDescription: err.Error()})
		return
	}
	responseTypes, err := dcrResponseTypes(meta.ResponseTypes)
	if err != nil {
		writeOAuthJSON(w, http.StatusBadRequest, dcrError{ErrorCode: "invalid_client_metadata", ErrorDescription: err.Error()})
		return
	}
	method := strings.TrimSpace(meta.TokenEndpointAuthMethod)
	if method == "" {
		method = "none"
	}
	if method != "none" && method != "client_secret_basic" && method != "client_secret_post" {
		writeOAuthJSON(w, http.StatusBadRequest, dcrError{ErrorCode: "invalid_client_metadata", ErrorDescription: "unsupported token_endpoint_auth_method"})
		return
	}
	redirectURIs, err := validateRedirectURIs(meta.RedirectURIs)
	if err != nil {
		writeOAuthJSON(w, http.StatusBadRequest, dcrError{ErrorCode: "invalid_redirect_uri", ErrorDescription: err.Error()})
		return
	}
	clientID, err := randomOAuthID("pindoc-dcr")
	if err != nil {
		writeOAuthJSON(w, http.StatusInternalServerError, dcrError{ErrorCode: "server_error", ErrorDescription: "could not generate client id"})
		return
	}
	public := method == "none"
	var (
		secret     string
		secretHash []byte
	)
	if !public {
		secret, err = randomOAuthSecret()
		if err != nil {
			writeOAuthJSON(w, http.StatusInternalServerError, dcrError{ErrorCode: "server_error", ErrorDescription: "could not generate client secret"})
			return
		}
		secretHash, err = s.secretHasher.Hash(r.Context(), []byte(secret))
		if err != nil {
			writeOAuthJSON(w, http.StatusInternalServerError, dcrError{ErrorCode: "server_error", ErrorDescription: "could not hash client secret"})
			return
		}
	}
	displayName := strings.TrimSpace(meta.ClientName)
	if displayName == "" {
		displayName = clientID
	}
	expiresAt := now.Add(dcrClientLifetime)
	if err := s.store.UpsertClient(r.Context(), OAuthClient{
		ID:                clientID,
		DisplayName:       displayName,
		SecretHash:        secretHash,
		RedirectURIs:      redirectURIs,
		GrantTypes:        grantTypes,
		ResponseTypes:     responseTypes,
		Scopes:            scopes,
		Public:            public,
		CreatedVia:        OAuthClientCreatedViaDCR,
		CreatedRemoteAddr: remoteAddr,
		CreatedAt:         &now,
		ExpiresAt:         &expiresAt,
	}); err != nil {
		writeOAuthJSON(w, http.StatusInternalServerError, dcrError{ErrorCode: "server_error", ErrorDescription: "could not store client"})
		return
	}
	rec, err := s.store.ClientRecord(r.Context(), clientID)
	if err != nil {
		writeOAuthJSON(w, http.StatusInternalServerError, dcrError{ErrorCode: "server_error", ErrorDescription: "could not read stored client"})
		return
	}
	issuedAt := rec.CreatedAt
	if issuedAt.IsZero() {
		issuedAt = now
	}
	if rec.ExpiresAt != nil {
		expiresAt = *rec.ExpiresAt
	}
	writeOAuthJSON(w, http.StatusCreated, dcrRegistrationResponse{
		ClientRegistrationMetadata: oauthex.ClientRegistrationMetadata{
			RedirectURIs:            redirectURIs,
			TokenEndpointAuthMethod: method,
			GrantTypes:              grantTypes,
			ResponseTypes:           responseTypes,
			ClientName:              displayName,
			Scope:                   strings.Join(scopes, " "),
			ClientURI:               meta.ClientURI,
			LogoURI:                 meta.LogoURI,
			Contacts:                meta.Contacts,
			TOSURI:                  meta.TOSURI,
			PolicyURI:               meta.PolicyURI,
			JWKSURI:                 meta.JWKSURI,
			JWKS:                    meta.JWKS,
			SoftwareID:              meta.SoftwareID,
			SoftwareVersion:         meta.SoftwareVersion,
			SoftwareStatement:       meta.SoftwareStatement,
		},
		ClientID:              clientID,
		ClientSecret:          secret,
		ClientIDIssuedAt:      issuedAt.UTC().Unix(),
		ClientSecretExpiresAt: dcrSecretExpiresAtUnix(expiresAt),
	})
}

var errDCRClientLimitReached = errors.New("auth: dcr client limit reached")

func (s *OAuthService) ensureDCRCapacity(ctx context.Context, now time.Time) error {
	if s == nil || s.store == nil {
		return errors.New("auth: nil OAuthService")
	}
	total, err := s.store.CountDCRClients(ctx)
	if err != nil {
		return err
	}
	if total < dcrMaxClients {
		return nil
	}
	if _, err := s.PruneDCRClients(ctx, now); err != nil {
		return err
	}
	total, err = s.store.CountDCRClients(ctx)
	if err != nil {
		return err
	}
	if total >= dcrMaxClients {
		return errDCRClientLimitReached
	}
	return nil
}

func (s *OAuthService) PruneDCRClients(ctx context.Context, now time.Time) (DCRPruneResult, error) {
	if s == nil || s.store == nil {
		return DCRPruneResult{}, errors.New("auth: nil OAuthService")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return s.store.PruneDCRClients(ctx, now.UTC(), dcrClientIdleTTL, s.defaultProjectSlug)
}

func (s *OAuthService) runDCRPruneLoop(ctx context.Context) {
	ticker := time.NewTicker(dcrPruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			_, _ = s.PruneDCRClients(ctx, now.UTC())
		}
	}
}

func (s *OAuthService) allowDCRRegistration(remoteAddr string, now time.Time) bool {
	if s == nil {
		return false
	}
	if s.dcrLimiter == nil {
		s.dcrLimiter = newDCRRateLimiter()
	}
	return s.dcrLimiter.Allow(remoteAddr, now, dcrRateLimitPerIP, dcrRateLimitWindow)
}

func (s *OAuthService) DCRMode(ctx context.Context) (string, error) {
	if s == nil || s.store == nil {
		return DCRModeClosed, errors.New("auth: nil OAuthService")
	}
	return s.store.DCRMode(ctx)
}

func (s *OAuthService) SetDCRMode(ctx context.Context, mode string) (string, error) {
	if s == nil || s.store == nil {
		return DCRModeClosed, errors.New("auth: nil OAuthService")
	}
	return s.store.SetDCRMode(ctx, mode)
}

func normalizeDCRMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case DCRModeOpen:
		return DCRModeOpen
	default:
		return DCRModeClosed
	}
}

func requestRemoteAddr(r *http.Request) string {
	if r == nil {
		return ""
	}
	remote := strings.TrimSpace(r.RemoteAddr)
	host, port, err := net.SplitHostPort(remote)
	if err == nil && strings.TrimSpace(host) != "" && strings.TrimSpace(port) != "" {
		return net.JoinHostPort(strings.TrimSpace(host), strings.TrimSpace(port))
	}
	return remote
}

func dcrSecretExpiresAtUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().Unix()
}

func dcrScopes(raw string) ([]string, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return SupportedOAuthScopes(), nil
	}
	for _, scope := range fields {
		if !isSupportedOAuthScope(scope) {
			return nil, fmt.Errorf("unsupported scope %q", scope)
		}
	}
	return normalizeOAuthScopes(fields), nil
}

func dcrGrantTypes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken)}, nil
	}
	out := make([]string, 0, len(raw))
	for _, gt := range raw {
		gt = strings.TrimSpace(gt)
		switch gt {
		case string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken):
			out = append(out, gt)
		default:
			return nil, fmt.Errorf("unsupported grant_type %q", gt)
		}
	}
	return normalizeOAuthScopes(out), nil
}

func dcrResponseTypes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{"code"}, nil
	}
	out := make([]string, 0, len(raw))
	for _, rt := range raw {
		rt = strings.TrimSpace(rt)
		if rt != "code" {
			return nil, fmt.Errorf("unsupported response_type %q", rt)
		}
		out = append(out, rt)
	}
	return normalizeOAuthScopes(out), nil
}

func randomOAuthID(prefix string) (string, error) {
	token, err := randomHex(12)
	if err != nil {
		return "", fmt.Errorf("generate oauth client id: %w", err)
	}
	return strings.Trim(strings.TrimSpace(prefix), "-") + "-" + token, nil
}

func randomOAuthSecret() (string, error) {
	token, err := randomHex(32)
	if err != nil {
		return "", fmt.Errorf("generate oauth client secret: %w", err)
	}
	return "pds_" + token, nil
}
