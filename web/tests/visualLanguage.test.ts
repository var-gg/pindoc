import { topLevelVisualAreaSlugs, visualLanguage, type VisualEntry } from "../src/reader/visualLanguage";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function assertEntryComplete(entry: VisualEntry, path: string): void {
  assert(entry.label_en.length > 0, `${path}.label_en must be filled`);
  assert(entry.label_ko.length > 0, `${path}.label_ko must be filled`);
  assert(entry.description_en.length > 0, `${path}.description_en must be filled`);
  assert(entry.description_ko.length > 0, `${path}.description_ko must be filled`);
  assert(entry.icon.length > 0, `${path}.icon must be filled`);
  assert(entry.color_token.startsWith("--"), `${path}.color_token must be a CSS custom property name`);
}

function testTokenCounts(): void {
  assertEqual(Object.keys(visualLanguage.types).length, 12, "Type token count");
  assertEqual(Object.keys(visualLanguage.areas).length, 14, "Area token count");
  assertEqual(Object.keys(visualLanguage.relations).length, 5, "Relation token count");
  assertEqual(Object.keys(visualLanguage.pins).length, 6, "Pin token count");
  assertEqual(Object.keys(visualLanguage.meta_enums).length, 6, "Meta enum group count");
  assertEqual(Object.keys(visualLanguage.quick_actions).length, 5, "Quick action count");
  assertEqual(topLevelVisualAreaSlugs.length, 8, "Top-level area count");
}

function testCopyCompleteness(): void {
  Object.entries(visualLanguage.types).forEach(([key, entry]) => assertEntryComplete(entry, `types.${key}`));
  Object.entries(visualLanguage.areas).forEach(([key, entry]) => assertEntryComplete(entry, `areas.${key}`));
  Object.entries(visualLanguage.relations).forEach(([key, entry]) => assertEntryComplete(entry, `relations.${key}`));
  Object.entries(visualLanguage.pins).forEach(([key, entry]) => assertEntryComplete(entry, `pins.${key}`));
  Object.entries(visualLanguage.quick_actions).forEach(([key, entry]) => assertEntryComplete(entry, `quick_actions.${key}`));
  Object.entries(visualLanguage.meta_enums).forEach(([groupKey, group]) => {
    Object.entries(group).forEach(([value, entry]) => assertEntryComplete(entry, `meta_enums.${groupKey}.${value}`));
  });
}

function testAreaHierarchy(): void {
  for (const slug of topLevelVisualAreaSlugs) {
    const area = visualLanguage.areas[slug];
    assert(area.fixed === true, `${slug} must be fixed`);
    assert(area.signature_color === true, `${slug} must be a signature-color top-level area`);
    assert(area.parent === null, `${slug} must not have a parent`);
  }
  for (const slug of ["security", "privacy", "accessibility", "reliability", "observability", "localization"] as const) {
    const area = visualLanguage.areas[slug];
    assert(area.fixed === true, `${slug} must be fixed`);
    assert(area.signature_color === false, `${slug} must not be a top-level signature color`);
    assert(area.parent === "cross-cutting", `${slug} must sit below cross-cutting`);
  }
}

testTokenCounts();
testCopyCompleteness();
testAreaHierarchy();
