package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeTaskQueueStatusFilter(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"", "pending", true},
		{" pending ", "pending", true},
		{"all", "all", true},
		{"open", "open", true},
		{"missing", taskStatusMissing, true},
		{"missing_status", taskStatusMissing, true},
		{"claimed_done", "claimed_done", true},
		{"blocked", "blocked", true},
		{"cancelled", "cancelled", true},
		{"verified", "", false},
		{"done", "", false},
		{"in_progress", "", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := normalizeTaskQueueStatusFilter(c.in)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestTaskQueueStatusMatchesReaderPendingSemantics(t *testing.T) {
	if !taskQueueStatusMatches("", "pending") {
		t.Fatalf("missing task_meta.status must match pending")
	}
	if !taskQueueStatusMatches("open", "pending") {
		t.Fatalf("open task_meta.status must match pending")
	}
	if taskQueueStatusMatches("claimed_done", "pending") {
		t.Fatalf("claimed_done must not match pending")
	}
	if !taskQueueStatusMatches("", taskStatusMissing) {
		t.Fatalf("missing task_meta.status must match missing_status filter")
	}
	if taskQueueStatusMatches("", "open") {
		t.Fatalf("missing task_meta.status must not match exact open filter")
	}
}

func TestTaskStatusBucket(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", taskStatusMissing},
		{"  ", taskStatusMissing},
		{"Open", "open"},
		{"claimed_done", "claimed_done"},
		{"DONE", taskStatusOther},
	}
	for _, c := range cases {
		if got := taskStatusBucket(c.raw); got != c.want {
			t.Fatalf("taskStatusBucket(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestTaskQueueNoticeSeparatesAcceptanceView(t *testing.T) {
	notice := taskQueueNotice()
	for _, want := range []string{"Reader", "pending", "task_meta.status", "pindoc.ping", "claimed_done", "pindoc.scope.in_flight"} {
		if !strings.Contains(notice, want) {
			t.Fatalf("notice %q missing %q", notice, want)
		}
	}
}

func TestTaskQueueCloseoutNextTools(t *testing.T) {
	if got := taskQueueCloseoutNextTools("pindoc", ""); got != nil {
		t.Fatalf("missing assignee should not emit closeout next tools: %+v", got)
	}
	got := taskQueueCloseoutNextTools("pindoc", " agent:codex ")
	if len(got) != 1 {
		t.Fatalf("next_tools len = %d, want 1", len(got))
	}
	if got[0].Tool != "pindoc.task.done_check" {
		t.Fatalf("tool = %q", got[0].Tool)
	}
	if got[0].Args["project_slug"] != "pindoc" || got[0].Args["assignee"] != "agent:codex" {
		t.Fatalf("args = %+v", got[0].Args)
	}
	if got[0].Args["mode"] != taskDoneCheckModeCurrentOpenOnly {
		t.Fatalf("mode arg = %+v", got[0].Args)
	}
}

func TestTaskQueueWarnings(t *testing.T) {
	bodyDone := "## Acceptance\n- [x] implemented\n- [-] deferred with reason\n"
	got := taskQueueWarnings("open", bodyDone)
	if len(got) != 1 || got[0] != taskWarningAcceptanceReconcilePending {
		t.Fatalf("open + resolved acceptance warnings = %v", got)
	}

	got = taskQueueWarnings(taskStatusMissing, bodyDone)
	if len(got) != 2 {
		t.Fatalf("missing + resolved acceptance warning count = %d, want 2 (%v)", len(got), got)
	}
	wantSeen := map[string]bool{
		taskWarningStatusMissing:              false,
		taskWarningAcceptanceReconcilePending: false,
	}
	for _, w := range got {
		if _, ok := wantSeen[w]; ok {
			wantSeen[w] = true
		}
	}
	for w, seen := range wantSeen {
		if !seen {
			t.Fatalf("missing warning %q in %v", w, got)
		}
	}

	if got := taskQueueWarnings("claimed_done", bodyDone); len(got) != 0 {
		t.Fatalf("claimed_done should not warn about pending status: %v", got)
	}
}

// TestApplyTaskQueueCompact pins the omit-on-compact contract: project-
// aggregate maps go to nil so json encoding skips them, while items
// + totals + notice survive. Default (compact=false) leaves every field
// alone — backward-compat for existing callers.
func TestApplyTaskQueueCompact(t *testing.T) {
	mkOut := func() taskQueueOutput {
		totalOpen := 7
		return taskQueueOutput{
			SourceSemantics:       taskQueueSemantics,
			StatusFilter:          "pending",
			DefaultFocus:          taskQueueDefaultFocus,
			AssigneeFilteredCount: 10,
			AssigneeOpenCount:     7,
			ProjectTotalCount:     100,
			TotalCount:            10,
			PendingCount:          7,
			CountDeprecationNote:  "legacy",
			CountLegend:           taskQueueCountLegend(),
			StatusCounts:          map[string]int{"open": 5, taskStatusMissing: 2, "claimed_done": 3},
			AreaCounts:            map[string]int{"ui": 4, "mcp": 6},
			PriorityCounts:        map[string]int{"p2": 3},
			WarningCounts:         map[string]int{"TASK_STATUS_MISSING": 2},
			Projects: map[string]taskQueueProjectOutput{
				"pindoc": {
					ProjectSlug:       "pindoc",
					AssigneeOpenCount: 7,
					Items:             []taskQueueItem{{ArtifactID: "id-1", Slug: "task-a", Title: "A"}},
				},
			},
			TotalAssigneeOpenCount: &totalOpen,
			Items: []taskQueueItem{
				{ArtifactID: "id-1", Slug: "task-a", Title: "A"},
			},
			Notice: "stay calm",
		}
	}

	t.Run("compact=false leaves aggregates intact", func(t *testing.T) {
		out := mkOut()
		applyTaskQueueCompact(&out, false)
		if out.Compact {
			t.Fatalf("compact mirror should be false")
		}
		if out.StatusCounts == nil || out.AreaCounts == nil || out.PriorityCounts == nil || out.WarningCounts == nil {
			t.Fatalf("default response must keep aggregate maps populated")
		}
	})

	t.Run("compact=true drops aggregates, keeps totals and items", func(t *testing.T) {
		out := mkOut()
		applyTaskQueueCompact(&out, true)
		if !out.Compact {
			t.Fatalf("compact mirror should be true")
		}
		if out.StatusCounts != nil || out.AreaCounts != nil || out.PriorityCounts != nil || out.WarningCounts != nil {
			t.Fatalf("compact must drop status/area/priority/warning maps; got %+v", out)
		}
		if out.TotalCount != 10 || out.PendingCount != 7 || out.AssigneeOpenCount != 7 || out.AssigneeFilteredCount != 10 || out.ProjectTotalCount != 100 {
			t.Fatalf("compact must preserve totals; got total=%d pending=%d open=%d assignee=%d project=%d", out.TotalCount, out.PendingCount, out.AssigneeOpenCount, out.AssigneeFilteredCount, out.ProjectTotalCount)
		}
		if len(out.Items) != 1 {
			t.Fatalf("compact must preserve items; got %d", len(out.Items))
		}
		if out.Projects == nil || out.Projects["pindoc"].AssigneeOpenCount != 7 {
			t.Fatalf("compact must preserve projects map; got %+v", out.Projects)
		}
		if out.TotalAssigneeOpenCount == nil || *out.TotalAssigneeOpenCount != 7 {
			t.Fatalf("compact must preserve total_assignee_open_count; got %v", out.TotalAssigneeOpenCount)
		}

		// JSON contract: aggregate keys are absent (omitempty), totals stay.
		buf, err := json.Marshal(out)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		body := string(buf)
		for _, omitted := range []string{`"status_counts"`, `"area_counts"`, `"priority_counts"`, `"warning_counts"`} {
			if strings.Contains(body, omitted) {
				t.Fatalf("compact JSON must not contain %s; got %s", omitted, body)
			}
		}
		for _, kept := range []string{`"default_focus":"assignee_open_count"`, `"total_count":10`, `"pending_count":7`, `"assignee_open_count":7`, `"assignee_filtered_count":10`, `"project_total_count":100`, `"count_legend"`, `"compact":true`, `"projects"`, `"total_assignee_open_count":7`} {
			if !strings.Contains(body, kept) {
				t.Fatalf("compact JSON missing %s; got %s", kept, body)
			}
		}
	})

	t.Run("nil receiver is safe", func(t *testing.T) {
		applyTaskQueueCompact(nil, true) // should not panic
	})
}

func TestBuildTaskQueueMultiProjectWorkspaceWarning(t *testing.T) {
	got, ok := buildTaskQueueMultiProjectWorkspaceWarning(projectSlugDefaultResult{
		ProjectSlug: "pindoc",
		Via:         projectSlugDefaultEnv,
	}, []string{"vargg", "pindoc", "pindoc"})
	if !ok {
		t.Fatal("expected MULTI_PROJECT_WORKSPACE warning")
	}
	if got.Code != taskWarningMultiProjectWorkspace {
		t.Fatalf("warning code = %q", got.Code)
	}
	if strings.Join(got.DetectedProjects, ",") != "pindoc,vargg" {
		t.Fatalf("detected projects = %v", got.DetectedProjects)
	}
	if !strings.Contains(got.Hint, "across_projects=true") || !strings.Contains(got.Hint, "project_slug") {
		t.Fatalf("warning hint should name across_projects and project_slug: %+v", got)
	}

	if _, ok := buildTaskQueueMultiProjectWorkspaceWarning(projectSlugDefaultResult{}, []string{"pindoc", "vargg"}); ok {
		t.Fatal("explicit project_slug calls must not warn")
	}
	if _, ok := buildTaskQueueMultiProjectWorkspaceWarning(projectSlugDefaultResult{Via: projectSlugDefaultEnv}, []string{"pindoc"}); ok {
		t.Fatal("single-project workspaces must not warn")
	}
}

func TestTaskQueueCountLegendNamesCountSemantics(t *testing.T) {
	legend := taskQueueCountLegend()
	for _, key := range []string{"default_focus", "assignee_filtered_count", "assignee_open_count", "project_total_count", "total_count", "pending_count", "items", "ready_to_close"} {
		if strings.TrimSpace(legend[key]) == "" {
			t.Fatalf("legend missing %s in %+v", key, legend)
		}
	}
	if !strings.Contains(legend["assignee_open_count"], "missing or open") {
		t.Fatalf("assignee_open_count legend should explain pending semantics: %q", legend["assignee_open_count"])
	}
	if !strings.Contains(legend["pending_count"], "assignee_open_count") {
		t.Fatalf("pending_count legend should point at assignee_open_count: %q", legend["pending_count"])
	}
}

func TestTaskQueueReadyToCloseSignal(t *testing.T) {
	cases := []struct {
		name      string
		status    string
		body      string
		ready     bool
		reason    string
		total     int
		resolved  int
		unchecked int
		partial   int
		deferred  int
	}{
		{
			name:      "open with unresolved acceptance",
			status:    "open",
			body:      "- [x] done\n- [ ] remaining\n- [~] partial\n- [-] moved\n",
			ready:     false,
			reason:    "unresolved_acceptance",
			total:     4,
			resolved:  3,
			unchecked: 1,
			partial:   1,
			deferred:  1,
		},
		{
			name:     "missing status all resolved is ready",
			status:   taskStatusMissing,
			body:     "- [x] done\n- [~] partial\n- [-] deferred\n",
			ready:    true,
			reason:   "ready",
			total:    3,
			resolved: 3,
			partial:  1,
			deferred: 1,
		},
		{
			name:     "claimed done is terminal not close target",
			status:   "claimed_done",
			body:     "- [x] done\n",
			ready:    false,
			reason:   "terminal_status",
			total:    1,
			resolved: 1,
		},
		{
			name:   "open task without checklist is not ready",
			status: "open",
			body:   "## Purpose\nNo acceptance section yet\n",
			ready:  false,
			reason: "no_acceptance_checkboxes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := taskQueueItem{}
			applyTaskQueueReadySignal(&item, tc.status, tc.body)
			if item.ReadyToClose != tc.ready || item.ReadyToCloseStatus != tc.reason {
				t.Fatalf("ready/status = %v/%q, want %v/%q", item.ReadyToClose, item.ReadyToCloseStatus, tc.ready, tc.reason)
			}
			if item.AcceptanceCheckboxesTotal != tc.total ||
				item.ResolvedCheckboxes != tc.resolved ||
				item.UnresolvedCheckboxes != tc.unchecked ||
				item.PartialCheckboxes != tc.partial ||
				item.DeferredCheckboxes != tc.deferred {
				t.Fatalf("counts got total=%d resolved=%d unchecked=%d partial=%d deferred=%d",
					item.AcceptanceCheckboxesTotal,
					item.ResolvedCheckboxes,
					item.UnresolvedCheckboxes,
					item.PartialCheckboxes,
					item.DeferredCheckboxes,
				)
			}
		})
	}
}
