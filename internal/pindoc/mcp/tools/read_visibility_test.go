package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestMCPReadArtifactVisibilityWhere(t *testing.T) {
	publicOnly, publicArgs := mcpReadArtifactVisibilityWhere(nil, "a", 1)
	if publicOnly != "(a.visibility = $1)" || strings.Join(anyStrings(publicArgs), ",") != projects.VisibilityPublic {
		t.Fatalf("public-only where = %q args=%v", publicOnly, publicArgs)
	}

	member := &mcpReadProjectScope{
		UserID: "11111111-1111-1111-1111-111111111111",
		Member: true,
	}
	memberWhere, memberArgs := mcpReadArtifactVisibilityWhere(member, "doc", 4)
	for _, want := range []string{"doc.visibility = $4", "doc.visibility = $5", "doc.visibility = $6", "doc.author_user_id::text = $7"} {
		if !strings.Contains(memberWhere, want) {
			t.Fatalf("member where %q missing %q", memberWhere, want)
		}
	}
	if strings.Join(anyStrings(memberArgs), ",") != "public,org,private,11111111-1111-1111-1111-111111111111" {
		t.Fatalf("member args = %v", memberArgs)
	}

	trustedWhere, trustedArgs := mcpReadArtifactVisibilityWhere(&mcpReadProjectScope{TrustedAll: true}, "a", 1)
	if trustedWhere != "TRUE" || len(trustedArgs) != 0 {
		t.Fatalf("trusted where = %q args=%v", trustedWhere, trustedArgs)
	}
}

func TestMCPReadVisibilityIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run MCP read visibility DB integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	fixture := seedMCPVisibilityFixture(t, ctx, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, fixture.projectID)
	})

	publicOnly := &auth.Principal{UserID: fixture.outsiderUserID, AgentID: "agent:visibility-test", Source: auth.SourceOAuth}
	member := &auth.Principal{UserID: fixture.memberUserID, AgentID: "agent:visibility-test", Source: auth.SourceOAuth}
	trusted := &auth.Principal{UserID: fixture.ownerUserID, AgentID: "agent:visibility-test", Source: auth.SourceLoopback}

	publicRead := callVisibilityTool[artifactReadOutput](t, ctx, pool, nil, publicOnly, "pindoc.artifact.read", map[string]any{
		"project_slug": fixture.projectSlug,
		"id_or_slug":   "vis-public",
	})
	if publicRead.Slug != "vis-public" {
		t.Fatalf("public read slug = %q", publicRead.Slug)
	}
	assertVisibilityToolError(t, ctx, pool, nil, publicOnly, "pindoc.artifact.read", map[string]any{
		"project_slug": fixture.projectSlug,
		"id_or_slug":   "vis-org",
	}, "not found")

	memberPrivate := callVisibilityTool[artifactReadOutput](t, ctx, pool, nil, member, "pindoc.artifact.read", map[string]any{
		"project_slug": fixture.projectSlug,
		"id_or_slug":   "vis-private-self",
	})
	if memberPrivate.Slug != "vis-private-self" {
		t.Fatalf("member private self read slug = %q", memberPrivate.Slug)
	}
	assertVisibilityToolError(t, ctx, pool, nil, member, "pindoc.artifact.read", map[string]any{
		"project_slug": fixture.projectSlug,
		"id_or_slug":   "vis-private-other",
	}, "not found")

	searchProvider := embed.NewStub(32)
	publicSearch := callVisibilityTool[artifactSearchOutput](t, ctx, pool, searchProvider, publicOnly, "pindoc.artifact.search", map[string]any{
		"project_slug": fixture.projectSlug,
		"query":        "visibility fixture",
		"top_k":        10,
	})
	if got := searchHitSlugs(publicSearch.Hits); strings.Join(got, ",") != "vis-public" {
		t.Fatalf("public search slugs = %v, want [vis-public]", got)
	}
	memberSearch := callVisibilityTool[artifactSearchOutput](t, ctx, pool, searchProvider, member, "pindoc.artifact.search", map[string]any{
		"project_slug": fixture.projectSlug,
		"query":        "visibility fixture",
		"top_k":        10,
	})
	if got := strings.Join(searchHitSlugs(memberSearch.Hits), ","); got != "vis-org,vis-private-self,vis-public" {
		t.Fatalf("member search slugs = %s", got)
	}
	trustedSearch := callVisibilityTool[artifactSearchOutput](t, ctx, pool, searchProvider, trusted, "pindoc.artifact.search", map[string]any{
		"project_slug": fixture.projectSlug,
		"query":        "visibility fixture",
		"top_k":        10,
	})
	if got := strings.Join(searchHitSlugs(trustedSearch.Hits), ","); got != "vis-org,vis-private-other,vis-private-self,vis-public" {
		t.Fatalf("trusted search slugs = %s", got)
	}

	publicTranslate := callVisibilityTool[artifactTranslateOutput](t, ctx, pool, nil, publicOnly, "pindoc.artifact.translate", map[string]any{
		"project_slug":  fixture.projectSlug,
		"artifact_slug": "vis-public",
		"target_locale": "ja",
		"use_cache":     true,
	})
	if publicTranslate.ArtifactSlug != "vis-public" || publicTranslate.CachedSlug != "" {
		t.Fatalf("public translate should see source but not private cache: %+v", publicTranslate)
	}
	assertVisibilityToolError(t, ctx, pool, nil, publicOnly, "pindoc.artifact.translate", map[string]any{
		"project_slug":  fixture.projectSlug,
		"artifact_slug": "vis-org",
		"target_locale": "ja",
	}, "not found")
	trustedTranslate := callVisibilityTool[artifactTranslateOutput](t, ctx, pool, nil, trusted, "pindoc.artifact.translate", map[string]any{
		"project_slug":  fixture.projectSlug,
		"artifact_slug": "vis-public",
		"target_locale": "ja",
	})
	if trustedTranslate.CachedSlug != "vis-public-ja-private-cache" {
		t.Fatalf("trusted translate cached slug = %q", trustedTranslate.CachedSlug)
	}

	memberAudit := callVisibilityTool[artifactAuditOutput](t, ctx, pool, nil, member, "pindoc.artifact.audit", map[string]any{
		"project_slug": fixture.projectSlug,
		"limit":        100,
	})
	if got := auditFindingSlugs(memberAudit.Findings); strings.Contains(strings.Join(got, ","), "vis-private-other") {
		t.Fatalf("member audit leaked private other: %v", got)
	}
	for _, want := range []string{"vis-public", "vis-org", "vis-private-self"} {
		if !containsString(auditFindingSlugs(memberAudit.Findings), want) {
			t.Fatalf("member audit slugs missing %s: %v", want, auditFindingSlugs(memberAudit.Findings))
		}
	}
}

type mcpVisibilityFixture struct {
	projectID      string
	projectSlug    string
	ownerUserID    string
	memberUserID   string
	outsiderUserID string
}

func seedMCPVisibilityFixture(t *testing.T, ctx context.Context, pool *db.Pool) mcpVisibilityFixture {
	t.Helper()
	suffix := time.Now().UnixNano()
	ownerID := insertMCPVisibilityUser(t, ctx, pool, fmt.Sprintf("owner-%d@example.invalid", suffix))
	memberID := insertMCPVisibilityUser(t, ctx, pool, fmt.Sprintf("member-%d@example.invalid", suffix))
	outsiderID := insertMCPVisibilityUser(t, ctx, pool, fmt.Sprintf("outsider-%d@example.invalid", suffix))

	projectSlug := fmt.Sprintf("vis-mcp-%d", suffix)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin project create: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            projectSlug,
		Name:            "Visibility MCP",
		PrimaryLanguage: "ko",
		OwnerUserID:     ownerID,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit project create: %v", err)
	}

	var orgID, areaID string
	if err := pool.QueryRow(ctx, `SELECT organization_id::text FROM projects WHERE id = $1::uuid`, created.ID).Scan(&orgID); err != nil {
		t.Fatalf("select org id: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM areas WHERE project_id = $1::uuid AND slug = 'misc'`, created.ID).Scan(&areaID); err != nil {
		t.Fatalf("select misc area: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, 'viewer')
		ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, created.ID, memberID); err != nil {
		t.Fatalf("insert project member: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, 'member')
		ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, orgID, memberID); err != nil {
		t.Fatalf("insert org member: %v", err)
	}

	ids := map[string]string{}
	ids["vis-public"] = insertMCPVisibilityArtifact(t, ctx, pool, created.ID, areaID, "vis-public", projects.VisibilityPublic, ownerID, "fr")
	ids["vis-org"] = insertMCPVisibilityArtifact(t, ctx, pool, created.ID, areaID, "vis-org", projects.VisibilityOrg, ownerID, "fr")
	ids["vis-private-self"] = insertMCPVisibilityArtifact(t, ctx, pool, created.ID, areaID, "vis-private-self", projects.VisibilityPrivate, memberID, "fr")
	ids["vis-private-other"] = insertMCPVisibilityArtifact(t, ctx, pool, created.ID, areaID, "vis-private-other", projects.VisibilityPrivate, ownerID, "fr")
	cacheID := insertMCPVisibilityArtifact(t, ctx, pool, created.ID, areaID, "vis-public-ja-private-cache", projects.VisibilityPrivate, ownerID, "ja")
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifact_edges (source_id, target_id, relation)
		VALUES ($1::uuid, $2::uuid, 'translation_of')
	`, cacheID, ids["vis-public"]); err != nil {
		t.Fatalf("insert translation edge: %v", err)
	}

	provider := embed.NewStub(32)
	for slug, id := range ids {
		insertMCPVisibilityChunk(t, ctx, pool, provider, id, "visibility fixture "+slug)
	}

	return mcpVisibilityFixture{
		projectID:      created.ID,
		projectSlug:    projectSlug,
		ownerUserID:    ownerID,
		memberUserID:   memberID,
		outsiderUserID: outsiderID,
	}
}

func insertMCPVisibilityUser(t *testing.T, ctx context.Context, pool *db.Pool, email string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ($1, $2, 'pindoc_admin')
		RETURNING id::text
	`, strings.TrimSuffix(email, "@example.invalid"), email).Scan(&id); err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	return id
}

func insertMCPVisibilityArtifact(t *testing.T, ctx context.Context, pool *db.Pool, projectID, areaID, slug, visibility, authorUserID, bodyLocale string) string {
	t.Helper()
	body := "## Context\nvisibility fixture " + slug + "\n"
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (
			project_id, area_id, slug, type, title, body_markdown, body_locale,
			author_id, author_user_id, completeness, status, visibility, published_at
		)
		VALUES (
			$1::uuid, $2::uuid, $3, 'Decision', $4, $5, $6,
			'agent:visibility-test', NULLIF($7, '')::uuid, 'partial', 'published', $8, now()
		)
		RETURNING id::text
	`, projectID, areaID, slug, "Visibility Fixture "+slug, body, bodyLocale, authorUserID, visibility).Scan(&id); err != nil {
		t.Fatalf("insert artifact %s: %v", slug, err)
	}
	return id
}

func insertMCPVisibilityChunk(t *testing.T, ctx context.Context, pool *db.Pool, provider embed.Provider, artifactID, text string) {
	t.Helper()
	res, err := provider.Embed(ctx, embed.Request{Texts: []string{text}, Kind: embed.KindDocument})
	if err != nil {
		t.Fatalf("embed fixture chunk: %v", err)
	}
	vec := embed.VectorString(embed.PadTo768(res.Vectors[0]))
	info := provider.Info()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifact_chunks (
			artifact_id, kind, chunk_index, heading, text, embedding, model_name, model_dim
		)
		VALUES ($1::uuid, 'body', 0, 'Body', $2, $3::vector, $4, $5)
	`, artifactID, text, vec, info.Name, info.Dimension); err != nil {
		t.Fatalf("insert artifact chunk: %v", err)
	}
}

type staticMCPVisibilityResolver struct {
	p *auth.Principal
}

func (r staticMCPVisibilityResolver) Resolve(context.Context, *sdk.CallToolRequest) (*auth.Principal, error) {
	return r.p, nil
}

func callVisibilityTool[T any](t *testing.T, ctx context.Context, pool *db.Pool, provider embed.Provider, p *auth.Principal, name string, args map[string]any) T {
	t.Helper()
	res, err := callVisibilityToolRaw(t, ctx, pool, provider, p, name, args)
	if err != nil {
		var zero T
		t.Fatalf("CallTool %s: %v", name, err)
		return zero
	}
	var out T
	if err := decodeStructuredContent(res.StructuredContent, &out); err != nil {
		t.Fatalf("decode %s structured content: %v", name, err)
	}
	return out
}

func assertVisibilityToolError(t *testing.T, ctx context.Context, pool *db.Pool, provider embed.Provider, p *auth.Principal, name string, args map[string]any, want string) {
	t.Helper()
	_, err := callVisibilityToolRaw(t, ctx, pool, provider, p, name, args)
	if err == nil {
		t.Fatalf("CallTool %s succeeded; want error containing %q", name, want)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
		t.Fatalf("CallTool %s error = %v; want contains %q", name, err, want)
	}
}

func callVisibilityToolRaw(t *testing.T, ctx context.Context, pool *db.Pool, provider embed.Provider, p *auth.Principal, name string, args map[string]any) (*sdk.CallToolResult, error) {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "pindoc-read-visibility-test", Version: "test"}, nil)
	deps := Deps{
		DB:        pool,
		Embedder:  provider,
		AuthChain: auth.NewChain(staticMCPVisibilityResolver{p: p}),
	}
	RegisterArtifactRead(server, deps)
	RegisterArtifactSearch(server, deps)
	RegisterArtifactTranslate(server, deps)
	RegisterArtifactAudit(server, deps)

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, err
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "read-visibility-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})

	res, err := clientSession.CallTool(ctx, &sdk.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, errors.New(toolResultText(res))
	}
	return res, nil
}

func searchHitSlugs(hits []SearchHit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		if strings.HasPrefix(h.Slug, "vis-") && h.Slug != "vis-public-ja-private-cache" {
			out = append(out, h.Slug)
		}
	}
	sort.Strings(out)
	return out
}

func auditFindingSlugs(findings []artifactAuditFinding) []string {
	seen := map[string]struct{}{}
	for _, finding := range findings {
		if strings.HasPrefix(finding.Slug, "vis-") {
			seen[finding.Slug] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for slug := range seen {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}

func anyStrings(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, fmt.Sprint(value))
	}
	return out
}
