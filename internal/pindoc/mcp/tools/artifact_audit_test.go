package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestArtifactAuditFindingsForRow(t *testing.T) {
	scope := &auth.ProjectScope{ProjectSlug: "pindoc", ProjectLocale: "ko"}
	now := time.Now()
	filter := artifactAuditFilter{Limit: artifactAuditDefaultLimit}

	titleRow := artifactAuditRow{
		ArtifactID:     "a1",
		Slug:           "english-title",
		Type:           "Decision",
		Title:          "Task flow lens cross-project sequence",
		AreaSlug:       "mcp",
		Status:         "published",
		BodyLocale:     "ko",
		RevisionNumber: 1,
		UpdatedAt:      now,
	}
	titleFindings := artifactAuditFindingsForRow(Deps{}, scope, titleRow, filter)
	if !artifactAuditHasCode(titleFindings, titleLocaleMismatchWarning) {
		t.Fatalf("title findings missing %s: %+v", titleLocaleMismatchWarning, titleFindings)
	}

	warningRow := artifactAuditRow{
		ArtifactID:        "a2",
		Slug:              "stored-warning",
		Type:              "Decision",
		Title:             "저장 warning",
		AreaSlug:          "mcp",
		Status:            "published",
		BodyLocale:        "ko",
		WarningPayloadRaw: []byte(`{"codes":["SOURCE_TYPE_UNCLASSIFIED"],"revision_number":2}`),
		RevisionNumber:    2,
		UpdatedAt:         now,
	}
	warningFindings := artifactAuditFindingsForRow(Deps{}, scope, warningRow, filter)
	sourceFinding := artifactAuditFindByCode(warningFindings, "SOURCE_TYPE_UNCLASSIFIED")
	if sourceFinding == nil || sourceFinding.FindingKind != artifactAuditKindMetadata || sourceFinding.RecommendedAction != "meta_patch" {
		t.Fatalf("stored warning finding = %+v in %+v", sourceFinding, warningFindings)
	}

	openTaskRow := artifactAuditRow{
		ArtifactID:     "a3",
		Slug:           "open-resolved",
		Type:           "Task",
		Title:          "완료된 열린 Task",
		AreaSlug:       "mcp",
		Status:         "published",
		BodyMarkdown:   "- [x] implemented\n- [~] partial accepted\n",
		BodyLocale:     "ko",
		TaskMetaRaw:    []byte(`{"status":"open"}`),
		RevisionNumber: 1,
		UpdatedAt:      now,
	}
	openFindings := artifactAuditFindingsForRow(Deps{}, scope, openTaskRow, filter)
	if got := artifactAuditFindByCode(openFindings, taskWarningAcceptanceReconcilePending); got == nil || got.RecommendedAction != "meta_patch" {
		t.Fatalf("open resolved task finding = %+v in %+v", got, openFindings)
	}

	claimedTaskRow := artifactAuditRow{
		ArtifactID:     "a4",
		Slug:           "claimed-unresolved",
		Type:           "Task",
		Title:          "미해결 완료 Task",
		AreaSlug:       "mcp",
		Status:         "published",
		BodyMarkdown:   "- [ ] QA pass\n- [~] partial investigation\n",
		BodyLocale:     "ko",
		TaskMetaRaw:    []byte(`{"status":"claimed_done"}`),
		RevisionNumber: 1,
		UpdatedAt:      now,
	}
	claimedFindings := artifactAuditFindingsForRow(Deps{}, scope, claimedTaskRow, filter)
	claimed := artifactAuditFindByCode(claimedFindings, "TASK_CLAIMED_DONE_UNRESOLVED_ACCEPTANCE")
	if claimed == nil || claimed.RecommendedAction == "reopen" || claimed.RecommendedAction != "create_followup_task" {
		t.Fatalf("claimed_done unresolved finding = %+v in %+v", claimed, claimedFindings)
	}

	staleRow := artifactAuditRow{
		ArtifactID:     "a5",
		Slug:           "old-artifact",
		Type:           "Decision",
		Title:          "오래된 artifact",
		AreaSlug:       "mcp",
		Status:         "published",
		BodyLocale:     "ko",
		RevisionNumber: 1,
		UpdatedAt:      now.Add(-90 * 24 * time.Hour),
	}
	staleFindings := artifactAuditFindingsForRow(Deps{}, scope, staleRow, filter)
	if got := artifactAuditFindByCode(staleFindings, "ARTIFACT_STALE_AGE"); got == nil || got.Severity != SeverityInfo {
		t.Fatalf("stale finding = %+v in %+v", got, staleFindings)
	}

	concentrationRow := artifactAuditRow{
		ArtifactID:        "a6",
		Slug:              "crowded-area-artifact",
		Type:              "Decision",
		Title:             "붐비는 area artifact",
		AreaSlug:          "content",
		Status:            "published",
		BodyLocale:        "ko",
		RevisionNumber:    1,
		AreaArtifactCount: 87,
		UpdatedAt:         now,
	}
	concentrationFindings := artifactAuditFindingsForRow(Deps{}, scope, concentrationRow, filter)
	concentration := artifactAuditFindByCode(concentrationFindings, "AREA_CONCENTRATION")
	if concentration == nil || concentration.FindingKind != artifactAuditKindAreaConcentration || concentration.RecommendedAction != "set_area" || concentration.Severity != SeverityInfo {
		t.Fatalf("area concentration finding = %+v in %+v", concentration, concentrationFindings)
	}

	kindFilter := artifactAuditFilter{
		Limit:    artifactAuditDefaultLimit,
		Kinds:    map[string]struct{}{artifactAuditKindTaskLifecycle: {}},
		KindList: []string{artifactAuditKindTaskLifecycle},
	}
	filtered := artifactAuditFindingsForRow(Deps{}, scope, openTaskRow, kindFilter)
	if len(filtered) != 1 || filtered[0].FindingKind != artifactAuditKindTaskLifecycle {
		t.Fatalf("kind-filtered findings = %+v", filtered)
	}
}

func TestNormalizeArtifactAuditFilter(t *testing.T) {
	filter, err := normalizeArtifactAuditFilter(artifactAuditInput{
		Area:              "mcp",
		Areas:             []string{"mcp", "ui"},
		Type:              "task",
		Status:            "",
		Kind:              "task_lifecycle",
		Limit:             900,
		IncludeSuperseded: true,
	})
	if err != nil {
		t.Fatalf("normalize filter: %v", err)
	}
	if strings.Join(filter.Areas, ",") != "mcp,ui" {
		t.Fatalf("areas = %v", filter.Areas)
	}
	if len(filter.Types) != 1 || filter.Types[0] != "Task" {
		t.Fatalf("types = %v", filter.Types)
	}
	if !containsString(filter.Statuses, "superseded") || !filter.DefaultStatuses {
		t.Fatalf("statuses/default = %v/%v", filter.Statuses, filter.DefaultStatuses)
	}
	if filter.Limit != artifactAuditMaxLimit {
		t.Fatalf("limit = %d, want max clamp %d", filter.Limit, artifactAuditMaxLimit)
	}
	if _, ok := filter.Kinds[artifactAuditKindTaskLifecycle]; !ok {
		t.Fatalf("kind filter missing task_lifecycle: %+v", filter.Kinds)
	}
	areaFilter, err := normalizeArtifactAuditFilter(artifactAuditInput{Kind: "area_concentration"})
	if err != nil {
		t.Fatalf("normalize area_concentration kind: %v", err)
	}
	if _, ok := areaFilter.Kinds[artifactAuditKindAreaConcentration]; !ok {
		t.Fatalf("kind filter missing area_concentration: %+v", areaFilter.Kinds)
	}

	if _, err := normalizeArtifactAuditFilter(artifactAuditInput{Status: "deleted"}); err == nil {
		t.Fatalf("invalid status should fail")
	}
	if _, err := normalizeArtifactAuditFilter(artifactAuditInput{Kind: "reopen"}); err == nil {
		t.Fatalf("invalid kind should fail")
	}
}

func TestArtifactAuditIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run artifact.audit DB integration")
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
	projectSlug := fmt.Sprintf("artifact-audit-%d", suffix)
	areaSlug := "mcp"
	projectID := insertContextReceiptProject(t, ctx, pool, projectSlug)
	areaID := insertContextReceiptArea(t, ctx, pool, projectID, areaSlug)
	if _, err := pool.Exec(ctx, `UPDATE projects SET primary_language = 'ko' WHERE id = $1::uuid`, projectID); err != nil {
		t.Fatalf("set project primary_language: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	})

	empty := callArtifactAuditForTest(t, ctx, pool, map[string]any{
		"project_slug": projectSlug,
	})
	if empty.Count != 0 || len(empty.Findings) != 0 {
		t.Fatalf("empty audit = %+v", empty)
	}

	titleID := insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectID,
		AreaID:    areaID,
		Slug:      "english-title",
		Type:      "Decision",
		Title:     "Task flow lens cross-project sequence",
		Body:      "## Context\nx\n## Decision\ny\n## Rationale\nz\n## Alternatives considered\na\n## Consequences\nb\n",
		Locale:    "ko",
		Status:    "published",
		UpdatedAt: time.Now(),
	})
	warningID := insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectID,
		AreaID:    areaID,
		Slug:      "stored-warning",
		Type:      "Decision",
		Title:     "저장 warning",
		Body:      "## Context\nx\n## Decision\ny\n## Rationale\nz\n## Alternatives considered\na\n## Consequences\nb\n",
		Locale:    "ko",
		Status:    "published",
		UpdatedAt: time.Now(),
	})
	insertArtifactAuditWarningEvent(t, ctx, pool, projectID, warningID, `{"codes":["SOURCE_TYPE_UNCLASSIFIED"],"revision_number":1,"author_id":"tester"}`)
	insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectID,
		AreaID:    areaID,
		Slug:      "open-resolved",
		Type:      "Task",
		Title:     "완료된 열린 Task",
		Body:      "- [x] implemented\n",
		Locale:    "ko",
		Status:    "published",
		TaskMeta:  `{"status":"open","priority":"p2"}`,
		UpdatedAt: time.Now(),
	})
	insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectID,
		AreaID:    areaID,
		Slug:      "claimed-unresolved",
		Type:      "Task",
		Title:     "미해결 완료 Task",
		Body:      "- [ ] QA pass\n",
		Locale:    "ko",
		Status:    "published",
		TaskMeta:  `{"status":"claimed_done","priority":"p2"}`,
		UpdatedAt: time.Now(),
	})
	insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectID,
		AreaID:    areaID,
		Slug:      "old-artifact",
		Type:      "Decision",
		Title:     "오래된 artifact",
		Body:      "## Context\nx\n## Decision\ny\n## Rationale\nz\n## Alternatives considered\na\n## Consequences\nb\n",
		Locale:    "ko",
		Status:    "published",
		UpdatedAt: time.Now().Add(-90 * 24 * time.Hour),
	})
	insertArtifactAuditArtifact(t, ctx, pool, artifactAuditFixture{
		ProjectID: projectID,
		AreaID:    areaID,
		Slug:      "superseded-artifact",
		Type:      "Decision",
		Title:     "대체된 artifact",
		Body:      "## Context\nx\n## Decision\ny\n## Rationale\nz\n## Alternatives considered\na\n## Consequences\nb\n",
		Locale:    "ko",
		Status:    "superseded",
		UpdatedAt: time.Now(),
	})

	revisionsBefore := artifactAuditRevisionCount(t, ctx, pool, projectID)
	out := callArtifactAuditForTest(t, ctx, pool, map[string]any{
		"project_slug": projectSlug,
		"limit":        100,
	})
	for _, code := range []string{
		titleLocaleMismatchWarning,
		"SOURCE_TYPE_UNCLASSIFIED",
		taskWarningAcceptanceReconcilePending,
		"TASK_CLAIMED_DONE_UNRESOLVED_ACCEPTANCE",
		"ARTIFACT_STALE_AGE",
	} {
		if !artifactAuditHasCode(out.Findings, code) {
			t.Fatalf("audit output missing %s: %+v", code, out.Findings)
		}
	}
	if artifactAuditHasCode(out.Findings, "ARTIFACT_SUPERSEDED") {
		t.Fatalf("default audit should exclude superseded findings: %+v", out.Findings)
	}
	if got := artifactAuditFindByCode(out.Findings, "TASK_CLAIMED_DONE_UNRESOLVED_ACCEPTANCE"); got == nil || got.RecommendedAction == "reopen" {
		t.Fatalf("completed task action should not reopen: %+v", got)
	}
	if revisionsAfter := artifactAuditRevisionCount(t, ctx, pool, projectID); revisionsAfter != revisionsBefore {
		t.Fatalf("artifact.audit must be read-only; revision count before=%d after=%d", revisionsBefore, revisionsAfter)
	}

	kindOut := callArtifactAuditForTest(t, ctx, pool, map[string]any{
		"project_slug": projectSlug,
		"kind":         "task_lifecycle",
		"limit":        100,
	})
	if len(kindOut.Findings) != 2 {
		t.Fatalf("task_lifecycle findings len = %d, want 2: %+v", len(kindOut.Findings), kindOut.Findings)
	}
	for _, finding := range kindOut.Findings {
		if finding.FindingKind != artifactAuditKindTaskLifecycle {
			t.Fatalf("kind-filtered finding = %+v", finding)
		}
	}

	supersededOut := callArtifactAuditForTest(t, ctx, pool, map[string]any{
		"project_slug":       projectSlug,
		"include_superseded": true,
		"kind":               "supersede_candidate",
		"limit":              100,
	})
	if got := artifactAuditFindByCode(supersededOut.Findings, "ARTIFACT_SUPERSEDED"); got == nil || got.Slug != "superseded-artifact" {
		t.Fatalf("include_superseded finding = %+v in %+v", got, supersededOut.Findings)
	}

	var titleUpdated time.Time
	if err := pool.QueryRow(ctx, `SELECT updated_at FROM artifacts WHERE id = $1::uuid`, titleID).Scan(&titleUpdated); err != nil {
		t.Fatalf("read title artifact updated_at: %v", err)
	}
	if titleUpdated.IsZero() {
		t.Fatalf("title fixture was not persisted")
	}
}

type artifactAuditFixture struct {
	ProjectID string
	AreaID    string
	Slug      string
	Type      string
	Title     string
	Body      string
	Locale    string
	Status    string
	TaskMeta  string
	UpdatedAt time.Time
}

func insertArtifactAuditArtifact(t *testing.T, ctx context.Context, pool *db.Pool, f artifactAuditFixture) string {
	t.Helper()
	if strings.TrimSpace(f.Locale) == "" {
		f.Locale = "en"
	}
	if strings.TrimSpace(f.Status) == "" {
		f.Status = "published"
	}
	if f.UpdatedAt.IsZero() {
		f.UpdatedAt = time.Now()
	}
	taskMeta := strings.TrimSpace(f.TaskMeta)
	if taskMeta == "" {
		taskMeta = "{}"
	}
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (
			project_id, area_id, slug, type, title, body_markdown, body_locale,
			author_id, completeness, status, task_meta, updated_at, published_at
		)
		VALUES (
			$1::uuid, $2::uuid, $3, $4, $5, $6, $7,
			'tester', 'partial', $8, $9::jsonb, $10, $10
		)
		RETURNING id::text
	`, f.ProjectID, f.AreaID, f.Slug, f.Type, f.Title, f.Body, f.Locale, f.Status, taskMeta, f.UpdatedAt).Scan(&id); err != nil {
		t.Fatalf("insert audit artifact %s: %v", f.Slug, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash, tags,
			completeness, author_kind, author_id, commit_msg, revision_shape, created_at
		)
		VALUES (
			$1::uuid, 1, $2, $3,
			encode(sha256(convert_to($3, 'UTF8')), 'hex'),
			'{}'::text[], 'partial', 'agent', 'tester', 'seed audit fixture', 'body_patch', $4
		)
	`, id, f.Title, f.Body, f.UpdatedAt); err != nil {
		t.Fatalf("insert audit revision %s: %v", f.Slug, err)
	}
	return id
}

func insertArtifactAuditWarningEvent(t *testing.T, ctx context.Context, pool *db.Pool, projectID, artifactID, payload string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1::uuid, 'artifact.warning_raised', $2::uuid, $3::jsonb)
	`, projectID, artifactID, payload); err != nil {
		t.Fatalf("insert warning event: %v", err)
	}
}

func artifactAuditRevisionCount(t *testing.T, ctx context.Context, pool *db.Pool, projectID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::int
		FROM artifact_revisions r
		JOIN artifacts a ON a.id = r.artifact_id
		WHERE a.project_id = $1::uuid
	`, projectID).Scan(&n); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	return n
}

func callArtifactAuditForTest(t *testing.T, ctx context.Context, pool *db.Pool, args map[string]any) artifactAuditOutput {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-artifact-audit-test", Version: "test"}, nil)
	RegisterArtifactAudit(server, Deps{
		DB:        pool,
		AuthChain: auth.NewChain(auth.NewTrustedLocalResolver("", "agent:codex")),
	})

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "artifact-audit-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})

	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name:      "pindoc.artifact.audit",
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool artifact.audit: %v", err)
	}
	if res.IsError {
		t.Fatalf("artifact.audit result error: %s", toolResultText(res))
	}
	var out artifactAuditOutput
	if err := decodeStructuredContent(res.StructuredContent, &out); err != nil {
		t.Fatalf("decode artifact.audit structured content: %v", err)
	}
	return out
}

func artifactAuditHasCode(findings []artifactAuditFinding, code string) bool {
	return artifactAuditFindByCode(findings, code) != nil
}

func artifactAuditFindByCode(findings []artifactAuditFinding, code string) *artifactAuditFinding {
	for i := range findings {
		if findings[i].Code == code {
			return &findings[i]
		}
	}
	return nil
}
