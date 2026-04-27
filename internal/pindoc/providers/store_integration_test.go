package providers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// TestProviderStoreRoundTrip exercises the encrypt-on-write / decrypt-
// on-read path against a real Postgres so we catch encoding /
// migration mismatches up front. Skipped unless
// PINDOC_TEST_DATABASE_URL points at a writeable instance.
func TestProviderStoreRoundTrip(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run providers DB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	cipher, err := NewCipherFromBase64(base64.StdEncoding.EncodeToString(integrationFreshKey(t)))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	store := New(pool, cipher)

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	providerName := "github"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM instance_providers WHERE provider_name = $1`, providerName)
	})

	// Insert with a secret.
	rec, err := store.Upsert(ctx, UpsertInput{
		ProviderName: providerName,
		ClientID:     "client-" + suffix,
		ClientSecret: "secret-" + suffix,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if rec.ClientSecret != "secret-"+suffix {
		t.Fatalf("ClientSecret = %q, want round-trip", rec.ClientSecret)
	}

	// Update without a secret keeps the existing ciphertext.
	rec2, err := store.Upsert(ctx, UpsertInput{
		ProviderName: providerName,
		ClientID:     "client2-" + suffix,
	})
	if err != nil {
		t.Fatalf("update without secret: %v", err)
	}
	if rec2.ClientID != "client2-"+suffix {
		t.Fatalf("client_id was not updated: %q", rec2.ClientID)
	}
	if rec2.ClientSecret != "secret-"+suffix {
		t.Fatalf("ClientSecret should be preserved on rotation; got %q", rec2.ClientSecret)
	}

	// EnsureKeyAvailable should pass because we have the key.
	if err := store.EnsureKeyAvailable(ctx); err != nil {
		t.Fatalf("EnsureKeyAvailable with key: %v", err)
	}

	// Switch to an unconfigured cipher; EnsureKeyAvailable must complain
	// because an encrypted row is present.
	storeNoKey := New(pool, &Cipher{})
	if err := storeNoKey.EnsureKeyAvailable(ctx); !errors.Is(err, ErrInstanceKeyMissing) {
		t.Fatalf("EnsureKeyAvailable without key err = %v, want ErrInstanceKeyMissing", err)
	}

	// Active() should also error because List wraps Decrypt.
	if _, err := storeNoKey.Active(ctx); !errors.Is(err, ErrInstanceKeyMissing) {
		t.Fatalf("Active without key err = %v, want ErrInstanceKeyMissing", err)
	}

	// Delete by name.
	if err := store.Delete(ctx, providerName); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.GetByName(ctx, providerName); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after Delete err = %v, want ErrNotFound", err)
	}

	// Upsert refusing unsupported provider.
	if _, err := store.Upsert(ctx, UpsertInput{ProviderName: "unknown-idp", ClientID: "x"}); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("unsupported provider err = %v, want ErrUnsupportedProvider", err)
	}

	// Upsert refusing missing client id.
	if _, err := store.Upsert(ctx, UpsertInput{ProviderName: providerName}); !errors.Is(err, ErrClientIDRequired) {
		t.Fatalf("missing client id err = %v, want ErrClientIDRequired", err)
	}
}

func integrationFreshKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, instanceKeyBytes)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return key
}
