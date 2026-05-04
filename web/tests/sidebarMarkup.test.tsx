import { renderToStaticMarkup } from "react-dom/server";
import type { Area, Project } from "../src/api/client";
import { I18nProvider } from "../src/i18n";
import { Sidebar } from "../src/reader/Sidebar";
import { PindocTooltipProvider } from "../src/reader/Tooltip";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function hasNestedButton(html: string): boolean {
  let depth = 0;
  const tagRe = /<\/?button\b[^>]*>/g;
  for (const match of html.matchAll(tagRe)) {
    const tag = match[0] ?? "";
    if (tag.startsWith("</")) {
      depth = Math.max(0, depth - 1);
      continue;
    }
    if (depth > 0) return true;
    depth += 1;
  }
  return false;
}

function project(): Project {
  return {
    id: "project-pindoc",
    slug: "pindoc",
    organization_slug: "default",
    name: "Pindoc",
    primary_language: "en",
    areas_count: 2,
    artifacts_count: 2,
    created_at: "2026-05-04T00:00:00Z",
    current_role: "owner",
  };
}

function area(id: string, slug: string, name: string, parentSlug?: string): Area {
  return {
    id,
    slug,
    name,
    parent_slug: parentSlug,
    is_cross_cutting: false,
    artifact_count: 1,
  };
}

function sidebarHTML(selectedArea: string | null): string {
  return renderToStaticMarkup(
    <PindocTooltipProvider>
      <I18nProvider projectLang="en">
        <Sidebar
          project={project()}
          projectSlug="pindoc"
          orgSlug="default"
          artifacts={[]}
          areas={[
            area("area-alpha", "alpha", "Alpha"),
            area("area-alpha-child", "alpha.child", "Alpha child", "alpha"),
          ]}
          types={[]}
          agents={[]}
          selectedArea={selectedArea}
          onSelectArea={() => undefined}
          selectedType={null}
          onSelectType={() => undefined}
          open
          showTemplates={false}
          onToggleTemplates={() => undefined}
          showProjectSwitcher={false}
          projectCreateAllowed={false}
        />
      </I18nProvider>
    </PindocTooltipProvider>,
  );
}

function testAreaRowsDoNotNestButtons(): void {
  const html = sidebarHTML("alpha.child");
  assert(!hasNestedButton(html), "Sidebar area tree must not render button-in-button markup");
  assert(html.includes('class="side-item__toggle"'), "parent area should expose a chevron button");
  assert(html.includes('aria-label="Collapse Alpha"'), "selected child should auto-expand its parent");
  assert(html.includes("Alpha child"), "auto-expanded selected child should remain visible");
}

testAreaRowsDoNotNestButtons();
