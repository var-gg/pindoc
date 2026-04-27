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

func TestHarnessInstallRejectsInvalidResponseFormat(t *testing.T) {
	if _, err := normalizeHarnessResponseFormat("compact"); err == nil {
		t.Fatalf("invalid response_format should return an error")
	}
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
