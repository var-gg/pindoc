package db

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestAuditUserFKsSetNullIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run audit user FK DB integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var inviterID, memberID, projectID, areaID, artifactID, targetArtifactID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ('Audit Inviter', 'audit-inviter@example.invalid', 'pindoc_admin')
		RETURNING id::text
	`).Scan(&inviterID); err != nil {
		t.Fatalf("insert inviter: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ('Audit Member', 'audit-member@example.invalid', 'pindoc_admin')
		RETURNING id::text
	`).Scan(&memberID); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO projects (slug, name, organization_id, primary_language)
		VALUES ('audit-fk-it', 'Audit FK IT', (SELECT id FROM organizations WHERE slug = 'default' LIMIT 1), 'en')
		RETURNING id::text
	`).Scan(&projectID); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO areas (project_id, slug, name)
		VALUES ($1::uuid, 'misc', 'Misc')
		RETURNING id::text
	`, projectID).Scan(&areaID); err != nil {
		t.Fatalf("insert area: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO artifacts (project_id, area_id, slug, type, title, body_markdown, author_id, author_user_id, published_at)
		VALUES ($1::uuid, $2::uuid, 'audit-fk-source', 'Analysis', 'Audit FK Source', 'body', 'agent', $3::uuid, now())
		RETURNING id::text
	`, projectID, areaID, inviterID).Scan(&artifactID); err != nil {
		t.Fatalf("insert source artifact: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO artifacts (project_id, area_id, slug, type, title, body_markdown, author_id, published_at)
		VALUES ($1::uuid, $2::uuid, 'audit-fk-target', 'Analysis', 'Audit FK Target', 'body', 'agent', now())
		RETURNING id::text
	`, projectID, areaID).Scan(&targetArtifactID); err != nil {
		t.Fatalf("insert target artifact: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role, invited_by)
		VALUES ($1::uuid, $2::uuid, 'viewer', $3::uuid)
	`, projectID, memberID, inviterID); err != nil {
		t.Fatalf("insert project member: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO mcp_tool_calls (
			started_at, duration_ms, tool_name, agent_id, user_id, project_slug,
			input_bytes, output_bytes, input_chars, output_chars, input_tokens_est, output_tokens_est
		) VALUES (
			now(), 7, 'pindoc.test', 'agent:test', $1::uuid, 'audit-fk-it',
			1, 1, 1, 1, 1, 1
		)
	`, inviterID); err != nil {
		t.Fatalf("insert tool call: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_scope_edges (from_artifact_id, from_item_ref, to_artifact_id, reason, created_by_user_id, created_by_agent)
		VALUES ($1::uuid, 'acceptance[0]', $2::uuid, 'audit test', $3::uuid, 'agent:test')
	`, artifactID, targetArtifactID, inviterID); err != nil {
		t.Fatalf("insert scope edge: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO read_events (artifact_id, user_id, user_key, started_at, ended_at, active_seconds, scroll_max_pct)
		VALUES ($1::uuid, $2::uuid, 'audit-fk-tester', now(), now() + interval '1 minute', 10, 0.5)
	`, artifactID, inviterID); err != nil {
		t.Fatalf("insert read event: %v", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1::uuid`, inviterID); err != nil {
		t.Fatalf("delete inviter: %v", err)
	}

	assertNullFK(t, ctx, tx, "artifacts.author_user_id", `SELECT author_user_id::text FROM artifacts WHERE id = $1::uuid`, artifactID)
	assertNullFK(t, ctx, tx, "project_members.invited_by", `SELECT invited_by::text FROM project_members WHERE project_id = $1::uuid AND user_id = $2::uuid`, projectID, memberID)
	assertNullFK(t, ctx, tx, "mcp_tool_calls.user_id", `SELECT user_id::text FROM mcp_tool_calls WHERE project_slug = 'audit-fk-it'`)
	assertNullFK(t, ctx, tx, "artifact_scope_edges.created_by_user_id", `SELECT created_by_user_id::text FROM artifact_scope_edges WHERE from_artifact_id = $1::uuid`, artifactID)
	assertNullFK(t, ctx, tx, "read_events.user_id", `SELECT user_id::text FROM read_events WHERE artifact_id = $1::uuid`, artifactID)
}

type auditFKQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func assertNullFK(t *testing.T, ctx context.Context, q auditFKQuerier, label, sql string, args ...any) {
	t.Helper()
	var got *string
	if err := q.QueryRow(ctx, sql, args...).Scan(&got); err != nil {
		t.Fatalf("select %s: %v", label, err)
	}
	if got != nil {
		t.Fatalf("%s = %q, want NULL", label, *got)
	}
}
