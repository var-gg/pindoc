package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/invites"
)

type inviteIssueRequest struct {
	Role           string `json:"role"`
	ExpiresInHours int    `json:"expires_in_hours,omitempty"`
}

type inviteIssueResponse struct {
	InviteURL string    `json:"invite_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type inviteJoinInfoResponse struct {
	ProjectSlug string    `json:"project_slug"`
	ProjectName string    `json:"project_name"`
	Role        string    `json:"role"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type inviteJoinRequest struct {
	InviteToken string `json:"invite_token"`
}

type inviteJoinResponse struct {
	Status      string `json:"status"`
	ProjectSlug string `json:"project_slug"`
	ProjectName string `json:"project_name"`
	Role        string `json:"role"`
}

type inviteError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func writeInviteError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, inviteError{ErrorCode: code, Message: message})
}

func (d Deps) handleInviteIssue(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil {
		writeInviteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return
	}
	var in inviteIssueRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeInviteError(w, http.StatusBadRequest, "BAD_JSON", "could not parse request body as JSON")
		return
	}
	role := invites.NormalizeRole(in.Role)
	if !invites.ValidRole(role) {
		writeInviteError(w, http.StatusBadRequest, "INVITE_ROLE_INVALID", "role must be editor or viewer")
		return
	}
	principal, ok := d.principalForInvite(w, r, false)
	if !ok {
		return
	}
	scope, err := pauth.ResolveProject(r.Context(), d.DB, principal, projectSlugFrom(r))
	if err != nil {
		d.writeProjectAuthError(w, err)
		return
	}
	if scope.Role != pauth.RoleOwner {
		writeInviteError(w, http.StatusForbidden, "PROJECT_OWNER_REQUIRED", "only project owners can issue invites")
		return
	}
	hours := in.ExpiresInHours
	if hours == 0 {
		hours = 24
	}
	if hours < 1 || hours > 24*30 {
		writeInviteError(w, http.StatusBadRequest, "INVITE_EXPIRY_INVALID", "expires_in_hours must be between 1 and 720")
		return
	}
	rawToken, rec, err := invites.Issue(r.Context(), d.DB, invites.IssueInput{
		ProjectID: scope.ProjectID,
		Role:      role,
		IssuedBy:  principal.UserID,
		ExpiresAt: time.Now().UTC().Add(time.Duration(hours) * time.Hour),
	})
	if err != nil {
		if errors.Is(err, invites.ErrRoleInvalid) {
			writeInviteError(w, http.StatusBadRequest, "INVITE_ROLE_INVALID", "role must be editor or viewer")
			return
		}
		writeInviteError(w, http.StatusInternalServerError, "INVITE_ISSUE_FAILED", "failed to issue invite")
		return
	}
	u := d.inviteBaseURL(r) + "/join?invite=" + url.QueryEscape(rawToken)
	writeJSON(w, http.StatusOK, inviteIssueResponse{InviteURL: u, ExpiresAt: rec.ExpiresAt})
}

func (d Deps) handleInviteJoinInfo(w http.ResponseWriter, r *http.Request) {
	rec, ok := d.lookupInviteForHTTP(w, r.URL.Query().Get("invite"), r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, inviteJoinInfoResponse{
		ProjectSlug: rec.ProjectSlug,
		ProjectName: rec.ProjectName,
		Role:        rec.Role,
		ExpiresAt:   rec.ExpiresAt,
	})
}

func (d Deps) handleInviteJoin(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil {
		writeInviteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return
	}
	var in inviteJoinRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeInviteError(w, http.StatusBadRequest, "BAD_JSON", "could not parse request body as JSON")
		return
	}
	principal, ok := d.principalForInvite(w, r, true)
	if !ok {
		return
	}
	rec, err := invites.Consume(r.Context(), d.DB, in.InviteToken, principal.UserID, time.Now().UTC())
	if err != nil {
		d.writeInviteLookupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inviteJoinResponse{
		Status:      "accepted",
		ProjectSlug: rec.ProjectSlug,
		ProjectName: rec.ProjectName,
		Role:        rec.Role,
	})
}

func (d Deps) lookupInviteForHTTP(w http.ResponseWriter, rawToken string, r *http.Request) (*invites.Record, bool) {
	if d.DB == nil {
		writeInviteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return nil, false
	}
	rec, err := invites.Lookup(r.Context(), d.DB, rawToken, time.Now().UTC())
	if err != nil {
		d.writeInviteLookupError(w, err)
		return nil, false
	}
	return rec, true
}

func (d Deps) principalForInvite(w http.ResponseWriter, r *http.Request, requireUser bool) (*pauth.Principal, bool) {
	switch d.authMode() {
	case config.AuthModeOAuthGitHub:
		if d.OAuth == nil {
			writeInviteError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "OAuth session is required")
			return nil, false
		}
		userID := d.OAuth.BrowserSessionUserID(r)
		if strings.TrimSpace(userID) == "" {
			writeInviteError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "OAuth session is required")
			return nil, false
		}
		return &pauth.Principal{UserID: userID, AuthMode: pauth.AuthModeOAuthGitHub}, true
	case config.AuthModeTrustedLocal, "":
		if requireUser {
			writeInviteError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authenticated user is required")
			return nil, false
		}
		return &pauth.Principal{AuthMode: pauth.AuthModeTrustedLocal}, true
	default:
		writeInviteError(w, http.StatusForbidden, "AUTH_MODE_LOCKED", "invite endpoints require trusted_local or oauth_github")
		return nil, false
	}
}

func (d Deps) writeProjectAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pauth.ErrProjectSlugRequired):
		writeInviteError(w, http.StatusBadRequest, "PROJECT_SLUG_REQUIRED", "project slug is required")
	case errors.Is(err, pauth.ErrProjectNotFound):
		writeInviteError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found")
	case errors.Is(err, pauth.ErrProjectAccessDenied):
		writeInviteError(w, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "project access denied")
	default:
		writeInviteError(w, http.StatusInternalServerError, "PROJECT_LOOKUP_FAILED", "project lookup failed")
	}
}

func (d Deps) writeInviteLookupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, invites.ErrTokenRequired):
		writeInviteError(w, http.StatusBadRequest, "INVITE_TOKEN_REQUIRED", "invite token is required")
	case errors.Is(err, invites.ErrTokenNotFound):
		writeInviteError(w, http.StatusNotFound, "INVITE_TOKEN_NOT_FOUND", "invite token not found")
	case errors.Is(err, invites.ErrTokenInactive):
		writeInviteError(w, http.StatusGone, "INVITE_TOKEN_INACTIVE", "invite token is expired or already consumed")
	default:
		writeInviteError(w, http.StatusInternalServerError, "INVITE_LOOKUP_FAILED", "invite lookup failed")
	}
}

func (d Deps) inviteBaseURL(r *http.Request) string {
	if d.Settings != nil {
		if base := normalizeHTTPBase(d.Settings.Get().PublicBaseURL); base != "" {
			return base
		}
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	return strings.TrimRight(scheme+"://"+host, "/")
}

func normalizeHTTPBase(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return "http://" + raw
}
