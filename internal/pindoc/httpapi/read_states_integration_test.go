package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/readstate"
)

// TestReadStateAggregationIntegration verifies that the artifact_read_states
// view + readstate package classify a 60s/0.9-scroll session over a 600 ko
// char body as deeply_read with completion ≈ 0.9. Skip when no test DB.
func TestReadStateAggregationIntegration(t *testing.T) {
	pool, cleanupPool := openTestPool(t)
	if pool == nil {
		return
	}
	defer cleanupPool()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	slug := "rs-" + suffix
	projectID := insertReadStateProject(t, ctx, pool, slug)
	areaID := insertReadStateArea(t, ctx, pool, projectID, "rs-area-"+suffix)
	body := strings.Repeat("가", 600) // 60s expected at 600 chars/min
	artifactID := insertReadStateArtifact(t, ctx, pool, projectID, areaID, "rs-art-"+suffix, body, "ko")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO read_events (
			artifact_id, user_id, user_key, started_at, ended_at,
			active_seconds, scroll_max_pct, idle_seconds, locale
		) VALUES ($1::uuid, NULL, 'local',
		          now() - interval '70 seconds', now() - interval '10 seconds',
		          60, 0.9, 0, 'ko')
	`, artifactID); err != nil {
		t.Fatalf("insert read_event: %v", err)
	}

	state, err := readstate.ArtifactState(ctx, pool, slug, artifactID, "local")
	if err != nil {
		t.Fatalf("ArtifactState: %v", err)
	}
	if state.ReadState != readstate.StateDeeplyRead {
		t.Errorf("ReadState = %v, want deeply_read (completion %.3f)", state.ReadState, state.CompletionPct)
	}
	if state.CompletionPct < 0.85 || state.CompletionPct > 0.95 {
		t.Errorf("CompletionPct = %.3f, want ~0.9", state.CompletionPct)
	}
	if state.EventCount != 1 {
		t.Errorf("EventCount = %d, want 1", state.EventCount)
	}

	states, err := readstate.ProjectStates(ctx, pool, slug, "local")
	if err != nil {
		t.Fatalf("ProjectStates: %v", err)
	}
	var got *readstate.State
	for i := range states {
		if states[i].ArtifactID == artifactID {
			got = &states[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("ProjectStates missing our artifact %s", artifactID)
	}
	if got.ReadState != readstate.StateDeeplyRead {
		t.Errorf("ProjectStates ReadState = %v, want deeply_read", got.ReadState)
	}
}

// TestChangeGroupsFallbackImportanceTopIntegration verifies the 3-tier
// fallback contract from docs/06-ui-flows.md Flow 1a. With reader_watermarks
// pegged to "now" and only an ancient (>7d) revision available, the response
// must drop through both watermark-since and 7-day tiers and surface the
// ancient group via fallback_used="importance_top".
func TestChangeGroupsFallbackImportanceTopIntegration(t *testing.T) {
	pool, cleanupPool := openTestPool(t)
	if pool == nil {
		return
	}
	defer cleanupPool()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	slug := "cg-" + suffix
	projectID := insertReadStateProject(t, ctx, pool, slug)
	areaID := insertReadStateArea(t, ctx, pool, projectID, "cg-area-"+suffix)
	artifactID := insertReadStateArtifact(t, ctx, pool, projectID, areaID, "cg-art-"+suffix, "stub body", "en")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = $1`, slug)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, commit_msg, created_at
		) VALUES (
			$1::uuid, 1, 'Ancient', 'old body', 'h1',
			'{}', 'partial', 'agent', 'tester', 'ancient revision', now() - interval '14 days'
		)
	`, artifactID); err != nil {
		t.Fatalf("insert ancient revision: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO reader_watermarks (user_key, project_id, revision_watermark, seen_at)
		VALUES ('local', $1::uuid, 1, now())
	`, projectID); err != nil {
		t.Fatalf("insert watermark: %v", err)
	}

	handler := New(&config.Config{}, Deps{
		DB:     pool,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/p/"+slug+"/change-groups", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp changeGroupsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Baseline.FallbackUsed != "importance_top" {
		t.Errorf("FallbackUsed = %q, want importance_top (groups=%d, fallback=%v, last_seen_at=%v)",
			resp.Baseline.FallbackUsed, len(resp.Groups), resp.Baseline.FallbackUsed, resp.Baseline.LastSeenAt)
	}
	if len(resp.Groups) == 0 {
		t.Error("importance_top fallback returned 0 groups, want >=1")
	}
}

func openTestPool(t *testing.T) (*db.Pool, func()) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run read_states / change_groups DB integration")
		return nil, func() {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		pool.Close()
		t.Fatalf("migrate db: %v", err)
	}
	return pool, func() { pool.Close() }
}

func insertReadStateProject(t *testing.T, ctx context.Context, pool *db.Pool, slug string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (slug, name, primary_language)
		VALUES ($1, $2, 'en')
		RETURNING id::text
	`, slug, "test "+slug).Scan(&id); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return id
}

func insertReadStateArea(t *testing.T, ctx context.Context, pool *db.Pool, projectID, slug string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO areas (project_id, slug, name)
		VALUES ($1::uuid, $2, $2)
		RETURNING id::text
	`, projectID, slug).Scan(&id); err != nil {
		t.Fatalf("insert area: %v", err)
	}
	return id
}

func insertReadStateArtifact(t *testing.T, ctx context.Context, pool *db.Pool, projectID, areaID, slug, body, locale string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (
			project_id, area_id, slug, type, title,
			body_markdown, body_locale, author_id, completeness
		)
		VALUES ($1::uuid, $2::uuid, $3, 'Doc', $3, $4, $5, 'tester', 'partial')
		RETURNING id::text
	`, projectID, areaID, slug, body, locale).Scan(&id); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	return id
}
