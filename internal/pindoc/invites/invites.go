package invites

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

const (
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

var (
	ErrTokenRequired = errors.New("invites: token is required")
	ErrTokenNotFound = errors.New("invites: token not found")
	ErrTokenInactive = errors.New("invites: token expired or consumed")
	ErrRoleInvalid   = errors.New("invites: role must be editor or viewer")
	ErrExtendInvalid = errors.New("invites: extend_to must be +7d, +30d, or permanent")
)

// ListSummary is the read shape returned to the Reader UI for the
// active-invite panel. Token hash is the public id (the raw token
// remains owner-only knowledge after issue), role/expiry/issued_by
// drive the row UI, IsConsumed lets the UI greyout already-claimed
// tokens if the panel ever surfaces history.
type ListSummary struct {
	TokenHash  string
	Role       string
	IssuedByID string
	ExpiresAt  *time.Time
	CreatedAt  time.Time
	ConsumedAt *time.Time
}

type Record struct {
	TokenHash   string
	ProjectID   string
	ProjectSlug string
	ProjectName string
	Role        string
	IssuedBy    string
	ExpiresAt   *time.Time
	ConsumedAt  *time.Time
	ConsumedBy  string
}

type IssueInput struct {
	ProjectID string
	Role      string
	IssuedBy  string
	ExpiresAt time.Time
	Permanent bool
}

func Issue(ctx context.Context, pool *db.Pool, in IssueInput) (string, *Record, error) {
	if pool == nil {
		return "", nil, errors.New("invites: nil DB pool")
	}
	role := NormalizeRole(in.Role)
	if !ValidRole(role) {
		return "", nil, ErrRoleInvalid
	}
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return "", nil, errors.New("invites: project_id is required")
	}
	var expiresAt any
	if !in.Permanent {
		next := in.ExpiresAt.UTC()
		if next.IsZero() {
			next = time.Now().UTC().Add(30 * 24 * time.Hour)
		}
		expiresAt = next
	}
	rawToken, err := GenerateToken()
	if err != nil {
		return "", nil, err
	}
	hash := HashToken(rawToken)
	var issuedBy any
	if strings.TrimSpace(in.IssuedBy) != "" {
		issuedBy = strings.TrimSpace(in.IssuedBy)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO invite_tokens (token_hash, project_id, role, issued_by, expires_at)
		VALUES ($1, $2::uuid, $3, $4::uuid, $5)
	`, hash, projectID, role, issuedBy, expiresAt); err != nil {
		return "", nil, fmt.Errorf("insert invite token: %w", err)
	}
	rec, err := Lookup(ctx, pool, rawToken, time.Now().UTC())
	if err != nil {
		return "", nil, err
	}
	return rawToken, rec, nil
}

func Lookup(ctx context.Context, pool *db.Pool, rawToken string, now time.Time) (*Record, error) {
	if pool == nil {
		return nil, errors.New("invites: nil DB pool")
	}
	token, err := NormalizeToken(rawToken)
	if err != nil {
		return nil, err
	}
	rec, err := load(ctx, pool, HashToken(token), false)
	if err != nil {
		return nil, err
	}
	if inactive(rec, now) {
		return nil, ErrTokenInactive
	}
	return rec, nil
}

func Consume(ctx context.Context, pool *db.Pool, rawToken, userID string, now time.Time) (*Record, error) {
	if pool == nil {
		return nil, errors.New("invites: nil DB pool")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("invites: user_id is required")
	}
	token, err := NormalizeToken(rawToken)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin invite consume: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rec, err := load(ctx, tx, HashToken(token), true)
	if err != nil {
		return nil, err
	}
	if inactive(rec, now) {
		return nil, ErrTokenInactive
	}
	if _, err := tx.Exec(ctx, `
		UPDATE invite_tokens
		   SET consumed_at = now(),
		       consumed_by = $2::uuid
		 WHERE token_hash = $1
	`, rec.TokenHash, userID); err != nil {
		return nil, fmt.Errorf("mark invite consumed: %w", err)
	}
	var invitedBy any
	if rec.IssuedBy != "" {
		invitedBy = rec.IssuedBy
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role, invited_by)
		VALUES ($1::uuid, $2::uuid, $3, $4::uuid)
		ON CONFLICT (project_id, user_id) DO UPDATE SET
			role = EXCLUDED.role,
			invited_by = EXCLUDED.invited_by
	`, rec.ProjectID, userID, rec.Role, invitedBy); err != nil {
		return nil, fmt.Errorf("upsert project membership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit invite consume: %w", err)
	}
	rec.ConsumedBy = userID
	nowConsumed := now.UTC()
	rec.ConsumedAt = &nowConsumed
	return rec, nil
}

type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func load(ctx context.Context, q queryer, tokenHash string, forUpdate bool) (*Record, error) {
	query := `
		SELECT it.token_hash, it.project_id::text, p.slug, p.name, it.role,
		       it.issued_by::text, it.expires_at, it.consumed_at, it.consumed_by::text
		  FROM invite_tokens it
		  JOIN projects p ON p.id = it.project_id
		 WHERE it.token_hash = $1
	`
	if forUpdate {
		query += ` FOR UPDATE OF it`
	}
	var rec Record
	var issuedBy, consumedBy *string
	var consumedAt *time.Time
	err := q.QueryRow(ctx, query, tokenHash).Scan(
		&rec.TokenHash,
		&rec.ProjectID,
		&rec.ProjectSlug,
		&rec.ProjectName,
		&rec.Role,
		&issuedBy,
		&rec.ExpiresAt,
		&consumedAt,
		&consumedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load invite token: %w", err)
	}
	if issuedBy != nil {
		rec.IssuedBy = *issuedBy
	}
	if consumedAt != nil {
		rec.ConsumedAt = consumedAt
	}
	if consumedBy != nil {
		rec.ConsumedBy = *consumedBy
	}
	return &rec, nil
}

func inactive(rec *Record, now time.Time) bool {
	if rec == nil {
		return true
	}
	if rec.ConsumedAt != nil {
		return true
	}
	if rec.ExpiresAt == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return !now.Before(*rec.ExpiresAt)
}

func GenerateToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate invite token: %w", err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:])
	return "jt_" + strings.ToLower(encoded), nil
}

func NormalizeToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", ErrTokenRequired
	}
	if !strings.HasPrefix(token, "jt_") {
		return "", ErrTokenRequired
	}
	return token, nil
}

func HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawToken)))
	return hex.EncodeToString(sum[:])
}

func NormalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func ValidRole(role string) bool {
	switch NormalizeRole(role) {
	case RoleEditor, RoleViewer:
		return true
	default:
		return false
	}
}

// ListActive returns invite_tokens rows for the project that haven't
// been consumed and haven't expired. Sorted newest issued first so
// the Reader UI shows the most recent invite at the top, matching the
// human flow of "I just sent one — let me revoke it".
func ListActive(ctx context.Context, pool *db.Pool, projectID string, now time.Time) ([]ListSummary, error) {
	if pool == nil {
		return nil, errors.New("invites: nil DB pool")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errors.New("invites: project_id is required")
	}
	rows, err := pool.Query(ctx, `
		SELECT token_hash, role, COALESCE(issued_by::text, ''),
		       expires_at, issued_at, consumed_at
		  FROM invite_tokens
		 WHERE project_id = $1::uuid
		   AND consumed_at IS NULL
		   AND (expires_at IS NULL OR expires_at > $2)
		 ORDER BY issued_at DESC
	`, projectID, now)
	if err != nil {
		return nil, fmt.Errorf("list active invites: %w", err)
	}
	defer rows.Close()
	out := make([]ListSummary, 0, 4)
	for rows.Next() {
		var s ListSummary
		var consumedAt *time.Time
		if err := rows.Scan(&s.TokenHash, &s.Role, &s.IssuedByID, &s.ExpiresAt, &s.CreatedAt, &consumedAt); err != nil {
			return nil, fmt.Errorf("scan invite row: %w", err)
		}
		s.ConsumedAt = consumedAt
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invites: %w", err)
	}
	return out, nil
}

type ExtendTo string

const (
	ExtendPlus7D    ExtendTo = "+7d"
	ExtendPlus30D   ExtendTo = "+30d"
	ExtendPermanent ExtendTo = "permanent"
)

func NormalizeExtendTo(raw string) ExtendTo {
	return ExtendTo(strings.ToLower(strings.TrimSpace(raw)))
}

func ValidExtendTo(raw string) bool {
	switch NormalizeExtendTo(raw) {
	case ExtendPlus7D, ExtendPlus30D, ExtendPermanent:
		return true
	default:
		return false
	}
}

// Extend updates the expiry of an active invite. Relative extensions are
// added to the current future expiry rather than replacing it; permanent
// writes NULL expires_at. Already-permanent invites cannot be shortened
// through +7d/+30d, so those attempts return ErrExtendInvalid.
func Extend(ctx context.Context, pool *db.Pool, projectID, tokenHash, extendTo, actorUserID string, now time.Time) (*Record, error) {
	if pool == nil {
		return nil, errors.New("invites: nil DB pool")
	}
	projectID = strings.TrimSpace(projectID)
	tokenHash = strings.TrimSpace(tokenHash)
	if projectID == "" || tokenHash == "" {
		return nil, errors.New("invites: project_id and token_hash are required")
	}
	mode := NormalizeExtendTo(extendTo)
	if !ValidExtendTo(string(mode)) {
		return nil, ErrExtendInvalid
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin invite extend: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rec, err := load(ctx, tx, tokenHash, true)
	if err != nil {
		return nil, err
	}
	if rec.ProjectID != projectID {
		return nil, ErrTokenNotFound
	}
	if inactive(rec, now) {
		return nil, ErrTokenInactive
	}

	var nextExpiresAt any
	switch mode {
	case ExtendPermanent:
		nextExpiresAt = nil
	case ExtendPlus7D, ExtendPlus30D:
		if rec.ExpiresAt == nil {
			return nil, ErrExtendInvalid
		}
		base := now.UTC()
		if rec.ExpiresAt.After(base) {
			base = rec.ExpiresAt.UTC()
		}
		days := 7
		if mode == ExtendPlus30D {
			days = 30
		}
		next := base.Add(time.Duration(days) * 24 * time.Hour)
		nextExpiresAt = next
	default:
		return nil, ErrExtendInvalid
	}

	if _, err := tx.Exec(ctx, `
		UPDATE invite_tokens
		   SET expires_at = $3
		 WHERE token_hash = $1 AND project_id = $2::uuid
	`, tokenHash, projectID, nextExpiresAt); err != nil {
		return nil, fmt.Errorf("update invite expiry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit invite extend: %w", err)
	}
	return load(ctx, pool, tokenHash, false)
}

// Revoke marks an invite token consumed (consumed_at = now, consumed_by
// = revoker) so the same lookup path that rejects already-consumed
// tokens also rejects revoked ones — no separate revoked column
// needed in the first pass. The caller is the owner who pressed the
// revoke button; we record them as consumed_by so the audit trail
// shows who killed which token.
//
// Returns ErrTokenNotFound when no row matches the hash + project_id
// pair (404 / mistyped hash); ErrTokenInactive when the token was
// already consumed (no-op double-revoke is treated as inactive so
// the UI can collapse an idempotent retry without surfacing an
// error). Project scoping is enforced at the SQL layer so a leaked
// hash from project A cannot be revoked through project B's URL.
func Revoke(ctx context.Context, pool *db.Pool, projectID, tokenHash, revokerUserID string, now time.Time) error {
	if pool == nil {
		return errors.New("invites: nil DB pool")
	}
	projectID = strings.TrimSpace(projectID)
	tokenHash = strings.TrimSpace(tokenHash)
	if projectID == "" || tokenHash == "" {
		return errors.New("invites: project_id and token_hash are required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var revokerArg any
	if v := strings.TrimSpace(revokerUserID); v != "" {
		revokerArg = v
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin invite revoke: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var consumedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT consumed_at FROM invite_tokens
		 WHERE token_hash = $1 AND project_id = $2::uuid
		 FOR UPDATE
	`, tokenHash, projectID).Scan(&consumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTokenNotFound
	}
	if err != nil {
		return fmt.Errorf("lock invite for revoke: %w", err)
	}
	if consumedAt != nil {
		return ErrTokenInactive
	}
	if _, err := tx.Exec(ctx, `
		UPDATE invite_tokens
		   SET consumed_at = $3,
		       consumed_by = $4::uuid
		 WHERE token_hash = $1 AND project_id = $2::uuid
	`, tokenHash, projectID, now, revokerArg); err != nil {
		return fmt.Errorf("mark invite revoked: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit invite revoke: %w", err)
	}
	return nil
}
