package tools

import (
	"strings"
	"testing"
)

func TestRenderPindocMDAreaTaxonomySection(t *testing.T) {
	body := renderPindocMD("Pindoc", "pindoc", "ko", "test")

	for _, want := range []string{
		"## Area taxonomy",
		"strategy",
		"context",
		"experience",
		"system",
		"operations",
		"governance",
		"cross-cutting",
		"docs/19-area-taxonomy.md",
		"Type=Decision",
		"Decision artifacts must land in their subject area",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered PINDOC.md missing %q", want)
		}
	}
	for _, old := range []string{"decisions area", "mcp-surface", "embedding-layer", "data-model"} {
		if strings.Contains(body, old) {
			t.Fatalf("rendered PINDOC.md should not carry legacy valid-area examples: %q", old)
		}
	}
}

func TestRenderPindocMDSlugGuidance(t *testing.T) {
	body := renderPindocMD("Pindoc", "pindoc", "ko", "test")

	for _, want := range []string{
		"## Slug 규약",
		"25 runes or fewer",
		"Unicode slugs are allowed",
		"Slugs are immutable",
		"task-reader-toc-sidecar-이전",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered PINDOC.md missing slug guidance %q", want)
		}
	}
	if strings.Contains(body, "영문 권장") {
		t.Fatalf("rendered PINDOC.md should not recommend English-only slugs")
	}
}

func TestRenderPindocMDProjectLanguageGuidance(t *testing.T) {
	body := renderPindocMD("Pindoc", "pindoc", "ko", "test")

	for _, want := range []string{
		"primary_language=en|ko|ja",
		"ask the user",
		"do not infer or default",
		"immutable after create",
		"recreating the project",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered PINDOC.md missing project language guidance %q", want)
		}
	}
}

func TestRenderPindocMDBodyVsGraphEdgesGuidance(t *testing.T) {
	body := renderPindocMD("Pindoc", "pindoc", "ko", "test")

	for _, want := range []string{
		"## Body vs graph edges",
		"Relationships belong in the",
		"relates_to input field",
		"## 연관",
		"SECTION_DUPLICATES_EDGES",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered PINDOC.md missing edge guidance %q", want)
		}
	}
}
