package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// ErrNotFound is returned when a provider lookup misses. Handlers map
// this to PROVIDER_NOT_FOUND.
var ErrNotFound = errors.New("providers: not found")

// ErrUnsupportedProvider is returned by Upsert when the operator
// passes a provider_name this build does not know how to wire. Keeps
// the admin API from accepting credentials it can never use.
var ErrUnsupportedProvider = errors.New("providers: unsupported provider name")

// ErrClientIDRequired covers the missing-client-id path on Upsert —
// every IdP this build supports needs a client id.
var ErrClientIDRequired = errors.New("providers: client_id is required")

// Store reads and writes the `instance_providers` table. Holds the
// AES-GCM cipher used for credential secrets — Get/List automatically
// decrypt before returning Records. Multi-row safe via the underlying
// pgx pool; no in-memory caching here because mutations need to be
// visible to the next request even from a separate process.
type Store struct {
	db     *db.Pool
	cipher *Cipher
}

// New constructs a Store. cipher may be unconfigured (Cipher.
// Configured() == false) — the daemon then refuses any operation
// touching credential_secret_encrypted.
func New(pool *db.Pool, cipher *Cipher) *Store {
	return &Store{db: pool, cipher: cipher}
}

// EnsureKeyAvailable returns ErrInstanceKeyMissing when the store has
// at least one encrypted credential row but no master key. Called at
// boot so misconfigured deployments fail before any request lands.
func (s *Store) EnsureKeyAvailable(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	if s.cipher.Configured() {
		return nil
	}
	var hasEncrypted bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM instance_providers WHERE octet_length(credential_secret_encrypted) > 0)`,
	).Scan(&hasEncrypted)
	if err != nil {
		return fmt.Errorf("providers: check encrypted rows: %w", err)
	}
	if hasEncrypted {
		return ErrInstanceKeyMissing
	}
	return nil
}

// List returns every row, decrypted. Includes disabled providers so
// the admin UI can show "off" rows alongside active ones.
func (s *Store) List(ctx context.Context) ([]Record, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, provider_name, display_name, client_id,
		       credential_secret_encrypted, config_json, enabled,
		       created_at, updated_at,
		       COALESCE(created_by_user_id::text, '')
		  FROM instance_providers
		 ORDER BY provider_name
	`)
	if err != nil {
		return nil, fmt.Errorf("providers: list: %w", err)
	}
	defer rows.Close()

	out := []Record{}
	for rows.Next() {
		rec, err := s.scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("providers: list iter: %w", err)
	}
	return out, nil
}

// Active is a convenience that filters List to enabled rows.
// OAuthService consumes this on boot (and on hot-reload after Upsert)
// to decide which IdPs to register with fosite.
func (s *Store) Active(ctx context.Context) ([]Record, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(all))
	for _, r := range all {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}

// GetByName resolves a single row by provider_name (case-insensitive
// after trim).
func (s *Store) GetByName(ctx context.Context, name string) (Record, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return Record{}, ErrNotFound
	}
	row := s.db.QueryRow(ctx, `
		SELECT id::text, provider_name, display_name, client_id,
		       credential_secret_encrypted, config_json, enabled,
		       created_at, updated_at,
		       COALESCE(created_by_user_id::text, '')
		  FROM instance_providers
		 WHERE provider_name = $1
		 LIMIT 1
	`, name)
	rec, err := s.scanRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return rec, err
}

// Upsert inserts a new provider row or updates an existing one in
// place. Empty ClientSecret on update preserves the stored ciphertext
// — operators rotate by sending the new secret explicitly. Returns
// the full Record after the write.
func (s *Store) Upsert(ctx context.Context, in UpsertInput) (Record, error) {
	provider := strings.ToLower(strings.TrimSpace(in.ProviderName))
	if !SupportsProvider(provider) {
		return Record{}, fmt.Errorf("%w: %q", ErrUnsupportedProvider, in.ProviderName)
	}
	clientID := strings.TrimSpace(in.ClientID)
	if clientID == "" {
		return Record{}, ErrClientIDRequired
	}
	display := strings.TrimSpace(in.DisplayName)
	if display == "" {
		display = canonicalDisplayName(provider)
	}
	configJSON, err := json.Marshal(orEmptyMap(in.Config))
	if err != nil {
		return Record{}, fmt.Errorf("providers: marshal config: %w", err)
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	// Two paths so we keep the existing ciphertext when secret is empty.
	if in.ClientSecret == "" {
		var createdByPtr any
		if strings.TrimSpace(in.CreatedByUserID) != "" {
			createdByPtr = in.CreatedByUserID
		}
		row := s.db.QueryRow(ctx, `
			INSERT INTO instance_providers (
				provider_name, display_name, client_id,
				credential_secret_encrypted, config_json, enabled, created_by_user_id
			) VALUES ($1, $2, $3, ''::bytea, $4, $5, $6::uuid)
			ON CONFLICT (provider_name) DO UPDATE SET
				display_name = EXCLUDED.display_name,
				client_id    = EXCLUDED.client_id,
				config_json  = EXCLUDED.config_json,
				enabled      = EXCLUDED.enabled,
				updated_at   = now()
			RETURNING id::text, provider_name, display_name, client_id,
			          credential_secret_encrypted, config_json, enabled,
			          created_at, updated_at,
			          COALESCE(created_by_user_id::text, '')
		`, provider, display, clientID, configJSON, enabled, createdByPtr)
		return s.scanRecord(row)
	}

	cipherText, err := s.cipher.Encrypt([]byte(in.ClientSecret))
	if err != nil {
		return Record{}, err
	}
	var createdByPtr any
	if strings.TrimSpace(in.CreatedByUserID) != "" {
		createdByPtr = in.CreatedByUserID
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO instance_providers (
			provider_name, display_name, client_id,
			credential_secret_encrypted, config_json, enabled, created_by_user_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7::uuid)
		ON CONFLICT (provider_name) DO UPDATE SET
			display_name                  = EXCLUDED.display_name,
			client_id                     = EXCLUDED.client_id,
			credential_secret_encrypted   = EXCLUDED.credential_secret_encrypted,
			config_json                   = EXCLUDED.config_json,
			enabled                       = EXCLUDED.enabled,
			updated_at                    = now()
		RETURNING id::text, provider_name, display_name, client_id,
		          credential_secret_encrypted, config_json, enabled,
		          created_at, updated_at,
		          COALESCE(created_by_user_id::text, '')
	`, provider, display, clientID, cipherText, configJSON, enabled, createdByPtr)
	return s.scanRecord(row)
}

// Delete removes one provider row by id (uuid) or name. Returns
// ErrNotFound when no row matches.
func (s *Store) Delete(ctx context.Context, idOrName string) error {
	value := strings.TrimSpace(idOrName)
	if value == "" {
		return ErrNotFound
	}
	tag, err := s.db.Exec(ctx, `
		DELETE FROM instance_providers
		 WHERE id::text = $1 OR provider_name = lower($1)
	`, value)
	if err != nil {
		return fmt.Errorf("providers: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) scanRecord(row pgx.Row) (Record, error) {
	var (
		rec        Record
		secretEnc  []byte
		configRaw  []byte
		createdAt  time.Time
		updatedAt  time.Time
		createdBy  string
	)
	if err := row.Scan(
		&rec.ID, &rec.ProviderName, &rec.DisplayName, &rec.ClientID,
		&secretEnc, &configRaw, &rec.Enabled,
		&createdAt, &updatedAt,
		&createdBy,
	); err != nil {
		return Record{}, err
	}
	rec.CreatedAt = createdAt
	rec.UpdatedAt = updatedAt
	if createdBy != "" {
		rec.CreatedByUserID = createdBy
	}
	if len(secretEnc) > 0 {
		plain, err := s.cipher.Decrypt(secretEnc)
		if err != nil {
			return Record{}, err
		}
		rec.ClientSecret = string(plain)
	}
	if len(configRaw) > 0 {
		var cfg map[string]any
		if err := json.Unmarshal(configRaw, &cfg); err != nil {
			return Record{}, fmt.Errorf("providers: decode config: %w", err)
		}
		rec.Config = cfg
	}
	return rec, nil
}

func canonicalDisplayName(provider string) string {
	switch provider {
	case ProviderGitHub:
		return "GitHub"
	default:
		return strings.Title(provider) //nolint:staticcheck — V1 admin only ASCII names
	}
}

func orEmptyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}
