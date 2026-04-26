import { isStructureOverlapHeading, structureOverlapSectionsFromBody } from "../src/reader/structureSections";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testHeadingDetection(): void {
  assert(isStructureOverlapHeading("관련 artifacts"), "related artifacts heading should match");
  assert(isStructureOverlapHeading("Dependencies / 선후"), "mixed dependency heading should match");
  assert(!isStructureOverlapHeading("목적"), "narrative purpose heading should not match");
}

function testSectionExtractionSkipsFences(): void {
  const body = [
    "## 목적",
    "",
    "narrative",
    "",
    "```md",
    "## 관련",
    "not a heading",
    "```",
    "",
    "## 관련 artifacts",
    "",
    "- a",
    "",
    "## 범위",
    "",
    "scope",
  ].join("\n");
  const sections = structureOverlapSectionsFromBody(body);
  assertEqual(sections.length, 1, "overlap section count");
  assertEqual(sections[0]?.title, "관련 artifacts", "overlap title");
  assertEqual(sections[0]?.body, "- a", "overlap body");
}

testHeadingDetection();
testSectionExtractionSkipsFences();
