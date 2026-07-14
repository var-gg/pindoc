package indexstate

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

type failingProvider struct{}

func (failingProvider) Info() embed.Info {
	return embed.Info{Name: "failing-test", ModelID: "offline", Dimension: 8}
}

func (failingProvider) Embed(context.Context, embed.Request) (*embed.Response, error) {
	return nil, errors.New("test provider offline")
}

func TestRefreshIntegrationPreservesChunksAndRecovers(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run artifact index state DB integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("initial Migrate() error = %v", err)
	}

	// Rewind only 0069, seed a pre-existing artifact, and reapply it. This
	// proves the migration labels legacy rows unknown instead of claiming
	// their old chunks are current.
	if _, err := pool.Exec(ctx, `
		DROP VIEW IF EXISTS artifact_index_health;
		DROP TABLE IF EXISTS artifact_index_state;
		DELETE FROM schema_migrations WHERE id = '0069_artifact_index_state.sql';
	`); err != nil {
		t.Fatalf("rewind 0069: %v", err)
	}

	var projectID, areaID, artifactID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (organization_id, slug, name, reader_hidden)
		SELECT id, 'index-state-integration', 'Index state integration', TRUE
		FROM organizations
		WHERE slug = 'default'
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO areas (project_id, slug, name)
		VALUES ($1::uuid, 'data', 'Data')
		ON CONFLICT (project_id, slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`, projectID).Scan(&areaID); err != nil {
		t.Fatalf("seed area: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (
			project_id, area_id, slug, type, title, body_markdown, author_id
		) VALUES ($1::uuid, $2::uuid, 'index-target', 'Analysis', 'Original title', 'Original body', 'integration-test')
		ON CONFLICT (project_id, slug) DO UPDATE SET
			title = EXCLUDED.title,
			body_markdown = EXCLUDED.body_markdown
		RETURNING id::text
	`, projectID, areaID).Scan(&artifactID); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM artifact_revisions WHERE artifact_id = $1::uuid`, artifactID); err != nil {
		t.Fatalf("clear revisions: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, revision_shape
		) VALUES ($1::uuid, 1, 'Original title', 'Original body', $2,
			'{}', 'partial', 'agent', 'integration-test', 'body_patch')
	`, artifactID, HashText("Original body")); err != nil {
		t.Fatalf("seed revision: %v", err)
	}
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("reapply 0069: %v", err)
	}

	var backfill State
	if err := pool.QueryRow(ctx, `
		SELECT revision_number, body_hash, title_hash, model_name, model_id,
		       model_dim, status, attempt_count, COALESCE(last_error, '')
		FROM artifact_index_state WHERE artifact_id = $1::uuid
	`, artifactID).Scan(
		&backfill.RevisionNumber,
		&backfill.BodyHash,
		&backfill.TitleHash,
		&backfill.ModelName,
		&backfill.ModelID,
		&backfill.ModelDim,
		&backfill.Status,
		&backfill.AttemptCount,
		&backfill.LastError,
	); err != nil {
		t.Fatalf("read backfill state: %v", err)
	}
	if backfill.Status != StatusUnknown || backfill.RevisionNumber != 1 || backfill.BodyHash != HashText("Original body") {
		t.Fatalf("backfill state = %+v, want explicit unknown rev 1", backfill)
	}

	stub := embed.NewStub(8)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin initial refresh: %v", err)
	}
	state, err := Refresh(ctx, tx, stub, RefreshInput{
		ArtifactID:     artifactID,
		RevisionNumber: 1,
		Title:          "Original title",
		Body:           "Original body",
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("initial Refresh() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit initial refresh: %v", err)
	}
	if state.Status != StatusIndexed {
		t.Fatalf("initial state = %+v", state)
	}
	beforeChunks := chunkSnapshot(t, ctx, pool, artifactID)

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin failed refresh: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, revision_shape
		) VALUES ($1::uuid, 2, 'Changed title', 'Changed body', $2,
			'{}', 'partial', 'agent', 'integration-test', 'body_patch')
	`, artifactID, HashText("Changed body")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert changed revision: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE artifacts
		SET title = 'Changed title', body_markdown = 'Changed body', updated_at = now()
		WHERE id = $1::uuid
	`, artifactID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("update artifact: %v", err)
	}
	failedState, refreshErr := Refresh(ctx, tx, failingProvider{}, RefreshInput{
		ArtifactID:     artifactID,
		RevisionNumber: 2,
		Title:          "Changed title",
		Body:           "Changed body",
	})
	if !IsRetryable(refreshErr) {
		_ = tx.Rollback(ctx)
		t.Fatalf("failed Refresh() error = %v, want retryable", refreshErr)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit failed state: %v", err)
	}
	if failedState.Status != StatusFailed || !failedState.Retryable {
		t.Fatalf("failed state = %+v", failedState)
	}
	afterFailureChunks := chunkSnapshot(t, ctx, pool, artifactID)
	if afterFailureChunks != beforeChunks {
		t.Fatalf("chunks changed after provider failure\nbefore: %s\nafter:  %s", beforeChunks, afterFailureChunks)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin recovery refresh: %v", err)
	}
	recovered, err := Refresh(ctx, tx, stub, RefreshInput{
		ArtifactID:     artifactID,
		RevisionNumber: 2,
		Title:          "Changed title",
		Body:           "Changed body",
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("recovery Refresh() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit recovery: %v", err)
	}
	if recovered.Status != StatusIndexed || recovered.RevisionNumber != 2 || recovered.AttemptCount != 3 {
		t.Fatalf("recovered state = %+v", recovered)
	}
	afterRecoveryChunks := chunkSnapshot(t, ctx, pool, artifactID)
	if afterRecoveryChunks == beforeChunks {
		t.Fatalf("recovery did not replace chunks: %s", afterRecoveryChunks)
	}
}

func chunkSnapshot(t *testing.T, ctx context.Context, pool *db.Pool, artifactID string) string {
	t.Helper()
	var snapshot string
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(string_agg(kind || ':' || chunk_index::text || ':' || text, '|' ORDER BY kind, chunk_index), '')
		FROM artifact_chunks
		WHERE artifact_id = $1::uuid
	`, artifactID).Scan(&snapshot); err != nil {
		t.Fatalf("chunk snapshot: %v", err)
	}
	return snapshot
}
