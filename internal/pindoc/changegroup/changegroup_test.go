package changegroup

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestGroupRowsPriorityKeysAndImportance(t *testing.T) {
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	rows := []RevisionRow{
		row("pindoc", "a1", "ui", "codex", "update Today screen", now, `{"bulk_op_id":"bulk-1"}`, `{"verification_state":"verified"}`),
		row("pindoc", "a2", "mcp", "codex", "update context tool", now.Add(time.Minute), `{"bulk_op_id":"bulk-1"}`, `{"verification_state":"unverified"}`),
	}
	groups := GroupRows(rows, Options{Limit: 10})
	if len(groups) != 1 {
		t.Fatalf("groups=%d want 1", len(groups))
	}
	g := groups[0]
	if g.GroupingKey.Kind != "bulk_op_id" || g.GroupingKey.Confidence != "high" {
		t.Fatalf("grouping key = %#v", g.GroupingKey)
	}
	if g.ArtifactCount != 2 || g.Importance.Level != "high" {
		t.Fatalf("importance/artifacts = %#v / %d", g.Importance, g.ArtifactCount)
	}
	if g.VerificationState != "unverified" {
		t.Fatalf("verification = %q", g.VerificationState)
	}
}

func TestGroupRowsTypeCountsUniqueArtifacts(t *testing.T) {
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	firstTaskRev := row("pindoc", "task-1", "ui", "codex", "edit task", now, `{"bulk_op_id":"bulk-types"}`, `{}`)
	secondTaskRev := firstTaskRev
	secondTaskRev.RevisionID = "task-1-r2"
	secondTaskRev.RevisionNumber = 2
	secondTaskRev.CommitMsg = "edit task again"
	secondTaskRev.CreatedAt = now.Add(time.Minute)
	decisionRev := row("pindoc", "decision-1", "ui", "codex", "edit decision", now.Add(2*time.Minute), `{"bulk_op_id":"bulk-types"}`, `{}`)
	decisionRev.ArtifactType = "Decision"
	groups := GroupRows([]RevisionRow{firstTaskRev, secondTaskRev, decisionRev}, Options{Limit: 10})
	if len(groups) != 1 {
		t.Fatalf("groups=%d want 1", len(groups))
	}
	got := groups[0].TypeCounts
	want := []TypeCount{{Type: "Decision", Count: 1}, {Type: "Task", Count: 1}}
	if len(got) != len(want) {
		t.Fatalf("type counts = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("type counts = %#v, want %#v", got, want)
		}
	}
	if groups[0].FirstArtifact == nil || groups[0].FirstArtifact.Slug != "task-1" {
		t.Fatalf("first artifact = %#v, want task-1", groups[0].FirstArtifact)
	}
	if len(groups[0].Artifacts) != 2 || groups[0].Artifacts[0].Slug != "task-1" || groups[0].Artifacts[1].Slug != "decision-1" {
		t.Fatalf("artifacts = %#v, want task-1 then decision-1", groups[0].Artifacts)
	}
}

func TestGroupRowsFallbackLowConfidence(t *testing.T) {
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	groups := GroupRows([]RevisionRow{
		row("pindoc", "a1", "ui", "codex", "standalone edit", now, `{}`, `{}`),
	}, Options{})
	if len(groups) != 1 {
		t.Fatalf("groups=%d want 1", len(groups))
	}
	if groups[0].GroupingKey.Kind != "author_time_window" || groups[0].GroupingKey.Confidence != "low" {
		t.Fatalf("fallback key = %#v", groups[0].GroupingKey)
	}
}

func TestGroupRowsStandaloneExceptionKinds(t *testing.T) {
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	msgs := []string{
		"migration added reader_watermarks",
		"import markdown bundle",
		"seed starter areas",
		"code sync from harness",
		"agent propagation reconcile",
		"maintenance docker daemon health",
	}
	rows := make([]RevisionRow, 0, len(msgs))
	for i, msg := range msgs {
		rows = append(rows, row("pindoc", string(rune('a'+i)), "mechanisms", "codex", msg, now.Add(time.Duration(i)*time.Hour), `{}`, `{}`))
	}
	groups := GroupRows(rows, Options{Limit: 20})
	if len(groups) != len(msgs) {
		t.Fatalf("standalone exception groups=%d want %d", len(groups), len(msgs))
	}
	kinds := map[string]bool{}
	for _, g := range groups {
		kinds[g.GroupKind] = true
	}
	if !kinds["maintenance"] || !kinds["auto_sync"] {
		t.Fatalf("expected maintenance and auto_sync kinds, got %v", kinds)
	}
}

func TestSummaryCacheKeyAndPrompt(t *testing.T) {
	keyA := SummaryCacheKey("local", "pindoc", "u1", 1, 4, "ko", "all")
	keyB := SummaryCacheKey("local", "pindoc", "u1", 1, 5, "ko", "all")
	if keyA == keyB {
		t.Fatalf("cache key should change when max revision changes")
	}
	prompt := SourceBoundPrompt([]Group{{GroupID: "g1", GroupKind: "human_trigger", CommitSummary: "x"}}, "ko", 5)
	if strings.Contains(prompt, "body_markdown") || !strings.Contains(prompt, "ChangeGroups") {
		t.Fatalf("prompt should be compact/source-bound: %s", prompt)
	}
}

func TestCompactNoGroupsOneGroupAndCap(t *testing.T) {
	if got := Compact(nil, 5); len(got) != 0 {
		t.Fatalf("no groups compact len=%d", len(got))
	}
	groups := []Group{
		{GroupID: "g1", GroupKind: "human_trigger", CommitSummary: "one", ArtifactCount: 1},
		{GroupID: "g2", GroupKind: "auto_sync", CommitSummary: "two", ArtifactCount: 2},
	}
	one := Compact(groups[:1], 5)
	if len(one) != 1 || one[0].GroupID != "g1" {
		t.Fatalf("one group compact = %#v", one)
	}
	capped := Compact(groups, 1)
	if len(capped) != 1 || capped[0].GroupID != "g1" {
		t.Fatalf("capped compact = %#v", capped)
	}
}

func TestTrimTextRuneSafeBoundaries(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{
			name: "english",
			in:   "abcdefghijklmnopqrstuvwxyz",
			max:  10,
			want: "abcdefghij...",
		},
		{
			name: "korean-byte-boundary",
			in:   "한글ABC",
			max:  4,
			want: "한글AB...",
		},
		{
			name: "mixed",
			in:   "abc한글def",
			max:  5,
			want: "abc한글...",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := trimText(tc.in, tc.max)
			if got != tc.want {
				t.Fatalf("trimText() = %q, want %q", got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("trimText() returned invalid UTF-8: %q", got)
			}
		})
	}
}

func TestSummarizeCommitsKeepsUTF8Valid(t *testing.T) {
	msg := strings.Repeat("가", 120)
	got := summarizeCommits([]string{msg}, 1, 1)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("summary should preserve ellipsis on truncation: %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("summary returned invalid UTF-8: %q", got)
	}
}

func row(project, artifactID, area, author, msg string, created time.Time, source, meta string) RevisionRow {
	return RevisionRow{
		ProjectSlug:      project,
		RevisionID:       artifactID + "-r1",
		ArtifactID:       artifactID,
		ArtifactSlug:     artifactID,
		ArtifactTitle:    artifactID,
		ArtifactType:     "Task",
		AreaSlug:         area,
		RevisionNumber:   1,
		AuthorID:         author,
		CommitMsg:        msg,
		SourceSessionRef: json.RawMessage(source),
		ArtifactMeta:     json.RawMessage(meta),
		CreatedAt:        created,
	}
}
