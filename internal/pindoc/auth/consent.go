package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ory/fosite"
)

type consentInfoResponse struct {
	ClientID          string   `json:"client_id"`
	ClientDisplayName string   `json:"client_display_name"`
	Scopes            []string `json:"scopes"`
	AlreadyGranted    bool     `json:"already_granted"`
}

type consentError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
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
	writeOAuthJSON(w, http.StatusOK, consentInfoResponse{
		ClientID:          clientID,
		ClientDisplayName: firstNonEmpty(rec.DisplayName, clientID),
		Scopes:            scopes,
		AlreadyGranted:    granted,
	})
}

func (s *OAuthService) handleAuthorizeConfirm(w http.ResponseWriter, r *http.Request) {
	action, rawQuery, err := parseAuthorizeConfirm(r)
	if err != nil {
		writeOAuthJSON(w, http.StatusBadRequest, consentError{ErrorCode: "BAD_CONFIRM", Message: err.Error()})
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
	if action == "deny" {
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrAccessDenied.WithHint("The user denied the OAuth client request."))
		return
	}
	clientID := clientIDFromRequester(ar)
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

func parseAuthorizeConfirm(r *http.Request) (action string, rawQuery string, err error) {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var in struct {
			Action string `json:"action"`
			Query  string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			return "", "", fmt.Errorf("could not parse request body as JSON")
		}
		action = in.Action
		rawQuery = in.Query
	} else {
		if err := r.ParseForm(); err != nil {
			return "", "", fmt.Errorf("could not parse form")
		}
		action = r.Form.Get("action")
		rawQuery = r.Form.Get("query")
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "approve" && action != "deny" {
		return "", "", fmt.Errorf("action must be approve or deny")
	}
	rawQuery = strings.TrimPrefix(strings.TrimSpace(rawQuery), "?")
	if rawQuery == "" {
		return "", "", fmt.Errorf("query is required")
	}
	return action, rawQuery, nil
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
