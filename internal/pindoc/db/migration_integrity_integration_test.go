package db

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMigrateBackfillsBlankChecksumAndRejectsDriftIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run migration integrity DB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}

	manifest, err := embeddedMigrationManifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	target := manifest[len(manifest)-1]
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			UPDATE schema_migrations SET checksum = $2 WHERE id = $1
		`, target.ID, target.Checksum)
	})

	if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET checksum = '' WHERE id = $1`, target.ID); err != nil {
		t.Fatalf("blank checksum: %v", err)
	}
	if err := Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("backfill blank checksum: %v", err)
	}
	var checksum string
	if err := pool.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE id = $1`, target.ID).Scan(&checksum); err != nil {
		t.Fatalf("read backfilled checksum: %v", err)
	}
	if checksum != target.Checksum {
		t.Fatalf("backfilled checksum = %q, want %q", checksum, target.Checksum)
	}

	if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET checksum = 'corrupt' WHERE id = $1`, target.ID); err != nil {
		t.Fatalf("corrupt checksum: %v", err)
	}
	err = Migrate(ctx, pool.Pool)
	if err == nil || !strings.Contains(err.Error(), MigrationChecksumMismatchCode) {
		t.Fatalf("Migrate() drift error = %v, want %s", err, MigrationChecksumMismatchCode)
	}
}
