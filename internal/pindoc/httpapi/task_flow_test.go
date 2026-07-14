package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

func TestParseTaskFlowHTTPRequest(t *testing.T) {
	q := url.Values{}
	q.Set("project_scope", "caller_visible")
	q.Add("project_slugs", "alpha,beta")
	q.Add("actor_ids", "agent:codex,@owner")
	q.Set("include_unassigned", "true")
	q.Set("flow_scope", "all")
	q.Set("limit", "900")

	got, err := parseTaskFlowHTTPRequest("pindoc", q, false)
	if err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if got.ProjectSlug != "pindoc" {
		t.Fatalf("project slug = %q", got.ProjectSlug)
	}
	if strings.Join(got.ProjectSlugs, ",") != "alpha,beta" {
		t.Fatalf("project slugs = %v", got.ProjectSlugs)
	}
	if strings.Join(got.ActorIDs, ",") != "agent:codex,@owner" {
		t.Fatalf("actor ids = %v", got.ActorIDs)
	}
	if !got.IncludeUnassigned || got.FlowScope != "all" || got.Limit != 900 {
		t.Fatalf("parsed flags = %+v", got)
	}
}

func TestTaskFlowHiddenProjectIncludeRequiresOwnerScope(t *testing.T) {
	cases := []struct {
		name      string
		requested bool
		role      string
		want      bool
	}{
		{name: "owner with query", requested: true, role: pauth.RoleOwner, want: true},
		{name: "owner without query", requested: false, role: pauth.RoleOwner, want: false},
		{name: "viewer with query", requested: true, role: pauth.RoleViewer, want: false},
		{name: "editor with query", requested: true, role: pauth.RoleEditor, want: false},
		{name: "nil scope", requested: true, role: "", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var scope *pauth.ProjectScope
			if c.role != "" {
				scope = &pauth.ProjectScope{Role: c.role}
			}
			if got := includeReaderHiddenProjectsForScope(c.requested, scope); got != c.want {
				t.Fatalf("includeReaderHiddenProjectsForScope(%v, role=%q) = %v, want %v", c.requested, c.role, got, c.want)
			}
		})
	}
}

func TestTaskFlowProjectSelectionUsesReaderHiddenColumnIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run task-flow reader-hidden DB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	baseDefaultCount, err := projects.CountVisible(ctx, pool, projects.ViewerScope{TrustedLocal: true})
	if err != nil {
		t.Fatalf("baseline default visible count: %v", err)
	}
	baseOperatorCount, err := projects.CountVisible(ctx, pool, projects.ViewerScope{TrustedLocal: true, IncludeReaderHidden: true})
	if err != nil {
		t.Fatalf("baseline operator visible count: %v", err)
	}

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	visibleSlug := "customer-visible-" + suffix
	hiddenSlug := "plugin-fixture-" + suffix
	for slug, hidden := range map[string]bool{visibleSlug: false, hiddenSlug: true} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO projects (
				organization_id, slug, name, primary_language, reader_hidden
			) VALUES (
				(SELECT id FROM organizations WHERE slug = 'default' LIMIT 1),
				$1, $2, 'en', $3
			)
		`, slug, "Task flow "+slug, hidden); err != nil {
			t.Fatalf("insert project %s: %v", slug, err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE slug = ANY($1)`, []string{visibleSlug, hiddenSlug})
	})

	principal := &pauth.Principal{Source: pauth.SourceLoopback, UserID: "task-flow-fixture-owner"}
	deps := Deps{DB: pool}
	if _, err := deps.resolveTaskFlowHTTPScope(ctx, principal, hiddenSlug, false); !errors.Is(err, errTaskFlowHiddenProject) {
		t.Fatalf("hidden scope without opt-in error = %v, want errTaskFlowHiddenProject", err)
	}
	resolved, err := deps.resolveTaskFlowHTTPScope(ctx, principal, hiddenSlug, true)
	if err != nil {
		t.Fatalf("hidden scope with owner opt-in: %v", err)
	}
	if !resolved.ReaderHidden {
		t.Fatalf("resolved hidden scope = %+v", resolved)
	}

	visibleOnly, err := deps.visibleTaskFlowHTTPScopes(ctx, principal, false)
	if err != nil {
		t.Fatalf("visible scopes: %v", err)
	}
	if !taskFlowScopesContain(visibleOnly, visibleSlug) || taskFlowScopesContain(visibleOnly, hiddenSlug) {
		t.Fatalf("visible-only scopes = %+v", visibleOnly)
	}
	withHidden, err := deps.visibleTaskFlowHTTPScopes(ctx, principal, true)
	if err != nil {
		t.Fatalf("include-hidden scopes: %v", err)
	}
	if !taskFlowScopesContain(withHidden, hiddenSlug) {
		t.Fatalf("include-hidden scopes missing %q: %+v", hiddenSlug, withHidden)
	}

	defaultCount, err := projects.CountVisible(ctx, pool, projects.ViewerScope{TrustedLocal: true})
	if err != nil {
		t.Fatalf("default visible count: %v", err)
	}
	operatorCount, err := projects.CountVisible(ctx, pool, projects.ViewerScope{TrustedLocal: true, IncludeReaderHidden: true})
	if err != nil {
		t.Fatalf("operator visible count: %v", err)
	}
	if defaultCount != baseDefaultCount+1 {
		t.Fatalf("default count before=%d after=%d, want only the visible project added", baseDefaultCount, defaultCount)
	}
	if operatorCount != baseOperatorCount+2 {
		t.Fatalf("operator count before=%d after=%d, want visible and hidden projects added", baseOperatorCount, operatorCount)
	}
}

func taskFlowScopesContain(scopes []pauth.ProjectScope, slug string) bool {
	for _, scope := range scopes {
		if scope.ProjectSlug == slug {
			return true
		}
	}
	return false
}

func TestNormalizeTaskFlowHTTPActor(t *testing.T) {
	agent, err := normalizeTaskFlowHTTPActor(&pauth.Principal{AgentID: "codex"}, "agent", "", nil, true, true)
	if err != nil {
		t.Fatalf("normalize agent: %v", err)
	}
	if len(agent.IDs) != 1 || agent.IDs[0] != "agent:codex" || !agent.IncludeUnassigned {
		t.Fatalf("agent actor = %+v", agent)
	}

	user, err := normalizeTaskFlowHTTPActor(&pauth.Principal{UserID: "u1"}, "user", "", nil, false, true)
	if err != nil {
		t.Fatalf("normalize user: %v", err)
	}
	if len(user.IDs) != 1 || user.IDs[0] != "user:u1" {
		t.Fatalf("user actor = %+v", user)
	}

	if _, err := normalizeTaskFlowHTTPActor(&pauth.Principal{}, "agent", "", nil, false, true); err == nil {
		t.Fatalf("missing actor should fail when required")
	}
}

func TestTaskFlowHTTPSortIgnoresDueAtAsPrimaryTruth(t *testing.T) {
	now := time.Now()
	rows := []taskFlowHTTPRow{
		{ProjectSlug: "p", Slug: "blocked", Readiness: taskFlowHTTPReadinessBlocked, Priority: "p0", DueAt: "2026-01-01T00:00:00Z", UpdatedAt: now.Add(-4 * time.Hour)},
		{ProjectSlug: "p", Slug: "ready-p1-new", Readiness: taskFlowHTTPReadinessReady, Priority: "p1", DueAt: "2026-01-01T00:00:00Z", UpdatedAt: now.Add(-1 * time.Hour)},
		{ProjectSlug: "p", Slug: "ready-p0-late-deadline", Readiness: taskFlowHTTPReadinessReady, Priority: "p0", DueAt: "2027-01-01T00:00:00Z", UpdatedAt: now.Add(-2 * time.Hour)},
		{ProjectSlug: "p", Slug: "ready-p0-early-deadline", Readiness: taskFlowHTTPReadinessReady, Priority: "p0", DueAt: "2025-01-01T00:00:00Z", UpdatedAt: now.Add(-3 * time.Hour)},
	}
	sortTaskFlowHTTPRows(rows)
	got := []string{rows[0].Slug, rows[1].Slug, rows[2].Slug, rows[3].Slug}
	want := []string{"ready-p0-early-deadline", "ready-p0-late-deadline", "ready-p1-new", "blocked"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sorted rows = %v, want %v", got, want)
	}
}

func TestTaskFlowHTTPReadinessAndScope(t *testing.T) {
	ready := taskFlowHTTPRow{Status: "open", Readiness: taskFlowHTTPReadinessReady}
	blockedByEdge := taskFlowHTTPRow{Status: "open", Readiness: taskFlowHTTPReadinessBlocked}
	done := taskFlowHTTPRow{Status: "claimed_done", Readiness: taskFlowHTTPReadinessDone}

	if got := taskFlowHTTPReadiness("open", []taskFlowHTTPBlocker{{Slug: "blocker"}}); got != taskFlowHTTPReadinessBlocked {
		t.Fatalf("readiness with blocker = %q", got)
	}
	if !taskFlowHTTPScopeMatches(ready, taskFlowHTTPFlowActive) {
		t.Fatalf("ready open task should match active flow")
	}
	if !taskFlowHTTPScopeMatches(blockedByEdge, taskFlowHTTPFlowBlocked) {
		t.Fatalf("blocked-by-edge task should match blocked flow")
	}
	if taskFlowHTTPScopeMatches(done, taskFlowHTTPFlowActive) {
		t.Fatalf("claimed_done should not match active flow")
	}
	if !taskFlowHTTPScopeMatches(done, taskFlowHTTPFlowAll) {
		t.Fatalf("claimed_done should match all flow")
	}
}
