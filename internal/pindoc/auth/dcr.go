package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/ory/fosite"
)

type OAuthClientCreateInput struct {
	ClientID        string
	DisplayName     string
	RedirectURIs    []string
	Public          bool
	CreatedByUserID string
	CreatedVia      string
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
		ID:              clientID,
		DisplayName:     displayName,
		SecretHash:      secretHash,
		RedirectURIs:    redirectURIs,
		GrantTypes:      []string{string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken)},
		ResponseTypes:   []string{"code"},
		Scopes:          SupportedOAuthScopes(),
		Public:          in.Public,
		CreatedByUserID: strings.TrimSpace(in.CreatedByUserID),
		CreatedVia:      in.CreatedVia,
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

func (s *OAuthService) handleDynamicClientRegistration(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.UpsertClient(r.Context(), OAuthClient{
		ID:            clientID,
		DisplayName:   displayName,
		SecretHash:    secretHash,
		RedirectURIs:  redirectURIs,
		GrantTypes:    grantTypes,
		ResponseTypes: responseTypes,
		Scopes:        scopes,
		Public:        public,
		CreatedVia:    OAuthClientCreatedViaDCR,
	}); err != nil {
		writeOAuthJSON(w, http.StatusInternalServerError, dcrError{ErrorCode: "server_error", ErrorDescription: "could not store client"})
		return
	}
	writeOAuthJSON(w, http.StatusCreated, oauthex.ClientRegistrationResponse{
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
		ClientIDIssuedAt:      time.Now().UTC(),
		ClientSecretExpiresAt: time.Time{},
	})
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
