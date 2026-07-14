package db

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every unapplied `.sql` file from migrations/ in
// lexicographic order. It tracks applied migrations in a schema_migrations
// table, replicating goose's on-disk contract without bringing in goose's
// CLI as a runtime dependency (we want a single binary).
//
// Each file is parsed for `-- +goose Up` / `-- +goose Down` markers;
// only the Up block is applied. The Down block is preserved in-source
// for manual rollback via psql.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	manifest, err := embeddedMigrationManifest()
	if err != nil {
		return fmt.Errorf("build migration manifest: %w", err)
	}

	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration lock connection: %w", err)
	}
	defer lockConn.Release()
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock(hashtext('pindoc_schema_migrations'))`); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('pindoc_schema_migrations'))`)
	}()

	if _, err := lockConn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		id           TEXT PRIMARY KEY,
		checksum     TEXT NULL,
		applied_at   TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("init schema_migrations: %w", err)
	}
	if _, err := lockConn.Exec(ctx, `ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT NULL`); err != nil {
		return fmt.Errorf("ensure schema_migrations checksum: %w", err)
	}

	rows, err := lockConn.Query(ctx, `SELECT id, COALESCE(checksum, '') FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	applied := make(map[string]string)
	for rows.Next() {
		var id, checksum string
		if err := rows.Scan(&id, &checksum); err != nil {
			rows.Close()
			return fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[id] = strings.TrimSpace(checksum)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate schema_migrations: %w", err)
	}
	rows.Close()

	for _, migration := range manifest {
		storedChecksum, exists := applied[migration.ID]
		if exists {
			if storedChecksum == "" {
				if _, err := lockConn.Exec(ctx,
					`UPDATE schema_migrations
					    SET checksum = $2
					  WHERE id = $1
					    AND (checksum IS NULL OR btrim(checksum) = '')`,
					migration.ID, migration.Checksum); err != nil {
					return fmt.Errorf("backfill checksum for %s: %w", migration.ID, err)
				}
				continue
			}
			if storedChecksum != migration.Checksum {
				return fmt.Errorf("%s: %s expected %s, ledger has %s",
					MigrationChecksumMismatchCode, migration.ID, migration.Checksum, storedChecksum)
			}
			continue
		}

		raw, err := migrationsFS.ReadFile("migrations/" + migration.ID)
		if err != nil {
			return fmt.Errorf("read %s: %w", migration.ID, err)
		}
		up := extractUp(string(raw))

		tx, err := lockConn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", migration.ID, err)
		}
		if _, err := tx.Exec(ctx, up); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", migration.ID, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (id, checksum) VALUES ($1, $2)`, migration.ID, migration.Checksum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", migration.ID, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", migration.ID, err)
		}
	}
	return nil
}

// extractUp returns the text between `-- +goose Up` and `-- +goose Down`
// (or end of file), trimmed. Lines before the Up marker are discarded.
func extractUp(raw string) string {
	const upMarker = "-- +goose Up"
	const downMarker = "-- +goose Down"
	i := strings.Index(raw, upMarker)
	if i < 0 {
		return ""
	}
	tail := raw[i+len(upMarker):]
	if j := strings.Index(tail, downMarker); j >= 0 {
		tail = tail[:j]
	}
	return strings.TrimSpace(tail)
}
