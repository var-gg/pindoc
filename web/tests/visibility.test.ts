import {
  VISIBILITY_TIERS,
  canEditArtifactVisibility,
  isVisibilityTier,
  normalizeVisibilityTier,
  visibilityChipClass,
  visibilityDescriptionKey,
  visibilityLabelKey,
} from "../src/reader/visibility";

function assertEqual<T>(actual: T, expected: T, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function testVisibilityNormalization(): void {
  assertEqual(normalizeVisibilityTier("public"), "public", "public is preserved");
  assertEqual(normalizeVisibilityTier(" PRIVATE "), "private", "visibility trim/lowercase");
  assertEqual(normalizeVisibilityTier("deleted"), "org", "unknown visibility falls back to org");
  assertEqual(normalizeVisibilityTier(undefined), "org", "missing visibility falls back to org");
}

function testVisibilityKeysAndClasses(): void {
  assertEqual(VISIBILITY_TIERS.join(","), "public,org,private", "tier order matches dropdown order");
  assert(isVisibilityTier("org"), "org is a visibility tier");
  assert(!isVisibilityTier("owner"), "owner is not a visibility tier");
  assertEqual(visibilityLabelKey("private"), "artifact.visibility.private", "label key");
  assertEqual(visibilityDescriptionKey("public"), "artifact.visibility.public_desc", "description key");
  assertEqual(visibilityChipClass("org"), "visibility-chip visibility-chip--org", "chip class");
}

function testVisibilityEditPermission(): void {
  assertEqual(canEditArtifactVisibility(true), true, "explicit true enables edit");
  assertEqual(canEditArtifactVisibility(false), false, "false disables edit");
  assertEqual(canEditArtifactVisibility(undefined), false, "missing flag disables edit");
}

testVisibilityNormalization();
testVisibilityKeysAndClasses();
testVisibilityEditPermission();
