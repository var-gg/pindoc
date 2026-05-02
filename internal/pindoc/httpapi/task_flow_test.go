package httpapi

import (
	"net/url"
	"strings"
	"testing"
	"time"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
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
