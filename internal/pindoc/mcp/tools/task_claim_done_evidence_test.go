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
)

func TestTaskClaimDoneEvidenceArtifactsIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run task.claim_done evidence DB integration")
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
	projectSlug := fmt.Sprintf("claim-evidence-%d", suffix)
	areaSlug := fmt.Sprintf("mcp-%d", suffix)
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	areaID := insertContextReceiptArea(t, ctx, pool, projectID, areaSlug)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	taskSlug := fmt.Sprintf("task-evidence-%d", suffix)
	taskID := insertContextReceiptTask(t, ctx, pool, projectID, areaID, taskSlug)
	evidenceSlugA := fmt.Sprintf("evidence-a-%d", suffix)
	evidenceSlugB := fmt.Sprintf("evidence-b-%d", suffix)
	insertDryRunArtifact(t, ctx, pool, projectID, areaID, evidenceSlugA, "Decision", "Evidence A", "## Context\na\n## Decision\nb\n## Rationale\nc\n## Alternatives considered\nd\n## Consequences\ne\n")
	insertDryRunArtifact(t, ctx, pool, projectID, areaID, evidenceSlugB, "Analysis", "Evidence B", "## Context\na\n## Findings\nb\n")

	scope := &auth.ProjectScope{
		ProjectID:     projectID,
		ProjectSlug:   projectSlug,
		ProjectLocale: "en",
		Role:          "owner",
	}
	principal := &auth.Principal{AgentID: "agent:evidence-test", Source: auth.SourceLoopback}
	out, err := claimOneTaskDone(ctx, Deps{DB: pool}, principal, scope, taskClaimDoneInput{
		ProjectSlug:       projectSlug,
		SlugOrID:          taskSlug,
		Reason:            "claim with decision evidence",
		PinStrategy:       claimDonePinStrategyExplicit,
		Pins:              []ArtifactPinInput{{Kind: "url", Path: "https://example.invalid/evidence"}},
		EvidenceArtifacts: []string{evidenceSlugA, "pindoc://" + evidenceSlugB, evidenceSlugA, ""},
	})
	if err != nil {
		t.Fatalf("claimOneTaskDone: %v", err)
	}
	if out.Status != "accepted" || out.EvidenceEdgesStored != 2 || out.PinsStored != 1 || out.PinsExplicitCount != 1 {
		t.Fatalf("claim_done output = status=%q evidence=%d pins=%d explicit=%d", out.Status, out.EvidenceEdgesStored, out.PinsStored, out.PinsExplicitCount)
	}
	assertEvidenceEdges(t, ctx, pool, taskID, evidenceSlugA, evidenceSlugB)

	readOut := callArtifactReadForEvidenceTest(t, ctx, pool, projectSlug, taskSlug)
	if !edgeRefsContainEvidence(readOut.RelatesTo, evidenceSlugA, evidenceSlugB) {
		t.Fatalf("artifact.read continuation relates_to = %+v, want evidence edges to %q and %q", readOut.RelatesTo, evidenceSlugA, evidenceSlugB)
	}

	badTaskSlug := fmt.Sprintf("task-bad-evidence-%d", suffix)
	insertContextReceiptTask(t, ctx, pool, projectID, areaID, badTaskSlug)
	bad, err := claimOneTaskDone(ctx, Deps{DB: pool}, principal, scope, taskClaimDoneInput{
		ProjectSlug:       projectSlug,
		SlugOrID:          badTaskSlug,
		Reason:            "claim with missing evidence",
		EvidenceArtifacts: []string{"missing-evidence-artifact"},
	})
	if err != nil {
		t.Fatalf("claimOneTaskDone missing evidence: %v", err)
	}
	if bad.Status != "not_ready" || bad.ErrorCode != "EVIDENCE_TARGET_NOT_FOUND" {
		t.Fatalf("missing evidence output = status=%q code=%q, want not_ready EVIDENCE_TARGET_NOT_FOUND", bad.Status, bad.ErrorCode)
	}
}

func callArtifactReadForEvidenceTest(t *testing.T, ctx context.Context, pool *db.Pool, projectSlug, taskSlug string) artifactReadOutput {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-evidence-test", Version: "test"}, nil)
	RegisterArtifactRead(server, Deps{
		DB:        pool,
		Logger:    logger,
		AuthChain: auth.NewChain(auth.NewTrustedLocalResolver("", "agent:evidence-test")),
	})
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "artifact-read-evidence-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})

	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name: "pindoc.artifact.read",
		Arguments: map[string]any{
			"project_slug": projectSlug,
			"id_or_slug":   taskSlug,
			"view":         "continuation",
		},
	})
	if err != nil {
		t.Fatalf("CallTool artifact.read: %v", err)
	}
	if res.IsError {
		t.Fatalf("artifact.read result error: %s", toolResultText(res))
	}
	var out artifactReadOutput
	if err := decodeStructuredContent(res.StructuredContent, &out); err != nil {
		t.Fatalf("decode artifact.read structured content: %v", err)
	}
	return out
}

func assertEvidenceEdges(t *testing.T, ctx context.Context, pool *db.Pool, taskID, slugA, slugB string) {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM artifact_edges e
		JOIN artifacts target ON target.id = e.target_id
		WHERE e.source_id = $1::uuid
		  AND e.relation = 'evidence'
		  AND target.slug IN ($2, $3)
	`, taskID, slugA, slugB).Scan(&n); err != nil {
		t.Fatalf("count evidence edges: %v", err)
	}
	if n != 2 {
		t.Fatalf("evidence edge count = %d, want 2", n)
	}
}

func edgeRefsContainEvidence(edges []EdgeRef, slugA, slugB string) bool {
	seen := map[string]bool{}
	for _, edge := range edges {
		if edge.Relation == "evidence" && (edge.Slug == slugA || edge.Slug == slugB) {
			seen[edge.Slug] = true
		}
	}
	return seen[slugA] && seen[slugB]
}
