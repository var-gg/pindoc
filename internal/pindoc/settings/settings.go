// Package settings owns the operator-editable server config (server_settings
// table). Distinct from internal/pindoc/config which handles infrastructure
// env/file config that demands a restart.
//
// Design contract:
//   - env is a first-boot SEED, not an override. Operators can pass
//     PINDOC_PUBLIC_BASE_URL at first startup; the server writes it into
//     the row if the row is empty. After that, DB is the source of truth
//     and env changes are ignored. This mirrors Ghost / Plausible and
//     avoids the "UI change ignored because env is set" footgun that
//     Mattermost-style env-override tends to produce.
//   - The row always exists (migration 0007 seeds it). Get calls can
//     assume presence.
//   - Updates come via pindoc-admin CLI, the Reader admin pages, or
//     the onboarding identity flow. MCP tools and HTTP read-only
//     consumers don't write.
package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// Values is a snapshot of the current server settings. Immutable once
// returned — callers receive a copy, not a pointer into the store.
type Values struct {
	PublicBaseURL string

	// DefaultLoopbackUserID binds the loopback principal to a real
	// users.id row so artifact attribution and project_members owner
	// rows lock onto a stable identity without env edits. Empty means
	// "no identity bound" — the Reader's onboarding flow surfaces a
	// form when the users table is empty, otherwise main() back-fills
	// the lone non-test row at boot.
	DefaultLoopbackUserID string

	UpdatedAt time.Time
}

// Store caches the current row in memory and refreshes on demand. Safe
// for concurrent reads via atomic pointer swap. Writers go through the
// CLI / onboarding handler so the reload path is infrequent; we accept
// "stale up to next Reload call" in exchange for lock-free reads.
type Store struct {
	db      *db.Pool
	current atomic.Pointer[Values]
}

// New constructs a Store and loads the current row. Returns an error if
// the row doesn't exist (migration 0007 should have created it).
func New(ctx context.Context, pool *db.Pool) (*Store, error) {
	s := &Store{db: pool}
	if err := s.Reload(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// Reload re-reads the settings row from the DB. Called on startup and
// whenever an Update lands.
func (s *Store) Reload(ctx context.Context) error {
	var v Values
	var defaultLoopback *string
	err := s.db.QueryRow(ctx, `
		SELECT public_base_url,
		       COALESCE(default_loopback_user_id::text, '')::text AS default_loopback_user_id,
		       updated_at
		  FROM server_settings WHERE id = 1
	`).Scan(&v.PublicBaseURL, &defaultLoopback, &v.UpdatedAt)
	if err != nil {
		return fmt.Errorf("settings load: %w", err)
	}
	if defaultLoopback != nil {
		v.DefaultLoopbackUserID = strings.TrimSpace(*defaultLoopback)
	}
	s.current.Store(&v)
	return nil
}

// Get returns a copy of the current settings. Thread-safe.
func (s *Store) Get() Values {
	p := s.current.Load()
	if p == nil {
		return Values{}
	}
	return *p
}

// SeedFromEnv is called exactly once at server startup: if the named
// setting is empty in the DB and the env value is non-empty, write it.
// Subsequent boots with different env values are no-ops — the setting
// is now operator-owned.
//
// Returns whether a write happened, for logging.
func (s *Store) SeedFromEnv(ctx context.Context, key, envValue string) (bool, error) {
	envValue = trim(envValue)
	if envValue == "" {
		return false, nil
	}
	v := s.Get()
	switch key {
	case "public_base_url":
		if v.PublicBaseURL != "" {
			return false, nil
		}
		if _, err := s.db.Exec(ctx, `
			UPDATE server_settings SET public_base_url = $1, updated_at = now() WHERE id = 1
		`, envValue); err != nil {
			return false, fmt.Errorf("seed public_base_url: %w", err)
		}
	case "default_loopback_user_id":
		if v.DefaultLoopbackUserID != "" {
			return false, nil
		}
		if _, err := s.db.Exec(ctx, `
			UPDATE server_settings
			   SET default_loopback_user_id = $1::uuid, updated_at = now()
			 WHERE id = 1
		`, envValue); err != nil {
			return false, fmt.Errorf("seed default_loopback_user_id: %w", err)
		}
	default:
		return false, fmt.Errorf("unknown setting key: %s", key)
	}
	return true, s.Reload(ctx)
}

// Set updates a single setting and refreshes the cache. Called by
// pindoc-admin (M1), the Settings UI (V1.5), and the onboarding
// identity handler. Unknown keys return an error so typos can't
// silently no-op.
func (s *Store) Set(ctx context.Context, key, value string) error {
	value = trim(value)
	switch key {
	case "public_base_url":
		if _, err := s.db.Exec(ctx, `
			UPDATE server_settings SET public_base_url = $1, updated_at = now() WHERE id = 1
		`, value); err != nil {
			return err
		}
	case "default_loopback_user_id":
		if value == "" {
			if _, err := s.db.Exec(ctx, `
				UPDATE server_settings
				   SET default_loopback_user_id = NULL, updated_at = now()
				 WHERE id = 1
			`); err != nil {
				return err
			}
		} else {
			if _, err := s.db.Exec(ctx, `
				UPDATE server_settings
				   SET default_loopback_user_id = $1::uuid, updated_at = now()
				 WHERE id = 1
			`, value); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown setting key: %s", key)
	}
	return s.Reload(ctx)
}

// SetDefaultLoopbackUserID is a typed convenience wrapper over Set so
// the onboarding handler (and main()'s backfill heuristic) doesn't
// have to remember the string key.
func (s *Store) SetDefaultLoopbackUserID(ctx context.Context, userID string) error {
	return s.Set(ctx, "default_loopback_user_id", userID)
}

// AllKeys lists the editable keys. pindoc-admin list uses this.
func AllKeys() []string {
	return []string{"public_base_url", "default_loopback_user_id"}
}

// FindBackfillCandidate looks for a single non-test users row to bind
// to default_loopback_user_id when the operator has not set the env
// fallback and the column is empty. The "not test" filter excludes
// example.invalid email rows so OAuth integration test residue does
// not accidentally claim ownership of the operator's box.
//
// Returns ("", nil) when zero or multiple candidates exist — the
// caller leaves settings empty and Reader shows the onboarding form.
func FindBackfillCandidate(ctx context.Context, pool *db.Pool) (string, error) {
	if pool == nil {
		return "", errors.New("settings: nil pool")
	}
	rows, err := pool.Query(ctx, `
		SELECT id::text, email
		  FROM users
		 WHERE deleted_at IS NULL
		   AND lower(COALESCE(email, '')) NOT LIKE '%@example.invalid'
		 LIMIT 2
	`)
	if err != nil {
		return "", fmt.Errorf("settings: backfill candidate scan: %w", err)
	}
	defer rows.Close()
	var (
		first string
		count int
	)
	for rows.Next() {
		count++
		if count == 1 {
			var id, email string
			if err := rows.Scan(&id, &email); err != nil {
				return "", err
			}
			first = id
		} else {
			return "", nil // ambiguous — multiple non-test users
		}
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	return first, nil
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}
