package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/providers"
)

// task-providers-admin-ui — instance-level IdP registry over HTTP.
//
// GET    /api/instance/providers           list all rows (decrypted client_secret hidden)
// POST   /api/instance/providers           upsert one row by provider_name
// DELETE /api/instance/providers/{idOrName} disable + drop one row
//
// Authorization: instance owner only. For now that means "loopback
// principal" (the operator on the box) — admin UI hot-reload is not
// usable from cross-device OAuth callers in this first land. A future
// pass can promote the first OAuth signup to instance owner.

type providerError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

type providerListResponse struct {
	Providers []providers.PublicRecord `json:"providers"`
}

type providerUpsertRequest struct {
	ProviderName string         `json:"provider_name"`
	DisplayName  string         `json:"display_name,omitempty"`
	ClientID     string         `json:"client_id"`
	ClientSecret string         `json:"client_secret,omitempty"`
	Config       map[string]any `json:"config,omitempty"`
	Enabled      *bool          `json:"enabled,omitempty"`
}

type providerOpResponse struct {
	Status   string                  `json:"status"`
	Provider *providers.PublicRecord `json:"provider,omitempty"`
}

func writeProviderError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, providerError{ErrorCode: code, Message: message})
}

// instanceOwner returns the loopback principal when present, or nil
// when the caller is not allowed to mutate the instance registry. The
// admin UI rule is "instance owner only"; this build defines the
// instance owner as the loopback principal — the operator on the box
// — until users.is_instance_owner lands as a follow-up.
func (d Deps) instanceOwner(r *http.Request) *pauth.Principal {
	principal := d.principalForRequest(r)
	if principal == nil || !principal.IsLoopback() {
		return nil
	}
	return principal
}

func (d Deps) handleInstanceProvidersList(w http.ResponseWriter, r *http.Request) {
	if d.Providers == nil {
		writeProviderError(w, http.StatusServiceUnavailable, "PROVIDERS_UNAVAILABLE", "instance provider store not configured")
		return
	}
	if d.instanceOwner(r) == nil {
		writeProviderError(w, http.StatusForbidden, "INSTANCE_OWNER_REQUIRED", "instance owner only")
		return
	}
	rows, err := d.Providers.List(r.Context())
	if err != nil {
		d.writeProvidersErr(w, err, "PROVIDERS_LIST_FAILED")
		return
	}
	out := providerListResponse{Providers: make([]providers.PublicRecord, 0, len(rows))}
	for _, rec := range rows {
		out.Providers = append(out.Providers, rec.ToPublic())
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) handleInstanceProvidersUpsert(w http.ResponseWriter, r *http.Request) {
	if d.Providers == nil {
		writeProviderError(w, http.StatusServiceUnavailable, "PROVIDERS_UNAVAILABLE", "instance provider store not configured")
		return
	}
	owner := d.instanceOwner(r)
	if owner == nil {
		writeProviderError(w, http.StatusForbidden, "INSTANCE_OWNER_REQUIRED", "instance owner only")
		return
	}
	var in providerUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeProviderError(w, http.StatusBadRequest, "BAD_JSON", "could not parse request body as JSON")
		return
	}
	in.ProviderName = strings.ToLower(strings.TrimSpace(in.ProviderName))
	if in.ProviderName == "" {
		writeProviderError(w, http.StatusBadRequest, "PROVIDER_NAME_REQUIRED", "provider_name is required")
		return
	}
	if !providers.SupportsProvider(in.ProviderName) {
		writeProviderError(w, http.StatusBadRequest, "PROVIDER_UNSUPPORTED", "provider_name is not supported by this build")
		return
	}
	rec, err := d.Providers.Upsert(r.Context(), providers.UpsertInput{
		ProviderName:    in.ProviderName,
		DisplayName:     in.DisplayName,
		ClientID:        in.ClientID,
		ClientSecret:    in.ClientSecret,
		Config:          in.Config,
		Enabled:         in.Enabled,
		CreatedByUserID: owner.UserID,
	})
	if err != nil {
		d.writeProvidersErr(w, err, "PROVIDER_UPSERT_FAILED")
		return
	}
	if err := d.applyProviderToOAuth(rec); err != nil {
		d.writeProvidersErr(w, err, "PROVIDER_REFRESH_FAILED")
		return
	}
	pub := rec.ToPublic()
	writeJSON(w, http.StatusOK, providerOpResponse{Status: "upserted", Provider: &pub})
}

func (d Deps) handleInstanceProvidersDelete(w http.ResponseWriter, r *http.Request) {
	if d.Providers == nil {
		writeProviderError(w, http.StatusServiceUnavailable, "PROVIDERS_UNAVAILABLE", "instance provider store not configured")
		return
	}
	if d.instanceOwner(r) == nil {
		writeProviderError(w, http.StatusForbidden, "INSTANCE_OWNER_REQUIRED", "instance owner only")
		return
	}
	idOrName := strings.TrimSpace(r.PathValue("idOrName"))
	if idOrName == "" {
		writeProviderError(w, http.StatusBadRequest, "PROVIDER_ID_REQUIRED", "id or provider_name is required in path")
		return
	}
	// Capture the provider so we know which OAuth slot to unwire after
	// the row is gone. Treat ErrNotFound as "nothing to unwire".
	rec, err := d.Providers.GetByName(r.Context(), idOrName)
	if errors.Is(err, providers.ErrNotFound) {
		// idOrName might be a uuid that GetByName doesn't accept — fall
		// through to delete; nothing to unwire either way.
		err = nil
	} else if err != nil {
		d.writeProvidersErr(w, err, "PROVIDER_LOOKUP_FAILED")
		return
	}
	if err := d.Providers.Delete(r.Context(), idOrName); err != nil {
		if errors.Is(err, providers.ErrNotFound) {
			writeProviderError(w, http.StatusNotFound, "PROVIDER_NOT_FOUND", "provider not found")
			return
		}
		d.writeProvidersErr(w, err, "PROVIDER_DELETE_FAILED")
		return
	}
	if rec.ProviderName == providers.ProviderGitHub && d.OAuth != nil {
		if err := d.OAuth.SetGitHubCredentials("", ""); err != nil {
			d.writeProvidersErr(w, err, "PROVIDER_REFRESH_FAILED")
			return
		}
	}
	writeJSON(w, http.StatusOK, providerOpResponse{Status: "deleted"})
}

// applyProviderToOAuth pushes the just-saved row into the running
// OAuthService so the next /auth/github/login picks up the new
// credentials without a restart. Disabled rows unwire the IdP.
func (d Deps) applyProviderToOAuth(rec providers.Record) error {
	if d.OAuth == nil {
		return nil
	}
	if rec.ProviderName != providers.ProviderGitHub {
		return nil
	}
	if !rec.Enabled {
		return d.OAuth.SetGitHubCredentials("", "")
	}
	return d.OAuth.SetGitHubCredentials(rec.ClientID, rec.ClientSecret)
}

func (d Deps) writeProvidersErr(w http.ResponseWriter, err error, fallbackCode string) {
	switch {
	case errors.Is(err, providers.ErrInstanceKeyMissing):
		writeProviderError(w, http.StatusServiceUnavailable, "INSTANCE_KEY_MISSING", "PINDOC_INSTANCE_KEY is required to read or write encrypted credentials")
	case errors.Is(err, providers.ErrInstanceKeyInvalid):
		writeProviderError(w, http.StatusServiceUnavailable, "INSTANCE_KEY_INVALID", "PINDOC_INSTANCE_KEY is invalid")
	case errors.Is(err, providers.ErrCiphertextCorrupt):
		writeProviderError(w, http.StatusServiceUnavailable, "INSTANCE_KEY_INVALID", "stored credential cannot be decrypted with the current PINDOC_INSTANCE_KEY")
	case errors.Is(err, providers.ErrUnsupportedProvider):
		writeProviderError(w, http.StatusBadRequest, "PROVIDER_UNSUPPORTED", "provider_name is not supported by this build")
	case errors.Is(err, providers.ErrClientIDRequired):
		writeProviderError(w, http.StatusBadRequest, "CLIENT_ID_REQUIRED", "client_id is required")
	case errors.Is(err, providers.ErrNotFound):
		writeProviderError(w, http.StatusNotFound, "PROVIDER_NOT_FOUND", "provider not found")
	default:
		if d.Logger != nil {
			d.Logger.Error("instance providers handler failed", "err", err, "code", fallbackCode)
		}
		writeProviderError(w, http.StatusInternalServerError, fallbackCode, "internal error")
	}
}
