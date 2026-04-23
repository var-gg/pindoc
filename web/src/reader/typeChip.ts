// typeChip — resolves an artifact.type string into the CSS class pair that
// paints the type-chip in its semantic OKLCH band. Unknown types fall back
// to the neutral .type-chip default so new Tier B types render cleanly
// before a dedicated hue is adopted.
//
// The variant suffix is the lowercased type with non-letters stripped so
// "APIEndpoint" → "apiendpoint", matching the class names declared in
// web/src/styles/reader.css (Task task-type-palette-binding).

const KNOWN_VARIANTS = new Set([
  "decision",
  "analysis",
  "task",
  "debug",
  "glossary",
  "flow",
  "tc",
  "feature",
  "apiendpoint",
  "screen",
  "datamodel",
  "verificationreport",
]);

export function typeChipClass(type: string | undefined | null): string {
  if (!type) return "type-chip";
  const variant = type.toLowerCase().replace(/[^a-z]/g, "");
  if (!KNOWN_VARIANTS.has(variant)) return "type-chip";
  return `type-chip type-chip--${variant}`;
}
