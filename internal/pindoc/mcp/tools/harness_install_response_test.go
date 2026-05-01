package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHarnessInstallDefaultResponseFormatIsFull(t *testing.T) {
	format, err := normalizeHarnessResponseFormat("")
	if err != nil {
		t.Fatalf("normalize default response format: %v", err)
	}
	if format != harnessResponseFormatFull {
		t.Fatalf("default response_format = %q, want %q", format, harnessResponseFormatFull)
	}
}

func TestHarnessInstallFullResponseIncludesBodiesAndETags(t *testing.T) {
	body := "pindoc body"
	styleSnippet := "style snippet"
	out := testHarnessInstallOutput(body, styleSnippet)

	applyHarnessResponseFormat(&out, harnessInstallInput{}, harnessResponseFormatFull, body, styleSnippet)

	if out.Body != body || out.PindocMdContent != body {
		t.Fatalf("full response omitted PINDOC.md body")
	}
	if out.StyleSnippet != styleSnippet {
		t.Fatalf("full response omitted style snippet")
	}
	if out.ContentETag == "" || out.StyleSnippetETag == "" {
		t.Fatalf("full response must include etags")
	}
	if out.ContentOmitted || out.StyleSnippetOmitted {
		t.Fatalf("full response should not mark payloads omitted without matching etags")
	}
}

func TestHarnessInstallFileOnlyOmitsBodiesAndKeepsEtags(t *testing.T) {
	body := strings.Repeat("pindoc body\n", 80)
	styleSnippet := strings.Repeat("style snippet\n", 20)

	full := testHarnessInstallOutput(body, styleSnippet)
	applyHarnessResponseFormat(&full, harnessInstallInput{}, harnessResponseFormatFull, body, styleSnippet)
	fullJSON := mustMarshalHarnessInstallOutput(t, full)

	fileOnly := testHarnessInstallOutput(body, styleSnippet)
	applyHarnessResponseFormat(&fileOnly, harnessInstallInput{ResponseFormat: harnessResponseFormatFileOnly}, harnessResponseFormatFileOnly, body, styleSnippet)
	fileOnlyJSON := mustMarshalHarnessInstallOutput(t, fileOnly)

	if fileOnly.Body != "" || fileOnly.PindocMdContent != "" || fileOnly.StyleSnippet != "" {
		t.Fatalf("file_only response should omit body fields and style snippet")
	}
	for _, forbidden := range []string{`"body":`, `"pindoc_md_content":`, `"style_snippet":`} {
		if strings.Contains(string(fileOnlyJSON), forbidden) {
			t.Fatalf("file_only JSON contains omitted field %s: %s", forbidden, fileOnlyJSON)
		}
	}
	for _, want := range []string{`"content_etag":`, `"style_snippet_etag":`, `"content_omitted":true`, `"style_snippet_omitted":true`} {
		if !strings.Contains(string(fileOnlyJSON), want) {
			t.Fatalf("file_only JSON missing %s: %s", want, fileOnlyJSON)
		}
	}
	if len(fileOnlyJSON) >= len(fullJSON) {
		t.Fatalf("file_only response should be smaller than full response: file_only=%d full=%d", len(fileOnlyJSON), len(fullJSON))
	}
}

func TestHarnessInstallMatchingETagsOmitBodiesInFullMode(t *testing.T) {
	body := "pindoc body"
	styleSnippet := "style snippet"
	in := harnessInstallInput{
		IfContentETag:      " " + harnessETag(body) + " ",
		IfStyleSnippetETag: harnessETag(styleSnippet),
	}
	out := testHarnessInstallOutput(body, styleSnippet)

	applyHarnessResponseFormat(&out, in, harnessResponseFormatFull, body, styleSnippet)

	if out.Body != "" || out.PindocMdContent != "" {
		t.Fatalf("matching content etag should omit PINDOC.md body")
	}
	if out.StyleSnippet != "" {
		t.Fatalf("matching style snippet etag should omit style snippet")
	}
	if !out.ContentOmitted || !out.StyleSnippetOmitted {
		t.Fatalf("matching etags should mark payloads omitted")
	}
	if out.ResponseFormat != harnessResponseFormatFull {
		t.Fatalf("etag omission should preserve response_format=full, got %q", out.ResponseFormat)
	}
}

func TestHarnessInstallDriftCheckInSync(t *testing.T) {
	body := renderPindocMD("Pindoc", "00000000-0000-0000-0000-000000000000", "pindoc", "en", "en", "test", true)
	styleSnippet := testHarnessStyleSnippet()
	settings := "@PINDOC.md\n\n" + styleSnippet
	out := testHarnessInstallOutput(body, styleSnippet)

	applyHarnessDriftCheck(&out, harnessInstallInput{
		CurrentPindocMD:          &body,
		CurrentAgentSettingsBody: &settings,
	}, harnessResponseFormatFull, body, styleSnippet)

	if out.DriftStatus != "in_sync" || out.InSync == nil || !*out.InSync {
		t.Fatalf("drift status = %q in_sync=%v", out.DriftStatus, out.InSync)
	}
	if len(out.DriftedSections) != 0 || len(out.SuggestedWriteTargets) != 0 {
		t.Fatalf("in_sync should not return drift guidance: sections=%+v targets=%+v", out.DriftedSections, out.SuggestedWriteTargets)
	}
}

func TestHarnessInstallDriftCheckMissingInputs(t *testing.T) {
	body := renderPindocMD("Pindoc", "00000000-0000-0000-0000-000000000000", "pindoc", "en", "en", "test", true)
	styleSnippet := testHarnessStyleSnippet()
	settings := "@PINDOC.md\n\n" + styleSnippet
	out := testHarnessInstallOutput(body, styleSnippet)

	applyHarnessDriftCheck(&out, harnessInstallInput{
		CurrentAgentSettingsBody: &settings,
	}, harnessResponseFormatFull, body, styleSnippet)

	if out.DriftStatus != "missing" || out.InSync == nil || *out.InSync {
		t.Fatalf("missing drift status = %q in_sync=%v", out.DriftStatus, out.InSync)
	}
	if len(out.Missing) != 1 || out.Missing[0] != "PINDOC.md" {
		t.Fatalf("missing = %+v, want PINDOC.md", out.Missing)
	}
	if len(out.SuggestedWriteTargets) != 1 || out.SuggestedWriteTargets[0].Path != "PINDOC.md" {
		t.Fatalf("missing suggested targets = %+v", out.SuggestedWriteTargets)
	}
}

func TestHarnessInstallDriftCheckSessionBootstrapDrift(t *testing.T) {
	body := renderPindocMD("Pindoc", "00000000-0000-0000-0000-000000000000", "pindoc", "en", "en", "test", true)
	styleSnippet := testHarnessStyleSnippet()
	staleBody := strings.Replace(body, "pindoc.task.queue(across_projects=true)", "pindoc.task.queue", 1)
	staleSnippet := strings.Replace(styleSnippet, "artifact 본문", "artifact body", 1)
	settings := "@PINDOC.md\n\n" + staleSnippet
	out := testHarnessInstallOutput(body, styleSnippet)

	applyHarnessDriftCheck(&out, harnessInstallInput{
		CurrentPindocMD:          &staleBody,
		CurrentAgentSettingsBody: &settings,
	}, harnessResponseFormatFull, body, styleSnippet)

	if out.DriftStatus != "drift" || out.InSync == nil || *out.InSync {
		t.Fatalf("drift status = %q in_sync=%v", out.DriftStatus, out.InSync)
	}
	if !hasHarnessDriftSection(out.DriftedSections, "PINDOC.md", "Pre-flight Check") {
		t.Fatalf("expected Pre-flight Check drift in %+v", out.DriftedSections)
	}
	if !hasHarnessDriftSection(out.DriftedSections, "agent_settings", "register_separation_snippet") {
		t.Fatalf("expected style snippet drift in %+v", out.DriftedSections)
	}
	if !hasHarnessWriteTarget(out.SuggestedWriteTargets, "PINDOC.md", "pindoc_md_content") {
		t.Fatalf("expected PINDOC.md write target in %+v", out.SuggestedWriteTargets)
	}
	if !hasHarnessWriteTarget(out.SuggestedWriteTargets, "CLAUDE.md | AGENTS.md | .cursorrules", "style_snippet") {
		t.Fatalf("expected style snippet write target in %+v", out.SuggestedWriteTargets)
	}
}

func TestHarnessInstallDriftCheckFileOnlyStillReturnsGuidance(t *testing.T) {
	body := renderPindocMD("Pindoc", "00000000-0000-0000-0000-000000000000", "pindoc", "en", "en", "test", true)
	styleSnippet := testHarnessStyleSnippet()
	staleBody := strings.Replace(body, "across_projects=true", "across_projects=false", 1)
	out := testHarnessInstallOutput(body, styleSnippet)

	applyHarnessDriftCheck(&out, harnessInstallInput{
		CurrentPindocMD: &staleBody,
	}, harnessResponseFormatFileOnly, body, styleSnippet)

	if out.DriftStatus != "missing" {
		t.Fatalf("file_only drift status = %q, want missing due absent agent settings", out.DriftStatus)
	}
	if !hasHarnessWriteTargetRequiringBody(out.SuggestedWriteTargets, "PINDOC.md", "pindoc_md_content") {
		t.Fatalf("file_only should mark PINDOC.md target as requiring body: %+v", out.SuggestedWriteTargets)
	}
}

func TestHarnessInstallRejectsInvalidResponseFormat(t *testing.T) {
	if _, err := normalizeHarnessResponseFormat("compact"); err == nil {
		t.Fatalf("invalid response_format should return an error")
	}
}

func testHarnessStyleSnippet() string {
	return styleSnippetMarkerBegin + "\n" + styleSnippetBodyKO + "\n" + styleSnippetMarkerEnd
}

func hasHarnessDriftSection(sections []HarnessDriftedSection, target, sectionContains string) bool {
	for _, section := range sections {
		if section.Target == target && strings.Contains(section.Section, sectionContains) {
			return true
		}
	}
	return false
}

func hasHarnessWriteTarget(targets []HarnessSuggestedWriteTarget, path, sourceField string) bool {
	for _, target := range targets {
		if target.Path == path && target.SourceField == sourceField {
			return true
		}
	}
	return false
}

func hasHarnessWriteTargetRequiringBody(targets []HarnessSuggestedWriteTarget, path, sourceField string) bool {
	for _, target := range targets {
		if target.Path == path && target.SourceField == sourceField && target.RequiresBody {
			return true
		}
	}
	return false
}

func testHarnessInstallOutput(body, styleSnippet string) harnessInstallOutput {
	return harnessInstallOutput{
		SuggestedPath:       "PINDOC.md",
		Body:                body,
		PindocMdContent:     body,
		PindocMdPath:        "PINDOC.md",
		Instructions:        "write body",
		ClaudeMdIncludeLine: "@PINDOC.md",
		StyleSnippet:        styleSnippet,
		StyleSnippetTargets: []string{"CLAUDE.md", "AGENTS.md", ".cursorrules"},
		StyleSnippetMarker:  styleSnippetMarkerBegin,
		Message:             "write body",
	}
}

func mustMarshalHarnessInstallOutput(t *testing.T, out harnessInstallOutput) []byte {
	t.Helper()
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal harness install output: %v", err)
	}
	return raw
}
