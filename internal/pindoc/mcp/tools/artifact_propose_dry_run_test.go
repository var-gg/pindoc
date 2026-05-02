package tools

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/receipts"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
)

func TestArtifactProposeDryRunIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run artifact.propose dry_run DB integration")
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

	suffix := time.Now().UnixNano()
	projectSlug := fmt.Sprintf("dry-run-%d", suffix)
	areaSlug := "mcp"
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	areaID := insertContextReceiptArea(t, ctx, pool, projectID, areaSlug)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()
	evidenceID := insertDryRunArtifact(t, ctx, pool, projectID, areaID, "evidence", "Decision", "Evidence", "## Context\nx\n## Decision\ny\n## Rationale\nz\n## Alternatives considered\na\n## Consequences\nb\n")

	call := newArtifactProposeTestCaller(t, ctx, pool, nil)
	beforeArtifacts := countRows(t, ctx, pool, "artifacts", projectID)
	beforeEdges := countRows(t, ctx, pool, "artifact_edges", projectID)
	createSlug := fmt.Sprintf("dry-run-create-%d", suffix)
	createBody := validDecisionBodyForPropose("x", "y")
	out := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     areaSlug,
		"type":          "Decision",
		"title":         "Dry run create",
		"slug":          createSlug,
		"body_markdown": createBody,
		"author_id":     "codex-test",
		"dry_run":       true,
		"relates_to": []map[string]any{
			{"target_id": evidenceID, "relation": "evidence"},
		},
	})
	if out.Status != "accepted" || !out.DryRun || out.Created || out.ArtifactID != "" || out.RevisionNumber != 0 {
		t.Fatalf("dry_run create output = status=%q dry=%v created=%v artifact_id=%q rev=%d", out.Status, out.DryRun, out.Created, out.ArtifactID, out.RevisionNumber)
	}
	if got := countRows(t, ctx, pool, "artifacts", projectID); got != beforeArtifacts {
		t.Fatalf("artifact count after dry_run create = %d, want %d", got, beforeArtifacts)
	}
	if got := countRows(t, ctx, pool, "artifact_edges", projectID); got != beforeEdges {
		t.Fatalf("edge count after dry_run create = %d, want %d", got, beforeEdges)
	}

	persistedSlug := fmt.Sprintf("persisted-%d", suffix)
	persistedBody := validDecisionBodyForPropose("x", "y")
	persisted := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     areaSlug,
		"type":          "Decision",
		"title":         "Persisted decision",
		"slug":          persistedSlug,
		"body_markdown": persistedBody,
		"author_id":     "codex-test",
	})
	if persisted.Status != "accepted" || persisted.DryRun || !persisted.Created || persisted.RevisionNumber != 1 {
		t.Fatalf("persisted create output = status=%q dry=%v created=%v rev=%d", persisted.Status, persisted.DryRun, persisted.Created, persisted.RevisionNumber)
	}

	updateOut := call(ctx, map[string]any{
		"project_slug":     projectSlug,
		"area_slug":        areaSlug,
		"type":             "Decision",
		"title":            "Persisted decision update",
		"update_of":        persistedSlug,
		"expected_version": 1,
		"commit_msg":       "dry run update",
		"body_markdown":    validDecisionBodyForPropose("x2", "y"),
		"author_id":        "codex-test",
		"dry_run":          true,
	})
	if updateOut.Status != "accepted" || !updateOut.DryRun || updateOut.Created || updateOut.RevisionNumber != 0 {
		t.Fatalf("dry_run update output = status=%q dry=%v created=%v rev=%d", updateOut.Status, updateOut.DryRun, updateOut.Created, updateOut.RevisionNumber)
	}
	assertArtifactHead(t, ctx, pool, persisted.ArtifactID, 1, persistedBody)

	supersedeOut := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     areaSlug,
		"type":          "Decision",
		"title":         "Replacement decision",
		"slug":          fmt.Sprintf("replacement-%d", suffix),
		"supersede_of":  persistedSlug,
		"body_markdown": validDecisionBodyForPropose("x", "y2"),
		"author_id":     "codex-test",
		"dry_run":       true,
	})
	if supersedeOut.Status != "accepted" || !supersedeOut.DryRun || supersedeOut.Superseded || supersedeOut.ArtifactID != "" {
		t.Fatalf("dry_run supersede output = status=%q dry=%v superseded=%v artifact_id=%q", supersedeOut.Status, supersedeOut.DryRun, supersedeOut.Superseded, supersedeOut.ArtifactID)
	}
	assertArtifactStatus(t, ctx, pool, persisted.ArtifactID, "published")
}

func TestArtifactProposeDryRunDoesNotBypassReceipt(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run artifact.propose dry_run receipt integration")
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
	projectSlug := fmt.Sprintf("dry-run-receipt-%d", suffix)
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	insertContextReceiptArea(t, ctx, pool, projectID, "mcp")
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	call := newArtifactProposeTestCaller(t, ctx, pool, receipts.New(time.Hour))
	out := call(ctx, map[string]any{
		"project_slug":  projectSlug,
		"area_slug":     "mcp",
		"type":          "Decision",
		"title":         "Receipt gated dry run",
		"slug":          fmt.Sprintf("receipt-gated-%d", suffix),
		"body_markdown": validDecisionBodyForPropose("x", "y"),
		"author_id":     "codex-test",
		"dry_run":       true,
	})
	if out.Status != "not_ready" || out.ErrorCode != "NO_SRCH" {
		t.Fatalf("dry_run without receipt = status=%q code=%q, want not_ready NO_SRCH", out.Status, out.ErrorCode)
	}
}

func newArtifactProposeTestCaller(t *testing.T, ctx context.Context, pool *db.Pool, receiptStore *receipts.Store) func(context.Context, map[string]any) artifactProposeOutput {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	settingsStore, err := settings.New(ctx, pool)
	if err != nil {
		t.Fatalf("settings store: %v", err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-test", Version: "test"}, nil)
	RegisterArtifactPropose(server, Deps{
		DB:        pool,
		Logger:    logger,
		AuthChain: auth.NewChain(auth.NewTrustedLocalResolver("", "agent:dry-run-test")),
		Receipts:  receiptStore,
		Settings:  settingsStore,
	})
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "artifact-propose-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})
	return func(callCtx context.Context, args map[string]any) artifactProposeOutput {
		t.Helper()
		res, err := clientSession.CallTool(callCtx, &sdk.CallToolParams{
			Name:      "pindoc.artifact.propose",
			Arguments: args,
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("CallTool result error: %s", toolResultText(res))
		}
		var out artifactProposeOutput
		if err := decodeStructuredContent(res.StructuredContent, &out); err != nil {
			t.Fatalf("decode structured content: %v", err)
		}
		return out
	}
}

func validDecisionBodyForPropose(contextText, decisionText string) string {
	return "## TL;DR\n\nFixture decision for artifact.propose integration.\n\n" +
		"## Context\n\n" + contextText + "\n\n" +
		"## Decision\n\n" + decisionText + "\n\n" +
		"## Rationale\n\nFixture rationale.\n\n" +
		"## Alternatives considered\n\nAlternative path recorded for validation.\n\n" +
		"## Consequences\n\nFixture consequences.\n"
}

func insertDryRunArtifact(t *testing.T, ctx context.Context, pool *db.Pool, projectID, areaID, slug, typ, title, body string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (
			project_id, area_id, slug, type, title,
			body_markdown, body_locale, author_id, completeness
		)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, 'en', 'tester', 'partial')
		RETURNING id::text
	`, projectID, areaID, slug, typ, title, body).Scan(&id); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash, tags,
			completeness, author_kind, author_id, commit_msg, revision_shape
		)
		VALUES (
			$1::uuid, 1, $2, $3,
			encode(sha256(convert_to($3, 'UTF8')), 'hex'),
			'{}'::text[], 'partial', 'agent', 'tester', 'seed artifact', 'body_patch'
		)
	`, id, title, body); err != nil {
		t.Fatalf("insert revision: %v", err)
	}
	return id
}

func countRows(t *testing.T, ctx context.Context, pool *db.Pool, table, projectID string) int {
	t.Helper()
	var n int
	var query string
	switch table {
	case "artifacts":
		query = `SELECT count(*) FROM artifacts WHERE project_id = $1::uuid`
	case "artifact_edges":
		query = `
			SELECT count(*)
			FROM artifact_edges e
			JOIN artifacts a ON a.id = e.source_id
			WHERE a.project_id = $1::uuid
		`
	default:
		t.Fatalf("unsupported count table %q", table)
	}
	if err := pool.QueryRow(ctx, query, projectID).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func assertArtifactHead(t *testing.T, ctx context.Context, pool *db.Pool, artifactID string, wantRev int, wantBody string) {
	t.Helper()
	var gotRev int
	var gotBody string
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0),
		       a.body_markdown
		FROM artifacts a
		WHERE a.id = $1::uuid
	`, artifactID).Scan(&gotRev, &gotBody); err != nil {
		t.Fatalf("artifact head: %v", err)
	}
	if gotRev != wantRev || gotBody != wantBody {
		t.Fatalf("artifact head = rev %d body %q, want rev %d body %q", gotRev, gotBody, wantRev, wantBody)
	}
}

func assertArtifactStatus(t *testing.T, ctx context.Context, pool *db.Pool, artifactID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM artifacts WHERE id = $1::uuid`, artifactID).Scan(&got); err != nil {
		t.Fatalf("artifact status: %v", err)
	}
	if got != want {
		t.Fatalf("artifact status = %q, want %q", got, want)
	}
}
