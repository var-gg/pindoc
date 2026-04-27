package providers

import (
	"strings"
	"time"
)

// Provider names this build supports. The wire CSV
// (PINDOC_AUTH_PROVIDERS) and the admin UI both lowercase / dedupe
// before comparing — these constants are the canonical form.
const (
	ProviderGitHub = "github"
)

// SupportedProviders is the allow-list of provider names the admin
// API accepts. Adding a Pindoc-AS-backed IdP requires an entry here
// + matching wiring in OAuthService. Not exported — handler uses
// SupportsProvider() so future feature flags can gate per-build.
var supportedProviders = map[string]struct{}{
	ProviderGitHub: {},
}

// SupportsProvider reports whether `name` is recognised by this
// build. Comparison is case-insensitive after trim.
func SupportsProvider(name string) bool {
	_, ok := supportedProviders[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// Record is the in-memory shape of an `instance_providers` row with
// the credential secret already decrypted. Returned by Store.List /
// Store.Active so handlers and OAuthService can read credentials
// without round-tripping through the cipher each time.
type Record struct {
	ID                string
	ProviderName      string
	DisplayName       string
	ClientID          string
	ClientSecret      string
	Config            map[string]any
	Enabled           bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CreatedByUserID   string
}

// PublicRecord is the response shape for GET /api/instance/providers.
// Hides client_secret so the admin UI can list providers without ever
// surfacing a secret on the wire — operators rotate by writing a new
// secret, not reading the old one.
type PublicRecord struct {
	ID              string         `json:"id"`
	ProviderName    string         `json:"provider_name"`
	DisplayName     string         `json:"display_name"`
	ClientID        string         `json:"client_id"`
	HasClientSecret bool           `json:"has_client_secret"`
	Config          map[string]any `json:"config,omitempty"`
	Enabled         bool           `json:"enabled"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	CreatedByUserID string         `json:"created_by_user_id,omitempty"`
}

// ToPublic projects a Record into its wire-safe form.
func (r Record) ToPublic() PublicRecord {
	return PublicRecord{
		ID:              r.ID,
		ProviderName:    r.ProviderName,
		DisplayName:     r.DisplayName,
		ClientID:        r.ClientID,
		HasClientSecret: r.ClientSecret != "",
		Config:          r.Config,
		Enabled:         r.Enabled,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
		CreatedByUserID: r.CreatedByUserID,
	}
}

// UpsertInput captures the operator-supplied fields a POST /api/
// instance/providers carries. Empty ClientSecret means "leave the
// stored secret alone" — operators rotating credentials always send
// both new id and new secret.
type UpsertInput struct {
	ProviderName    string
	DisplayName     string
	ClientID        string
	ClientSecret    string
	Config          map[string]any
	Enabled         *bool
	CreatedByUserID string
}
