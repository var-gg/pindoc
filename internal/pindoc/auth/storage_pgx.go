package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ory/fosite"
	foauth2 "github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/pkce"
	fjwt "github.com/ory/fosite/token/jwt"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

const (
	defaultAuthorizeCodeTTL = 15 * time.Minute
	defaultAccessTokenTTL   = time.Hour
	defaultRefreshTokenTTL  = 30 * 24 * time.Hour

	OAuthClientCreatedViaEnvSeed = "env_seed"
	OAuthClientCreatedViaAdminUI = "admin_ui"
	OAuthClientCreatedViaDCR     = "dcr"
)

var (
	_ fosite.Storage                 = (*FositeStore)(nil)
	_ foauth2.AuthorizeCodeStorage   = (*FositeStore)(nil)
	_ foauth2.AccessTokenStorage     = (*FositeStore)(nil)
	_ foauth2.RefreshTokenStorage    = (*FositeStore)(nil)
	_ foauth2.TokenRevocationStorage = (*FositeStore)(nil)
	_ pkce.PKCERequestStorage        = (*FositeStore)(nil)

	ErrOAuthClientNotFound = errors.New("auth: oauth client not found")
)

// FositeStore implements the fosite storage interfaces directly on pgx.
// All code/token keys are fosite signatures, not raw bearer secrets.
type FositeStore struct {
	pool *db.Pool
}

func NewFositeStore(pool *db.Pool) *FositeStore {
	return &FositeStore{pool: pool}
}

type OAuthClient struct {
	ID              string
	DisplayName     string
	SecretHash      []byte
	RedirectURIs    []string
	GrantTypes      []string
	ResponseTypes   []string
	Scopes          []string
	Public          bool
	CreatedByUserID string
	CreatedVia      string
	SeedSuppressed  bool
}

type OAuthClientRecord struct {
	ID              string
	DisplayName     string
	RedirectURIs    []string
	GrantTypes      []string
	ResponseTypes   []string
	Scopes          []string
	Public          bool
	HasSecret       bool
	CreatedByUserID string
	CreatedVia      string
	SeedSuppressed  bool
	CreatedAt       time.Time
}

func (s *FositeStore) UpsertClient(ctx context.Context, c OAuthClient) error {
	if s == nil || s.pool == nil {
		return errors.New("auth: nil fosite store")
	}
	c.ID = strings.TrimSpace(c.ID)
	if c.ID == "" {
		return errors.New("auth: oauth client id is required")
	}
	c.DisplayName = strings.TrimSpace(c.DisplayName)
	if c.DisplayName == "" {
		c.DisplayName = c.ID
	}
	c.CreatedVia = normalizeOAuthClientCreatedVia(c.CreatedVia)
	if c.CreatedVia == OAuthClientCreatedViaEnvSeed {
		suppressed, err := s.envSeedSuppressed(ctx, c.ID)
		if err != nil {
			return err
		}
		if suppressed {
			return nil
		}
	}
	if len(c.GrantTypes) == 0 {
		c.GrantTypes = []string{"authorization_code", "refresh_token"}
	}
	if len(c.ResponseTypes) == 0 {
		c.ResponseTypes = []string{"code"}
	}
	if len(c.Scopes) == 0 {
		c.Scopes = []string{ScopePindoc, ScopeOfflineAccess}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_clients (
			client_id, display_name, secret_hash, redirect_uris, grant_types,
			response_types, scopes, public, created_by_user_id, created_via,
			seed_suppressed, deleted_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, '')::uuid, $10, $11, NULL)
		ON CONFLICT (client_id) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			secret_hash = EXCLUDED.secret_hash,
			redirect_uris = EXCLUDED.redirect_uris,
			grant_types = EXCLUDED.grant_types,
			response_types = EXCLUDED.response_types,
			scopes = EXCLUDED.scopes,
			public = EXCLUDED.public,
			created_by_user_id = COALESCE(EXCLUDED.created_by_user_id, oauth_clients.created_by_user_id),
			created_via = EXCLUDED.created_via,
			seed_suppressed = EXCLUDED.seed_suppressed,
			deleted_at = NULL
	`, c.ID, c.DisplayName, c.SecretHash, c.RedirectURIs, c.GrantTypes, c.ResponseTypes, c.Scopes, c.Public, c.CreatedByUserID, c.CreatedVia, c.SeedSuppressed)
	if err != nil {
		return fmt.Errorf("upsert oauth client %q: %w", c.ID, err)
	}
	return nil
}

func (s *FositeStore) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("auth: nil fosite store")
	}
	var c fosite.DefaultClient
	err := s.pool.QueryRow(ctx, `
		SELECT client_id, secret_hash, redirect_uris, grant_types, response_types, scopes, public
		  FROM oauth_clients
		 WHERE client_id = $1 AND deleted_at IS NULL
		 LIMIT 1
	`, id).Scan(&c.ID, &c.Secret, &c.RedirectURIs, &c.GrantTypes, &c.ResponseTypes, &c.Scopes, &c.Public)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fosite.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get oauth client %q: %w", id, err)
	}
	return &c, nil
}

func (s *FositeStore) ListClients(ctx context.Context) ([]OAuthClientRecord, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("auth: nil fosite store")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT client_id, display_name, redirect_uris, grant_types, response_types,
		       scopes, public, secret_hash IS NOT NULL AND octet_length(secret_hash) > 0,
		       COALESCE(created_by_user_id::text, ''), created_via, seed_suppressed, created_at
		  FROM oauth_clients
		 WHERE deleted_at IS NULL
		 ORDER BY created_at ASC, client_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list oauth clients: %w", err)
	}
	defer rows.Close()
	out := []OAuthClientRecord{}
	for rows.Next() {
		var rec OAuthClientRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.DisplayName,
			&rec.RedirectURIs,
			&rec.GrantTypes,
			&rec.ResponseTypes,
			&rec.Scopes,
			&rec.Public,
			&rec.HasSecret,
			&rec.CreatedByUserID,
			&rec.CreatedVia,
			&rec.SeedSuppressed,
			&rec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan oauth client: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate oauth clients: %w", err)
	}
	return out, nil
}

func (s *FositeStore) ClientRecord(ctx context.Context, id string) (OAuthClientRecord, error) {
	if s == nil || s.pool == nil {
		return OAuthClientRecord{}, errors.New("auth: nil fosite store")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return OAuthClientRecord{}, ErrOAuthClientNotFound
	}
	var rec OAuthClientRecord
	err := s.pool.QueryRow(ctx, `
		SELECT client_id, display_name, redirect_uris, grant_types, response_types,
		       scopes, public, secret_hash IS NOT NULL AND octet_length(secret_hash) > 0,
		       COALESCE(created_by_user_id::text, ''), created_via, seed_suppressed, created_at
		  FROM oauth_clients
		 WHERE client_id = $1 AND deleted_at IS NULL
		 LIMIT 1
	`, id).Scan(
		&rec.ID,
		&rec.DisplayName,
		&rec.RedirectURIs,
		&rec.GrantTypes,
		&rec.ResponseTypes,
		&rec.Scopes,
		&rec.Public,
		&rec.HasSecret,
		&rec.CreatedByUserID,
		&rec.CreatedVia,
		&rec.SeedSuppressed,
		&rec.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return OAuthClientRecord{}, ErrOAuthClientNotFound
	}
	if err != nil {
		return OAuthClientRecord{}, fmt.Errorf("get oauth client record %q: %w", id, err)
	}
	return rec, nil
}

func (s *FositeStore) DeleteClient(ctx context.Context, id string, suppressEnvSeed bool) error {
	if s == nil || s.pool == nil {
		return errors.New("auth: nil fosite store")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrOAuthClientNotFound
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE oauth_clients
		   SET deleted_at = now(),
		       seed_suppressed = $2
		 WHERE client_id = $1 AND deleted_at IS NULL
	`, id, suppressEnvSeed)
	if err != nil {
		return fmt.Errorf("delete oauth client %q: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrOAuthClientNotFound
	}
	return nil
}

func (s *FositeStore) HasConsent(ctx context.Context, userID, clientID string, scopes []string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("auth: nil fosite store")
	}
	userID = strings.TrimSpace(userID)
	clientID = strings.TrimSpace(clientID)
	scopes = normalizeOAuthScopes(scopes)
	if userID == "" || clientID == "" {
		return false, nil
	}
	if len(scopes) == 0 {
		return true, nil
	}
	var granted []string
	err := s.pool.QueryRow(ctx, `
		SELECT granted_scopes
		  FROM oauth_consents
		 WHERE user_id = $1::uuid AND client_id = $2
	`, userID, clientID).Scan(&granted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lookup oauth consent: %w", err)
	}
	grantedSet := map[string]bool{}
	for _, scope := range granted {
		grantedSet[scope] = true
	}
	for _, scope := range scopes {
		if !grantedSet[scope] {
			return false, nil
		}
	}
	return true, nil
}

func (s *FositeStore) GrantConsent(ctx context.Context, userID, clientID string, scopes []string) error {
	if s == nil || s.pool == nil {
		return errors.New("auth: nil fosite store")
	}
	userID = strings.TrimSpace(userID)
	clientID = strings.TrimSpace(clientID)
	scopes = normalizeOAuthScopes(scopes)
	if userID == "" {
		return errors.New("auth: oauth consent user id is required")
	}
	if clientID == "" {
		return errors.New("auth: oauth consent client id is required")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_consents (user_id, client_id, granted_scopes)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (user_id, client_id) DO UPDATE SET
			granted_scopes = (
				SELECT ARRAY(
					SELECT DISTINCT scope
					  FROM unnest(oauth_consents.granted_scopes || EXCLUDED.granted_scopes) AS scope
					 ORDER BY scope
				)
			),
			updated_at = now()
	`, userID, clientID, scopes)
	if err != nil {
		return fmt.Errorf("grant oauth consent: %w", err)
	}
	return nil
}

func (s *FositeStore) envSeedSuppressed(ctx context.Context, id string) (bool, error) {
	var suppressed bool
	err := s.pool.QueryRow(ctx, `
		SELECT seed_suppressed
		  FROM oauth_clients
		 WHERE client_id = $1 AND deleted_at IS NOT NULL
		 LIMIT 1
	`, id).Scan(&suppressed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lookup oauth env seed suppression %q: %w", id, err)
	}
	return suppressed, nil
}

func (s *FositeStore) ClientAssertionJWTValid(context.Context, string) error {
	return nil
}

func (s *FositeStore) SetClientAssertionJWT(context.Context, string, time.Time) error {
	return nil
}

type storedOAuthRequest struct {
	ID                string              `json:"id"`
	RequestedAt       time.Time           `json:"requested_at"`
	ClientID          string              `json:"client_id"`
	RequestedScopes   []string            `json:"requested_scopes"`
	GrantedScopes     []string            `json:"granted_scopes"`
	RequestedAudience []string            `json:"requested_audience"`
	GrantedAudience   []string            `json:"granted_audience"`
	Form              map[string][]string `json:"form"`
	Session           *foauth2.JWTSession `json:"session"`
}

func (s *FositeStore) CreateAuthorizeCodeSession(ctx context.Context, code string, req fosite.Requester) error {
	rec, formData, sessionData, err := s.serializeRequest(req)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO oauth_authorize_codes (
			code_hash, request_id, client_id, scopes, requested_scopes, requested_at, form_data, session, active
		) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, true)
		ON CONFLICT (code_hash) DO UPDATE SET
			request_id = EXCLUDED.request_id,
			client_id = EXCLUDED.client_id,
			scopes = EXCLUDED.scopes,
			requested_scopes = EXCLUDED.requested_scopes,
			requested_at = EXCLUDED.requested_at,
			form_data = EXCLUDED.form_data,
			session = EXCLUDED.session,
			active = true
	`, code, rec.ID, rec.ClientID, rec.GrantedScopes, rec.RequestedScopes, rec.RequestedAt, formData, sessionData)
	if err != nil {
		return fmt.Errorf("create authorize code session: %w", err)
	}
	return nil
}

func (s *FositeStore) GetAuthorizeCodeSession(ctx context.Context, code string, _ fosite.Session) (fosite.Requester, error) {
	req, active, err := s.getStoredRequest(ctx, `
		SELECT request_id, client_id, requested_scopes, scopes, requested_at, form_data, session, active
		  FROM oauth_authorize_codes
		 WHERE code_hash = $1
	`, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fosite.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !active {
		return req, fosite.ErrInvalidatedAuthorizeCode
	}
	return req, nil
}

func (s *FositeStore) InvalidateAuthorizeCodeSession(ctx context.Context, code string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE oauth_authorize_codes SET active = false WHERE code_hash = $1`, code)
	if err != nil {
		return fmt.Errorf("invalidate authorize code: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fosite.ErrNotFound
	}
	return nil
}

func (s *FositeStore) CreateAccessTokenSession(ctx context.Context, signature string, req fosite.Requester) error {
	rec, formData, sessionData, err := s.serializeRequest(req)
	if err != nil {
		return err
	}
	expiresAt := sessionExpiry(rec.Session, fosite.AccessToken, defaultAccessTokenTTL)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO oauth_access_tokens (
			token_hash, request_id, client_id, scopes, requested_scopes, expires_at, form_data, session
		) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb)
		ON CONFLICT (token_hash) DO UPDATE SET
			request_id = EXCLUDED.request_id,
			client_id = EXCLUDED.client_id,
			scopes = EXCLUDED.scopes,
			requested_scopes = EXCLUDED.requested_scopes,
			expires_at = EXCLUDED.expires_at,
			form_data = EXCLUDED.form_data,
			session = EXCLUDED.session
	`, signature, rec.ID, rec.ClientID, rec.GrantedScopes, rec.RequestedScopes, expiresAt, formData, sessionData)
	if err != nil {
		return fmt.Errorf("create access token session: %w", err)
	}
	return nil
}

func (s *FositeStore) GetAccessTokenSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	req, _, err := s.getStoredRequest(ctx, `
		SELECT request_id, client_id, requested_scopes, scopes, created_at, form_data, session, true
		  FROM oauth_access_tokens
		 WHERE token_hash = $1
	`, signature)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fosite.ErrNotFound
	}
	return req, err
}

func (s *FositeStore) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oauth_access_tokens WHERE token_hash = $1`, signature)
	if err != nil {
		return fmt.Errorf("delete access token session: %w", err)
	}
	return nil
}

func (s *FositeStore) CreateRefreshTokenSession(ctx context.Context, signature, accessSignature string, req fosite.Requester) error {
	rec, formData, sessionData, err := s.serializeRequest(req)
	if err != nil {
		return err
	}
	expiresAt := sessionExpiry(rec.Session, fosite.RefreshToken, defaultRefreshTokenTTL)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO oauth_refresh_tokens (
			token_hash, request_id, client_id, access_token_hash, expires_at, form_data, session, active
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, true)
		ON CONFLICT (token_hash) DO UPDATE SET
			request_id = EXCLUDED.request_id,
			client_id = EXCLUDED.client_id,
			access_token_hash = EXCLUDED.access_token_hash,
			expires_at = EXCLUDED.expires_at,
			form_data = EXCLUDED.form_data,
			session = EXCLUDED.session,
			active = true
	`, signature, rec.ID, rec.ClientID, accessSignature, expiresAt, formData, sessionData)
	if err != nil {
		return fmt.Errorf("create refresh token session: %w", err)
	}
	return nil
}

func (s *FositeStore) GetRefreshTokenSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	req, active, err := s.getStoredRequest(ctx, `
		SELECT request_id, client_id, '{}'::text[], '{}'::text[], created_at, form_data, session, active
		  FROM oauth_refresh_tokens
		 WHERE token_hash = $1
	`, signature)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fosite.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !active {
		return req, fosite.ErrInactiveToken
	}
	return req, nil
}

func (s *FositeStore) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oauth_refresh_tokens WHERE token_hash = $1`, signature)
	if err != nil {
		return fmt.Errorf("delete refresh token session: %w", err)
	}
	return nil
}

func (s *FositeStore) RotateRefreshToken(ctx context.Context, requestID string, refreshTokenSignature string) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE oauth_refresh_tokens
		   SET active = false
		 WHERE request_id = $1 OR token_hash = $2
	`, requestID, refreshTokenSignature); err != nil {
		return fmt.Errorf("rotate refresh token: %w", err)
	}
	return s.RevokeAccessToken(ctx, requestID)
}

func (s *FositeStore) RevokeRefreshToken(ctx context.Context, requestID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE oauth_refresh_tokens SET active = false WHERE request_id = $1`, requestID)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

func (s *FositeStore) RevokeAccessToken(ctx context.Context, requestID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oauth_access_tokens WHERE request_id = $1`, requestID)
	if err != nil {
		return fmt.Errorf("revoke access token: %w", err)
	}
	return nil
}

func (s *FositeStore) CreatePKCERequestSession(ctx context.Context, signature string, req fosite.Requester) error {
	rec, formData, sessionData, err := s.serializeRequest(req)
	if err != nil {
		return err
	}
	form := req.GetRequestForm()
	expiresAt := rec.RequestedAt.Add(defaultAuthorizeCodeTTL)
	if rec.RequestedAt.IsZero() {
		expiresAt = time.Now().UTC().Add(defaultAuthorizeCodeTTL)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO oauth_pkce_requests (
			request_id, client_id, code_challenge, code_challenge_method, expires_at, form_data, session
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb)
		ON CONFLICT (request_id) DO UPDATE SET
			client_id = EXCLUDED.client_id,
			code_challenge = EXCLUDED.code_challenge,
			code_challenge_method = EXCLUDED.code_challenge_method,
			expires_at = EXCLUDED.expires_at,
			form_data = EXCLUDED.form_data,
			session = EXCLUDED.session
	`, signature, rec.ClientID, form.Get("code_challenge"), form.Get("code_challenge_method"), expiresAt, formData, sessionData)
	if err != nil {
		return fmt.Errorf("create pkce request session: %w", err)
	}
	return nil
}

func (s *FositeStore) GetPKCERequestSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	req, _, err := s.getStoredRequest(ctx, `
		SELECT request_id, client_id, '{}'::text[], '{}'::text[], created_at, form_data, session, true
		  FROM oauth_pkce_requests
		 WHERE request_id = $1
	`, signature)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fosite.ErrNotFound
	}
	return req, err
}

func (s *FositeStore) DeletePKCERequestSession(ctx context.Context, signature string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oauth_pkce_requests WHERE request_id = $1`, signature)
	if err != nil {
		return fmt.Errorf("delete pkce request session: %w", err)
	}
	return nil
}

func (s *FositeStore) serializeRequest(req fosite.Requester) (storedOAuthRequest, []byte, []byte, error) {
	if s == nil || s.pool == nil {
		return storedOAuthRequest{}, nil, nil, errors.New("auth: nil fosite store")
	}
	if req == nil {
		return storedOAuthRequest{}, nil, nil, errors.New("auth: nil fosite request")
	}
	client := req.GetClient()
	if client == nil {
		return storedOAuthRequest{}, nil, nil, errors.New("auth: fosite request has nil client")
	}
	session := cloneJWTSession(req.GetSession())
	rec := storedOAuthRequest{
		ID:                req.GetID(),
		RequestedAt:       req.GetRequestedAt(),
		ClientID:          client.GetID(),
		RequestedScopes:   []string(req.GetRequestedScopes()),
		GrantedScopes:     []string(req.GetGrantedScopes()),
		RequestedAudience: []string(req.GetRequestedAudience()),
		GrantedAudience:   []string(req.GetGrantedAudience()),
		Form:              map[string][]string(req.GetRequestForm()),
		Session:           session,
	}
	formData, err := json.Marshal(rec.Form)
	if err != nil {
		return storedOAuthRequest{}, nil, nil, fmt.Errorf("marshal oauth form: %w", err)
	}
	sessionData, err := json.Marshal(rec)
	if err != nil {
		return storedOAuthRequest{}, nil, nil, fmt.Errorf("marshal oauth session: %w", err)
	}
	return rec, formData, sessionData, nil
}

func (s *FositeStore) getStoredRequest(ctx context.Context, query string, arg string) (fosite.Requester, bool, error) {
	var (
		rec             storedOAuthRequest
		requestedScopes []string
		grantedScopes   []string
		formData        []byte
		sessionData     []byte
		active          bool
	)
	err := s.pool.QueryRow(ctx, query, arg).Scan(
		&rec.ID,
		&rec.ClientID,
		&requestedScopes,
		&grantedScopes,
		&rec.RequestedAt,
		&formData,
		&sessionData,
		&active,
	)
	if err != nil {
		return nil, false, err
	}
	if len(sessionData) > 0 {
		if err := json.Unmarshal(sessionData, &rec); err != nil {
			return nil, false, fmt.Errorf("unmarshal oauth session: %w", err)
		}
	}
	if rec.ID == "" {
		rec.ID = arg
	}
	if rec.ClientID == "" {
		return nil, false, errors.New("auth: stored oauth request missing client id")
	}
	if len(requestedScopes) > 0 {
		rec.RequestedScopes = requestedScopes
	}
	if len(grantedScopes) > 0 {
		rec.GrantedScopes = grantedScopes
	}
	if len(formData) > 0 {
		if err := json.Unmarshal(formData, &rec.Form); err != nil {
			return nil, false, fmt.Errorf("unmarshal oauth form: %w", err)
		}
	}
	client, err := s.GetClient(ctx, rec.ClientID)
	if err != nil {
		return nil, false, err
	}
	req := &fosite.Request{
		ID:                rec.ID,
		RequestedAt:       rec.RequestedAt,
		Client:            client,
		RequestedScope:    fosite.Arguments(rec.RequestedScopes),
		GrantedScope:      fosite.Arguments(rec.GrantedScopes),
		RequestedAudience: fosite.Arguments(rec.RequestedAudience),
		GrantedAudience:   fosite.Arguments(rec.GrantedAudience),
		Form:              url.Values(rec.Form),
		Session:           rec.Session,
	}
	if req.Form == nil {
		req.Form = url.Values{}
	}
	if req.Session == nil {
		req.Session = &foauth2.JWTSession{}
	}
	return req, active, nil
}

func cloneJWTSession(session fosite.Session) *foauth2.JWTSession {
	if session == nil {
		return &foauth2.JWTSession{
			JWTClaims: &fjwt.JWTClaims{},
			JWTHeader: fjwt.NewHeaders(),
		}
	}
	if jwtSession, ok := session.(*foauth2.JWTSession); ok {
		cloned := jwtSession.Clone()
		if cloned == nil {
			return &foauth2.JWTSession{JWTClaims: &fjwt.JWTClaims{}, JWTHeader: fjwt.NewHeaders()}
		}
		return cloned.(*foauth2.JWTSession)
	}
	return &foauth2.JWTSession{
		JWTClaims: &fjwt.JWTClaims{Subject: session.GetSubject()},
		JWTHeader: fjwt.NewHeaders(),
		ExpiresAt: map[fosite.TokenType]time.Time{
			fosite.AccessToken:   session.GetExpiresAt(fosite.AccessToken),
			fosite.RefreshToken:  session.GetExpiresAt(fosite.RefreshToken),
			fosite.AuthorizeCode: session.GetExpiresAt(fosite.AuthorizeCode),
		},
		Subject:  session.GetSubject(),
		Username: session.GetUsername(),
	}
}

func sessionExpiry(session *foauth2.JWTSession, tokenType fosite.TokenType, fallback time.Duration) time.Time {
	if session != nil {
		if exp := session.GetExpiresAt(tokenType); !exp.IsZero() {
			return exp
		}
	}
	return time.Now().UTC().Add(fallback)
}

func normalizeOAuthClientCreatedVia(v string) string {
	switch strings.TrimSpace(v) {
	case OAuthClientCreatedViaAdminUI:
		return OAuthClientCreatedViaAdminUI
	case OAuthClientCreatedViaDCR:
		return OAuthClientCreatedViaDCR
	default:
		return OAuthClientCreatedViaEnvSeed
	}
}

func normalizeOAuthScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}
