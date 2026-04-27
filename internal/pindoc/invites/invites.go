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
)

type Record struct {
	TokenHash   string
	ProjectID   string
	ProjectSlug string
	ProjectName string
	Role        string
	IssuedBy    string
	ExpiresAt   time.Time
	ConsumedAt  *time.Time
	ConsumedBy  string
}

type IssueInput struct {
	ProjectID string
	Role      string
	IssuedBy  string
	ExpiresAt time.Time
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
	expiresAt := in.ExpiresAt.UTC()
	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().Add(24 * time.Hour)
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
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return !now.Before(rec.ExpiresAt)
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
