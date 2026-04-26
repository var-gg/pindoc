package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestUserEmailCanonicalIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run users.email DB integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	suffix := time.Now().UnixNano()
	email := fmt.Sprintf("Mixed-%d@Example.Invalid", suffix)
	lowerEmail := canonicalUserEmail(email)
	dupEmail := fmt.Sprintf("Dup-%d@Example.Invalid", suffix)
	lowerDup := canonicalUserEmail(dupEmail)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lower(email) IN ($1, $2)`, lowerEmail, lowerDup)
	}()

	deps := Deps{DB: pool}
	userID, err := UpsertUserFromEnv(ctx, deps, "Case User", email)
	if err != nil {
		t.Fatalf("upsert mixed-case email: %v", err)
	}
	var storedEmail string
	if err := pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1::uuid`, userID).Scan(&storedEmail); err != nil {
		t.Fatalf("select stored email: %v", err)
	}
	if storedEmail != lowerEmail {
		t.Fatalf("stored email = %q, want %q", storedEmail, lowerEmail)
	}

	userID2, err := UpsertUserFromEnv(ctx, deps, "Case User Renamed", strings.ToUpper(lowerEmail))
	if err != nil {
		t.Fatalf("upsert same email with different case: %v", err)
	}
	if userID2 != userID {
		t.Fatalf("case-insensitive upsert returned different id: %s vs %s", userID2, userID)
	}

	var dupID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ('Dup Upper', $1, 'pindoc_admin')
		RETURNING id::text
	`, strings.ToUpper(lowerDup)).Scan(&dupID); err != nil {
		t.Fatalf("insert uppercase duplicate seed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ('Dup Lower', $1, 'pindoc_admin')
	`, lowerDup); err == nil {
		t.Fatalf("active lower(email) duplicate should be rejected")
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET deleted_at = now() WHERE id = $1::uuid`, dupID); err != nil {
		t.Fatalf("soft delete uppercase duplicate seed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ('Dup Lower After Delete', $1, 'pindoc_admin')
	`, lowerDup); err != nil {
		t.Fatalf("soft-deleted email should be reusable: %v", err)
	}
}
