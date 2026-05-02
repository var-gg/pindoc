package main

import (
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/artifact/preflight"
)

func TestRenderAcceptanceVerbReport(t *testing.T) {
	report := renderAcceptanceVerbReport([]acceptanceVerbReportRow{
		{
			ProjectSlug: "pindoc",
			TaskSlug:    "task-1",
			Revision:    3,
			Finding: preflight.AcceptanceVerbFinding{
				LineNumber:    12,
				CheckboxIndex: 2,
				Verb:          "확인한다",
				Text:          "기존 라우팅을 확인한다.",
				ExampleAfter:  "기존 /p/{project}/wiki/{slug} 라우팅 regression test가 통과한다.",
			},
		},
	})
	for _, want := range []string{
		"# Acceptance Verb Lint Retro-pass Report",
		"Total findings: 1",
		"`task-1`",
		"확인한다",
		"regression test가 통과한다",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRenderAcceptanceVerbReportEmpty(t *testing.T) {
	report := renderAcceptanceVerbReport(nil)
	if !strings.Contains(report, "Total findings: 0") || !strings.Contains(report, "No forbidden action verbs") {
		t.Fatalf("empty report missing summary:\n%s", report)
	}
}
