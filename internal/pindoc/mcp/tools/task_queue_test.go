package tools

import (
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
