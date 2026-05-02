package main

import (
	"strings"
	"testing"
)

func TestRenderOutcomeMissingReport(t *testing.T) {
	report := renderOutcomeMissingReport([]outcomeMissingReportRow{
		{
			ProjectSlug: "pindoc",
			TaskSlug:    "task-1",
			Revision:    4,
			Codes:       []string{"OUTCOME_SECTION_MISSING", "OUTCOME_COMMIT_MISSING"},
			Prompt:      "Read `pindoc/task-1` and append an Outcome section.",
		},
	})
	for _, want := range []string{
		"# claim_done Outcome Missing Retro-pass Report",
		"Total findings: 1",
		"`task-1`",
		"OUTCOME_SECTION_MISSING",
		"append an Outcome section",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestOutcomeCommitExemptForReport(t *testing.T) {
	if !outcomeCommitExemptForReport([]byte(`{"outcome_commit_exempt":true}`), nil) {
		t.Fatal("task_meta outcome_commit_exempt should exempt")
	}
	if !outcomeCommitExemptForReport(nil, []byte(`{"code_coordinate_exempt":true}`)) {
		t.Fatal("artifact_meta code_coordinate_exempt should exempt")
	}
	if outcomeCommitExemptForReport([]byte(`{"outcome_commit_exempt":false}`), nil) {
		t.Fatal("false should not exempt")
	}
}
