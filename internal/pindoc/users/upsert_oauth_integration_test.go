package users

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestUpsertOAuthUserIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run users oauth DB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	linkEmail := fmt.Sprintf("Link-%s@Example.Invalid", suffix)
	newEmail := fmt.Sprintf("New-%s@Example.Invalid", suffix)
	defer func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM users
			 WHERE lower(email) IN ($1, $2)
			    OR provider_uid IN ($3, $4)
		`, strings.ToLower(linkEmail), strings.ToLower(newEmail), "gh-link-"+suffix, "gh-new-"+suffix)
	}()

	var existingID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ('Trusted Existing', $1, 'harness_install')
		RETURNING id::text
	`, strings.ToUpper(linkEmail)).Scan(&existingID); err != nil {
		t.Fatalf("insert trusted existing user: %v", err)
	}

	linked, created, err := UpsertOAuthUser(ctx, pool, OAuthUserInput{
		Provider:     ProviderGitHub,
		ProviderUID:  "gh-link-" + suffix,
		Email:        strings.ToLower(linkEmail),
		DisplayName:  "GitHub Linked",
		GithubHandle: "linked-" + suffix,
	})
	if err != nil {
		t.Fatalf("link existing: %v", err)
	}
	if created {
		t.Fatalf("link existing created a new row")
	}
	if linked.ID != existingID {
		t.Fatalf("linked ID = %s, want existing %s", linked.ID, existingID)
	}
	if linked.Source != SourceGitHub || linked.Provider != ProviderGitHub || linked.ProviderUID != "gh-link-"+suffix {
		t.Fatalf("linked oauth fields = %+v", linked)
	}
	if linked.Email != strings.ToLower(linkEmail) {
		t.Fatalf("linked email = %q, want canonical lowercase", linked.Email)
	}

	inserted, created, err := UpsertOAuthUser(ctx, pool, OAuthUserInput{
		Provider:     ProviderGitHub,
		ProviderUID:  "gh-new-" + suffix,
		Email:        newEmail,
		DisplayName:  "GitHub New",
		GithubHandle: "new-" + suffix,
	})
	if err != nil {
		t.Fatalf("insert new: %v", err)
	}
	if !created {
		t.Fatalf("insert new did not report created")
	}
	if inserted.ID == "" || inserted.Email != strings.ToLower(newEmail) || inserted.ProviderUID != "gh-new-"+suffix {
		t.Fatalf("inserted user = %+v", inserted)
	}
}
