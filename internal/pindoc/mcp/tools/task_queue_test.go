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
		{"verified", "verified", true},
		{"blocked", "blocked", true},
		{"cancelled", "cancelled", true},
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
// wide aggregate maps go to nil so json encoding skips them, while items
// + totals + notice survive. Default (compact=false) leaves every field
// alone — backward-compat for existing callers.
func TestApplyTaskQueueCompact(t *testing.T) {
	mkOut := func() taskQueueOutput {
		return taskQueueOutput{
			SourceSemantics: taskQueueSemantics,
			StatusFilter:    "pending",
			TotalCount:      10,
			PendingCount:    7,
			StatusCounts:    map[string]int{"open": 5, taskStatusMissing: 2, "claimed_done": 3},
			AreaCounts:      map[string]int{"ui": 4, "mcp": 6},
			PriorityCounts:  map[string]int{"p2": 3},
			WarningCounts:   map[string]int{"TASK_STATUS_MISSING": 2},
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
		if out.TotalCount != 10 || out.PendingCount != 7 {
			t.Fatalf("compact must preserve totals; got total=%d pending=%d", out.TotalCount, out.PendingCount)
		}
		if len(out.Items) != 1 {
			t.Fatalf("compact must preserve items; got %d", len(out.Items))
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
		for _, kept := range []string{`"total_count":10`, `"pending_count":7`, `"compact":true`} {
			if !strings.Contains(body, kept) {
				t.Fatalf("compact JSON missing %s; got %s", kept, body)
			}
		}
	})

	t.Run("nil receiver is safe", func(t *testing.T) {
		applyTaskQueueCompact(nil, true) // should not panic
	})
}
