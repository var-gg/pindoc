package users

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/organizations"
)

const (
	ProviderGitHub = "github"
	SourceGitHub   = "github_oauth"
)

type OAuthUserInput struct {
	Provider     string
	ProviderUID  string
	Email        string
	DisplayName  string
	GithubHandle string
}

type OAuthUser struct {
	ID           string
	DisplayName  string
	Email        string
	GithubHandle string
	Source       string
	Provider     string
	ProviderUID  string
}

// UpsertOAuthUser links a verified IdP identity to users. Existing
// trusted_local rows are matched by lower(email), then upgraded in place.
func UpsertOAuthUser(ctx context.Context, pool *db.Pool, in OAuthUserInput) (*OAuthUser, bool, error) {
	if pool == nil {
		return nil, false, errors.New("users: nil DB pool")
	}
	clean, err := normalizeOAuthUserInput(in)
	if err != nil {
		return nil, false, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin oauth user upsert: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, found, err := selectOAuthUserForUpdate(ctx, tx, `
		SELECT id::text, display_name, email, github_handle, source, provider, provider_uid
		  FROM users
		 WHERE provider = $1 AND provider_uid = $2 AND deleted_at IS NULL
		 LIMIT 1
		 FOR UPDATE
	`, clean.Provider, clean.ProviderUID)
	if err != nil {
		return nil, false, err
	}
	if !found {
		existing, found, err = selectOAuthUserForUpdate(ctx, tx, `
			SELECT id::text, display_name, email, github_handle, source, provider, provider_uid
			  FROM users
			 WHERE lower(email) = $1 AND deleted_at IS NULL
			 LIMIT 1
			 FOR UPDATE
		`, clean.Email)
		if err != nil {
			return nil, false, err
		}
	}
	if found {
		if existing.Provider != "" && (existing.Provider != clean.Provider || existing.ProviderUID != clean.ProviderUID) {
			return nil, false, fmt.Errorf("users: email %q is already linked to a different oauth identity", clean.Email)
		}
		updated, err := updateOAuthUser(ctx, tx, existing.ID, clean)
		if err != nil {
			return nil, false, err
		}
		if err := ensurePersonalOrgForOAuth(ctx, tx, updated); err != nil {
			return nil, false, fmt.Errorf("ensure personal org: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, fmt.Errorf("commit oauth user update: %w", err)
		}
		return updated, false, nil
	}

	created, err := insertOAuthUser(ctx, tx, clean)
	if err != nil {
		return nil, false, err
	}
	if err := ensurePersonalOrgForOAuth(ctx, tx, created); err != nil {
		return nil, false, fmt.Errorf("ensure personal org: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit oauth user insert: %w", err)
	}
	return created, true, nil
}

// ensurePersonalOrgForOAuth attaches a personal Organization to a freshly
// upserted OAuth user. Best-effort: if the natural username candidate
// (github_handle, lower-cased) is invalid (reserved word, length, char
// set) or already taken by another user, the function records nothing
// rather than failing the whole signup. The caller surface (Reader
// onboarding) can prompt the user to pick a username later, which then
// retries Personal Org creation through a dedicated handler.
//
// Schema constraint guarantee: organizations has a partial unique index
// (kind='personal', owner_user_id) that makes this idempotent — a second
// call for the same user is a no-op via EnsurePersonal's existence
// check.
func ensurePersonalOrgForOAuth(ctx context.Context, tx pgx.Tx, user *OAuthUser) error {
	if user == nil || user.ID == "" {
		return nil
	}
	candidate := strings.ToLower(strings.TrimSpace(user.GithubHandle))
	if candidate == "" {
		// No handle to derive a username from; defer Personal Org until
		// the user picks one explicitly.
		return nil
	}
	if err := organizations.ValidateSlug(candidate); err != nil {
		// Handle is unusable as a slug (reserved or invalid shape). Leave
		// users.username unset; a follow-up onboarding step prompts the
		// user to pick a different handle.
		return nil
	}
	// Try to claim the username. If it's taken by another row the partial
	// unique index returns 23505 — treat that as "needs onboarding" and
	// skip Personal Org creation rather than aborting signup.
	if _, err := tx.Exec(ctx, `
		UPDATE users
		   SET username = $1, updated_at = now()
		 WHERE id = $2::uuid AND username IS NULL
	`, candidate, user.ID); err != nil {
		// Best-effort: log nothing here (no logger in scope), let the
		// onboarding handler reprompt.
		return nil
	}
	if _, err := organizations.EnsurePersonal(ctx, tx, user.ID, candidate, user.DisplayName); err != nil {
		// Same best-effort policy: don't fail the auth flow on Org-create
		// hiccups. The user lands logged in but without a Personal Org;
		// the onboarding flow will retry.
		return nil
	}
	return nil
}

func normalizeOAuthUserInput(in OAuthUserInput) (OAuthUserInput, error) {
	out := OAuthUserInput{
		Provider:     strings.TrimSpace(in.Provider),
		ProviderUID:  strings.TrimSpace(in.ProviderUID),
		Email:        strings.ToLower(strings.TrimSpace(in.Email)),
		DisplayName:  strings.TrimSpace(in.DisplayName),
		GithubHandle: strings.TrimSpace(in.GithubHandle),
	}
	if out.Provider == "" {
		out.Provider = ProviderGitHub
	}
	if out.Provider != ProviderGitHub {
		return OAuthUserInput{}, fmt.Errorf("users: unsupported oauth provider %q", out.Provider)
	}
	if out.ProviderUID == "" {
		return OAuthUserInput{}, errors.New("users: oauth provider_uid is required")
	}
	if !strings.Contains(out.Email, "@") {
		return OAuthUserInput{}, fmt.Errorf("users: verified email is required")
	}
	if out.DisplayName == "" {
		out.DisplayName = out.Email[:strings.Index(out.Email, "@")]
	}
	return out, nil
}

func selectOAuthUserForUpdate(ctx context.Context, tx pgx.Tx, query string, args ...any) (*OAuthUser, bool, error) {
	var out OAuthUser
	var email, handle, provider, providerUID *string
	err := tx.QueryRow(ctx, query, args...).Scan(
		&out.ID, &out.DisplayName, &email, &handle, &out.Source, &provider, &providerUID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("select oauth user: %w", err)
	}
	if email != nil {
		out.Email = *email
	}
	if handle != nil {
		out.GithubHandle = *handle
	}
	if provider != nil {
		out.Provider = *provider
	}
	if providerUID != nil {
		out.ProviderUID = *providerUID
	}
	return &out, true, nil
}

func updateOAuthUser(ctx context.Context, tx pgx.Tx, id string, in OAuthUserInput) (*OAuthUser, error) {
	row := tx.QueryRow(ctx, `
		UPDATE users
		   SET display_name = $1,
		       email = $2,
		       github_handle = NULLIF($3, ''),
		       source = 'github_oauth',
		       provider = $4,
		       provider_uid = $5,
		       updated_at = now()
		 WHERE id = $6::uuid
		 RETURNING id::text, display_name, email, github_handle, source, provider, provider_uid
	`, in.DisplayName, in.Email, in.GithubHandle, in.Provider, in.ProviderUID, id)
	out, err := scanOAuthUser(row)
	if err != nil {
		return nil, fmt.Errorf("update oauth user: %w", err)
	}
	return out, nil
}

func insertOAuthUser(ctx context.Context, tx pgx.Tx, in OAuthUserInput) (*OAuthUser, error) {
	row := tx.QueryRow(ctx, `
		INSERT INTO users (display_name, email, github_handle, source, provider, provider_uid)
		VALUES ($1, $2, NULLIF($3, ''), 'github_oauth', $4, $5)
		RETURNING id::text, display_name, email, github_handle, source, provider, provider_uid
	`, in.DisplayName, in.Email, in.GithubHandle, in.Provider, in.ProviderUID)
	out, err := scanOAuthUser(row)
	if err != nil {
		return nil, fmt.Errorf("insert oauth user: %w", err)
	}
	return out, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanOAuthUser(row scanner) (*OAuthUser, error) {
	var out OAuthUser
	var email, handle, provider, providerUID *string
	if err := row.Scan(&out.ID, &out.DisplayName, &email, &handle, &out.Source, &provider, &providerUID); err != nil {
		return nil, err
	}
	if email != nil {
		out.Email = *email
	}
	if handle != nil {
		out.GithubHandle = *handle
	}
	if provider != nil {
		out.Provider = *provider
	}
	if providerUID != nil {
		out.ProviderUID = *providerUID
	}
	return &out, nil
}
