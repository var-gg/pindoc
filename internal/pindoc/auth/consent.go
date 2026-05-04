package auth

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ory/fosite"
)

type consentInfoResponse struct {
	ClientID          string   `json:"client_id"`
	ClientDisplayName string   `json:"client_display_name"`
	Scopes            []string `json:"scopes"`
	AlreadyGranted    bool     `json:"already_granted"`
	ConsentNonce      string   `json:"consent_nonce"`
	CreatedVia        string   `json:"created_via"`
	CreatedAt         string   `json:"created_at"`
	RedirectURIs      []string `json:"redirect_uris"`
}

type consentError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

const consentNonceTTL = 5 * time.Minute

type consentNonceRecord struct {
	Subject   string
	ClientID  string
	Scopes    []string
	QueryHash [sha256.Size]byte
	ExpiresAt time.Time
}

func (s *OAuthService) handleConsentInfo(w http.ResponseWriter, r *http.Request) {
	subject := strings.TrimSpace(s.browserSessionUserID(r))
	if subject == "" {
		writeOAuthJSON(w, http.StatusUnauthorized, consentError{ErrorCode: "LOGIN_REQUIRED", Message: "OAuth login is required before consent"})
		return
	}
	ar, err := s.provider.NewAuthorizeRequest(r.Context(), r)
	if err != nil {
		writeOAuthJSON(w, http.StatusBadRequest, consentError{ErrorCode: "AUTHORIZE_REQUEST_INVALID", Message: err.Error()})
		return
	}
	grantRequestedScopes(ar)
	for _, audience := range ar.GetRequestedAudience() {
		ar.GrantAudience(audience)
	}
	clientID := clientIDFromRequester(ar)
	rec, err := s.store.ClientRecord(r.Context(), clientID)
	if errors.Is(err, ErrOAuthClientNotFound) {
		writeOAuthJSON(w, http.StatusNotFound, consentError{ErrorCode: "CLIENT_NOT_FOUND", Message: "OAuth client not found"})
		return
	}
	if err != nil {
		writeOAuthJSON(w, http.StatusInternalServerError, consentError{ErrorCode: "CLIENT_LOOKUP_FAILED", Message: "could not load OAuth client"})
		return
	}
	scopes := []string(ar.GetGrantedScopes())
	granted, err := s.store.HasConsent(r.Context(), subject, clientID, scopes)
	if err != nil {
		writeOAuthJSON(w, http.StatusInternalServerError, consentError{ErrorCode: "CONSENT_LOOKUP_FAILED", Message: "could not load OAuth consent"})
		return
	}
	nonce, err := s.createConsentNonce(subject, clientID, scopes, r.URL.RawQuery)
	if err != nil {
		writeOAuthJSON(w, http.StatusInternalServerError, consentError{ErrorCode: "CONSENT_NONCE_FAILED", Message: "could not create OAuth consent nonce"})
		return
	}
	writeOAuthJSON(w, http.StatusOK, consentInfoResponse{
		ClientID:          clientID,
		ClientDisplayName: firstNonEmpty(rec.DisplayName, clientID),
		Scopes:            scopes,
		AlreadyGranted:    granted,
		ConsentNonce:      nonce,
		CreatedVia:        rec.CreatedVia,
		CreatedAt:         rec.CreatedAt.UTC().Format(time.RFC3339),
		RedirectURIs:      append([]string(nil), rec.RedirectURIs...),
	})
}

func (s *OAuthService) handleAuthorizeConfirm(w http.ResponseWriter, r *http.Request) {
	action, rawQuery, nonce, err := parseAuthorizeConfirm(r)
	if err != nil {
		writeOAuthJSON(w, http.StatusBadRequest, consentError{ErrorCode: "BAD_CONFIRM", Message: err.Error()})
		return
	}
	if err := s.validateAuthorizeConfirmOrigin(r); err != nil {
		writeOAuthJSON(w, http.StatusForbidden, consentError{ErrorCode: "CSRF_DETECTED", Message: err.Error()})
		return
	}
	replay, err := s.authorizeReplayRequest(r, rawQuery)
	if err != nil {
		writeOAuthJSON(w, http.StatusBadRequest, consentError{ErrorCode: "BAD_CONFIRM", Message: err.Error()})
		return
	}
	ctx := r.Context()
	ar, err := s.provider.NewAuthorizeRequest(ctx, replay)
	if err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, err)
		return
	}
	grantRequestedScopes(ar)
	for _, audience := range ar.GetRequestedAudience() {
		ar.GrantAudience(audience)
	}
	subject := strings.TrimSpace(s.browserSessionUserID(r))
	if subject == "" {
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrAccessDenied.WithHint("OAuth login is required before authorizing this client."))
		return
	}
	clientID := clientIDFromRequester(ar)
	scopes := []string(ar.GetGrantedScopes())
	if err := s.consumeConsentNonce(nonce, subject, clientID, scopes, rawQuery); err != nil {
		writeOAuthJSON(w, http.StatusForbidden, consentError{ErrorCode: "CSRF_DETECTED", Message: err.Error()})
		return
	}
	if action == "deny" {
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrAccessDenied.WithHint("The user denied the OAuth client request."))
		return
	}
	if err := s.store.GrantConsent(ctx, subject, clientID, []string(ar.GetGrantedScopes())); err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrServerError.WithDebug(err.Error()))
		return
	}
	resp, err := s.provider.NewAuthorizeResponse(ctx, ar, s.newJWTSession(subject))
	if err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, err)
		return
	}
	s.provider.WriteAuthorizeResponse(ctx, w, ar, resp)
}

func parseAuthorizeConfirm(r *http.Request) (action string, rawQuery string, nonce string, err error) {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var in struct {
			Action       string `json:"action"`
			Query        string `json:"query"`
			ConsentNonce string `json:"consent_nonce"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			return "", "", "", fmt.Errorf("could not parse request body as JSON")
		}
		action = in.Action
		rawQuery = in.Query
		nonce = in.ConsentNonce
	} else {
		if err := r.ParseForm(); err != nil {
			return "", "", "", fmt.Errorf("could not parse form")
		}
		action = r.Form.Get("action")
		rawQuery = r.Form.Get("query")
		nonce = r.Form.Get("consent_nonce")
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "approve" && action != "deny" {
		return "", "", "", fmt.Errorf("action must be approve or deny")
	}
	rawQuery = strings.TrimPrefix(strings.TrimSpace(rawQuery), "?")
	if rawQuery == "" {
		return "", "", "", fmt.Errorf("query is required")
	}
	return action, rawQuery, strings.TrimSpace(nonce), nil
}

func (s *OAuthService) authorizeReplayRequest(r *http.Request, rawQuery string) (*http.Request, error) {
	target := s.issuer + "/oauth/authorize"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	replay, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	replay.Header = r.Header.Clone()
	replay.RemoteAddr = r.RemoteAddr
	replay.Host = r.Host
	replay.TLS = r.TLS
	return replay, nil
}

func (s *OAuthService) createConsentNonce(subject, clientID string, scopes []string, rawQuery string) (string, error) {
	if s == nil {
		return "", errors.New("auth: nil OAuthService")
	}
	token, err := randomHex(32)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	s.consentNonceMu.Lock()
	defer s.consentNonceMu.Unlock()
	if s.consentNonces == nil {
		s.consentNonces = map[string]consentNonceRecord{}
	}
	for nonce, rec := range s.consentNonces {
		if !rec.ExpiresAt.After(now) {
			delete(s.consentNonces, nonce)
		}
	}
	s.consentNonces[token] = consentNonceRecord{
		Subject:   strings.TrimSpace(subject),
		ClientID:  strings.TrimSpace(clientID),
		Scopes:    normalizeOAuthScopes(scopes),
		QueryHash: consentQueryHash(rawQuery),
		ExpiresAt: now.Add(consentNonceTTL),
	}
	return token, nil
}

func (s *OAuthService) consumeConsentNonce(token, subject, clientID string, scopes []string, rawQuery string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("consent nonce is required")
	}
	now := time.Now().UTC()
	s.consentNonceMu.Lock()
	defer s.consentNonceMu.Unlock()
	rec, ok := s.consentNonces[token]
	if !ok {
		return errors.New("consent nonce is invalid or already used")
	}
	delete(s.consentNonces, token)
	if !rec.ExpiresAt.After(now) {
		return errors.New("consent nonce expired")
	}
	if rec.Subject != strings.TrimSpace(subject) {
		return errors.New("consent nonce subject mismatch")
	}
	if rec.ClientID != strings.TrimSpace(clientID) {
		return errors.New("consent nonce client mismatch")
	}
	if strings.Join(rec.Scopes, "\x00") != strings.Join(normalizeOAuthScopes(scopes), "\x00") {
		return errors.New("consent nonce scope mismatch")
	}
	if rec.QueryHash != consentQueryHash(rawQuery) {
		return errors.New("consent nonce authorize request mismatch")
	}
	return nil
}

func consentQueryHash(rawQuery string) [sha256.Size]byte {
	rawQuery = strings.TrimPrefix(strings.TrimSpace(rawQuery), "?")
	return sha256.Sum256([]byte(rawQuery))
}

func (s *OAuthService) validateAuthorizeConfirmOrigin(r *http.Request) error {
	expected, err := url.Parse(s.issuer)
	if err != nil || expected.Scheme == "" || expected.Host == "" {
		return errors.New("OAuth issuer origin is not configured")
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		actual, err := url.Parse(origin)
		if err != nil || actual.Scheme == "" || actual.Host == "" {
			return errors.New("Origin header is invalid")
		}
		if sameOrigin(actual, expected) {
			return nil
		}
		return errors.New("Origin header is not same-origin")
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		actual, err := url.Parse(referer)
		if err != nil || actual.Scheme == "" || actual.Host == "" {
			return errors.New("Referer header is invalid")
		}
		if sameOrigin(actual, expected) {
			return nil
		}
		return errors.New("Referer header is not same-origin")
	}
	return errors.New("Origin or Referer header is required")
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
