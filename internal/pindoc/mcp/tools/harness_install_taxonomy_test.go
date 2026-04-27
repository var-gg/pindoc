package tools

import (
	"strings"
	"testing"
)

func TestRenderPindocMDAreaTaxonomySection(t *testing.T) {
	body := renderPindocMD("Pindoc", "project-123", "pindoc", "ko", "ko", "test", true)

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
	body := renderPindocMD("Pindoc", "project-123", "pindoc", "ko", "ko", "test", true)

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
	body := renderPindocMD("Pindoc", "project-123", "pindoc", "ko", "ko", "test", true)

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
	body := renderPindocMD("Pindoc", "project-123", "pindoc", "ko", "ko", "test", true)

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

func TestRenderPindocMDFrontmatter(t *testing.T) {
	body := renderPindocMD("Pindoc", "ddbbfa62-4511-41c2-af07-110f534fb6e4", "pindoc", "ko", "ko", "test", true)

	for _, want := range []string{
		"---\nproject_slug: pindoc",
		"project_id: ddbbfa62-4511-41c2-af07-110f534fb6e4",
		"locale: ko",
		"schema_version: 1",
		"---\n\n# PINDOC.md",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered PINDOC.md missing frontmatter %q", want)
		}
	}
}

func TestRenderPindocMDTaskLifecycleSection(t *testing.T) {
	body := renderPindocMD("Pindoc", "project-123", "pindoc", "ko", "ko", "test", true)

	for _, want := range []string{
		"## Section 12 — Task lifecycle (chip / parallel work)",
		"### Before spawn",
		"### During chip work",
		"### After chip merge to main",
		"### If interrupted / abandoned",
		"### Retroactive policy",
		"task_meta.status=\"open\"",
		"status: \"claimed_done\"",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered PINDOC.md missing Section 12 guidance %q", want)
		}
	}
}

func TestRenderPindocMDApplicableRulesSection(t *testing.T) {
	body := renderPindocMD("Pindoc", "project-123", "pindoc", "ko", "ko", "test", true)

	for _, want := range []string{
		"## Section X — Applicable Rules Mechanism",
		"artifact_meta.rule_severity",
		"applies_to_areas",
		"context.for_task",
		"applicable_rules",
		"Cross-cutting areas",
		"binding means pause for explicit confirmation",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered PINDOC.md missing Applicable Rules guidance %q", want)
		}
	}
}

func TestRenderPindocMDOmitsTaskLifecycleSection(t *testing.T) {
	body := renderPindocMD("Pindoc", "project-123", "pindoc", "ko", "ko", "test", false)

	if strings.Contains(body, "## Section 12 — Task lifecycle") {
		t.Fatalf("rendered PINDOC.md should omit Section 12 when includeSection12=false")
	}
}

func TestHarnessInstallRegisterSeparationAnchorGuide(t *testing.T) {
	message := harnessInstallMessage(styleSnippetMarkerBegin)
	for _, want := range []string{"immediately after the main", "agent-guidance H2", "# AGENTS.md instructions"} {
		if !strings.Contains(message, want) {
			t.Fatalf("harness install instructions missing anchor guide %q", want)
		}
	}
}
