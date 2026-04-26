// localizedAreaName — UI-locale translation of an Area name / slug.
// When the i18n bundle has an `area.{slug}` key, return its translation;
// otherwise fall back to the caller-provided label so custom Areas (or
// slugs the bundle hasn't seen yet) still render cleanly. Detection of
// a missing key relies on the fact that web/src/i18n/index.tsx returns
// the key itself when no translation exists.
//
// Shared between Sidebar (renders Area rows) and CmdK (renders search
// hits), so both surfaces display the same user-visible label for the
// same slug — Task task-area-name-i18n acceptance on Cmd+K consistency.
import type { Area } from "../api/client";

export const TOP_LEVEL_AREA_ORDER = [
  "strategy",
  "context",
  "experience",
  "system",
  "operations",
  "governance",
  "cross-cutting",
  "misc",
  "_unsorted",
] as const;

// docs/19-area-taxonomy.md fixes the 8 concern skeleton areas plus the
// 6 admitted cross-cutting children. Other sub-areas are project-specific
// promotion outcomes and render as user-promoted in the Sidebar.
export const FIXED_TAXONOMY_AREA_SLUGS = [
  "strategy",
  "context",
  "experience",
  "system",
  "operations",
  "governance",
  "cross-cutting",
  "misc",
  "security",
  "privacy",
  "accessibility",
  "reliability",
  "observability",
  "localization",
] as const;

const topLevelRank = new Map<string, number>(
  TOP_LEVEL_AREA_ORDER.map((slug, index) => [slug, index]),
);

const fixedTaxonomyAreaSlugs = new Set<string>(FIXED_TAXONOMY_AREA_SLUGS);

export function localizedAreaName(
  t: (key: string) => string,
  slug: string,
  fallback: string,
): string {
  const key = `area.${slug}`;
  const translated = t(key);
  return translated === key ? fallback : translated;
}

export function isFixedTaxonomyArea(slug: string): boolean {
  return fixedTaxonomyAreaSlugs.has(slug);
}

export function compareAreas(a: Area, b: Area): number {
  return areaRank(a) - areaRank(b) || a.slug.localeCompare(b.slug);
}

function areaRank(area: Area): number {
  if (area.parent_slug) return Number.MAX_SAFE_INTEGER;
  return topLevelRank.get(area.slug) ?? TOP_LEVEL_AREA_ORDER.length;
}
