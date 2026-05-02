import { transformPindocAssetURL } from "../src/reader/assetReferences";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testPindocAssetReferenceTransformsToBlobRoute(): void {
  assertEqual(
    transformPindocAssetURL("pindoc-asset://asset-123", "pindoc"),
    "/api/p/pindoc/assets/asset-123/blob",
    "asset ref blob route",
  );
}

function testPindocAssetReferenceRequiresProjectContext(): void {
  assertEqual(
    transformPindocAssetURL("pindoc-asset://asset-123"),
    "",
    "asset ref without project context",
  );
}

testPindocAssetReferenceTransformsToBlobRoute();
testPindocAssetReferenceRequiresProjectContext();
