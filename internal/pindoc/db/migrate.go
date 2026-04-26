package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
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
		applied_at   TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("init schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		var exists bool
		if err := lockConn.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE id = $1)`,
			name).Scan(&exists); err != nil {
			return fmt.Errorf("check %s: %w", name, err)
		}
		if exists {
			continue
		}

		raw, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		up := extractUp(string(raw))
		if up == "" {
			return fmt.Errorf("%s: no +goose Up block found", name)
		}

		tx, err := lockConn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, up); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (id) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
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
