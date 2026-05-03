package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
)

type oauthClientError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

type oauthClientRow struct {
	ClientID        string   `json:"client_id"`
	DisplayName     string   `json:"display_name"`
	RedirectURIs    []string `json:"redirect_uris"`
	GrantTypes      []string `json:"grant_types"`
	ResponseTypes   []string `json:"response_types"`
	Scopes          []string `json:"scopes"`
	Public          bool     `json:"public"`
	HasClientSecret bool     `json:"has_client_secret"`
	CreatedVia      string   `json:"created_via"`
	SeedSuppressed  bool     `json:"seed_suppressed"`
	CreatedAt       string   `json:"created_at"`
}

type oauthClientsListResponse struct {
	Clients []oauthClientRow `json:"clients"`
}

type oauthClientCreateRequest struct {
	ClientID     string   `json:"client_id,omitempty"`
	DisplayName  string   `json:"display_name"`
	RedirectURIs []string `json:"redirect_uris"`
	Public       *bool    `json:"public,omitempty"`
}

type oauthClientCreateResponse struct {
	Status       string         `json:"status"`
	Client       oauthClientRow `json:"client"`
	ClientSecret string         `json:"client_secret,omitempty"`
}

type oauthClientDeleteResponse struct {
	Status string `json:"status"`
}

func writeOAuthClientError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, oauthClientError{ErrorCode: code, Message: message})
}

func (d Deps) handleOAuthClientsList(w http.ResponseWriter, r *http.Request) {
	if d.OAuth == nil {
		writeOAuthClientError(w, http.StatusServiceUnavailable, "OAUTH_UNAVAILABLE", "OAuth service not configured")
		return
	}
	if d.instanceOwner(r) == nil {
		writeOAuthClientError(w, http.StatusForbidden, "INSTANCE_OWNER_REQUIRED", "instance owner only")
		return
	}
	records, err := d.OAuth.Store().ListClients(r.Context())
	if err != nil {
		d.writeOAuthClientErr(w, err, "OAUTH_CLIENTS_LIST_FAILED")
		return
	}
	out := oauthClientsListResponse{Clients: make([]oauthClientRow, 0, len(records))}
	for _, rec := range records {
		out.Clients = append(out.Clients, oauthClientRowFromRecord(rec))
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleOAuthClientsCreate(w http.ResponseWriter, r *http.Request) {
	if d.OAuth == nil {
		writeOAuthClientError(w, http.StatusServiceUnavailable, "OAUTH_UNAVAILABLE", "OAuth service not configured")
		return
	}
	owner := d.instanceOwner(r)
	if owner == nil {
		writeOAuthClientError(w, http.StatusForbidden, "INSTANCE_OWNER_REQUIRED", "instance owner only")
		return
	}
	var in oauthClientCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeOAuthClientError(w, http.StatusBadRequest, "BAD_JSON", "could not parse request body as JSON")
		return
	}
	public := true
	if in.Public != nil {
		public = *in.Public
	}
	result, err := d.OAuth.CreateClient(r.Context(), pauth.OAuthClientCreateInput{
		ClientID:        in.ClientID,
		DisplayName:     in.DisplayName,
		RedirectURIs:    in.RedirectURIs,
		Public:          public,
		CreatedByUserID: owner.UserID,
		CreatedVia:      pauth.OAuthClientCreatedViaAdminUI,
	})
	if err != nil {
		d.writeOAuthClientErr(w, err, "OAUTH_CLIENT_CREATE_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, oauthClientCreateResponse{
		Status:       "created",
		Client:       oauthClientRowFromRecord(result.Client),
		ClientSecret: result.ClientSecret,
	})
}

func (d Deps) handleOAuthClientsDelete(w http.ResponseWriter, r *http.Request) {
	if d.OAuth == nil {
		writeOAuthClientError(w, http.StatusServiceUnavailable, "OAUTH_UNAVAILABLE", "OAuth service not configured")
		return
	}
	if d.instanceOwner(r) == nil {
		writeOAuthClientError(w, http.StatusForbidden, "INSTANCE_OWNER_REQUIRED", "instance owner only")
		return
	}
	clientID := strings.TrimSpace(r.PathValue("clientID"))
	if clientID == "" {
		writeOAuthClientError(w, http.StatusBadRequest, "CLIENT_ID_REQUIRED", "client id is required")
		return
	}
	suppress := r.URL.Query().Get("suppress_env_seed") == "1" || strings.EqualFold(r.URL.Query().Get("suppress_env_seed"), "true")
	if err := d.OAuth.DeleteClient(r.Context(), clientID, suppress); err != nil {
		d.writeOAuthClientErr(w, err, "OAUTH_CLIENT_DELETE_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, oauthClientDeleteResponse{Status: "deleted"})
}

func (d Deps) writeOAuthClientErr(w http.ResponseWriter, err error, fallbackCode string) {
	switch {
	case errors.Is(err, pauth.ErrOAuthClientNotFound):
		writeOAuthClientError(w, http.StatusNotFound, "OAUTH_CLIENT_NOT_FOUND", "OAuth client not found")
	case strings.Contains(err.Error(), "oauth client id is required"),
		strings.Contains(err.Error(), "redirect_uri"),
		strings.Contains(err.Error(), "at least one oauth redirect_uri"):
		writeOAuthClientError(w, http.StatusBadRequest, "OAUTH_CLIENT_INVALID", err.Error())
	default:
		if d.Logger != nil {
			d.Logger.Error("oauth clients handler failed", "err", err, "code", fallbackCode)
		}
		writeOAuthClientError(w, http.StatusInternalServerError, fallbackCode, "internal error")
	}
}

func oauthClientRowFromRecord(rec pauth.OAuthClientRecord) oauthClientRow {
	return oauthClientRow{
		ClientID:        rec.ID,
		DisplayName:     rec.DisplayName,
		RedirectURIs:    rec.RedirectURIs,
		GrantTypes:      rec.GrantTypes,
		ResponseTypes:   rec.ResponseTypes,
		Scopes:          rec.Scopes,
		Public:          rec.Public,
		HasClientSecret: rec.HasSecret,
		CreatedVia:      rec.CreatedVia,
		SeedSuppressed:  rec.SeedSuppressed,
		CreatedAt:       rec.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
