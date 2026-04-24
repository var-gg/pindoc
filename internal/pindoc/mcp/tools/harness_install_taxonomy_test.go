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
