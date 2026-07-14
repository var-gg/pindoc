package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/indexstate"
)

type documentToggleProvider struct {
	delegate      embed.Provider
	failDocuments bool
}

func (p *documentToggleProvider) Info() embed.Info { return p.delegate.Info() }

func (p *documentToggleProvider) Embed(ctx context.Context, request embed.Request) (*embed.Response, error) {
	if p.failDocuments && request.Kind == embed.KindDocument {
		return nil, errors.New("document embedding intentionally unavailable")
	}
	return p.delegate.Embed(ctx, request)
}

func TestArtifactProposeIndexStateCreateAndFailedUpdateIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run artifact.propose index state DB integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	suffix := time.Now().UnixNano()
	projectSlug := fmt.Sprintf("index-propose-%d", suffix)
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	areaSlug := "data"
	_ = insertContextReceiptArea(t, ctx, pool, projectID, areaSlug)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	provider := &documentToggleProvider{delegate: embed.NewStub(8)}
	call := newArtifactProposeTestCallerWithEmbedder(t, ctx, pool, nil, provider)
	slug := fmt.Sprintf("index-state-%d", suffix)
	originalBody := validDecisionBodyForPropose("Original context", "Original decision")
	created := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     areaSlug,
		"type":          "Decision",
		"title":         "인덱스 상태 생성 검증",
		"slug":          slug,
		"body_markdown": originalBody,
		"author_id":     "codex-index-test",
	})
	if created.Status != "accepted" || created.IndexState == nil || created.IndexState.Status != indexstate.StatusIndexed {
		t.Fatalf("create output = %+v", created)
	}
	beforeChunks := proposeChunkSnapshot(t, ctx, pool, created.ArtifactID)
	if beforeChunks == "" {
		t.Fatal("create produced no chunks")
	}

	provider.failDocuments = true
	updatedBody := validDecisionBodyForPropose("Changed context", "Changed decision")
	updated := call(ctx, map[string]any{
		"project_slug":     projectSlug,
		"area_slug":        areaSlug,
		"type":             "Decision",
		"title":            "인덱스 상태 갱신 실패 검증",
		"update_of":        slug,
		"expected_version": 1,
		"commit_msg":       "verify retryable index failure",
		"body_markdown":    updatedBody,
		"author_id":        "codex-index-test",
	})
	if updated.Status != "accepted" || updated.RevisionNumber != 2 {
		t.Fatalf("update output = %+v", updated)
	}
	if updated.IndexState == nil || updated.IndexState.Status != indexstate.StatusFailed || !updated.IndexState.Retryable {
		t.Fatalf("update index state = %+v, want retryable failed", updated.IndexState)
	}
	afterChunks := proposeChunkSnapshot(t, ctx, pool, created.ArtifactID)
	if afterChunks != beforeChunks {
		t.Fatalf("failed update replaced chunks\nbefore: %s\nafter:  %s", beforeChunks, afterChunks)
	}

	var storedStatus, storedHash, lastError string
	var storedRevision, attempts int
	if err := pool.QueryRow(ctx, `
		SELECT status, revision_number, body_hash, attempt_count, COALESCE(last_error, '')
		FROM artifact_index_state
		WHERE artifact_id = $1::uuid
	`, created.ArtifactID).Scan(&storedStatus, &storedRevision, &storedHash, &attempts, &lastError); err != nil {
		t.Fatalf("read stored index state: %v", err)
	}
	if storedStatus != indexstate.StatusFailed || storedRevision != 2 || storedHash != indexstate.HashText(updatedBody) || attempts != 2 {
		t.Fatalf("stored state = status:%s rev:%d hash:%s attempts:%d error:%s", storedStatus, storedRevision, storedHash, attempts, lastError)
	}
	if !strings.Contains(lastError, "intentionally unavailable") {
		t.Fatalf("last_error = %q", lastError)
	}
}

func proposeChunkSnapshot(t *testing.T, ctx context.Context, pool *db.Pool, artifactID string) string {
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
