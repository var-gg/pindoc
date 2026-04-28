import type { SearchHit } from "../src/api/client";
import { cmdkResultMeta } from "../src/reader/cmdkViewModel";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function hit(overrides: Partial<SearchHit> = {}): SearchHit {
  return {
    artifact_id: "a1",
    slug: "reader-qa",
    type: "Task",
    title: "Reader QA",
    area_slug: "ui",
    snippet: "match",
    distance: 0.445,
    ...overrides,
  };
}

function t(key: string): string {
  const labels: Record<string, string> = {
    "area.ui": "UI",
  };
  return labels[key] ?? key;
}

function testCmdKMetaHidesRawDistance(): void {
  const meta = cmdkResultMeta(hit(), t);

  assertEqual(meta, "Task · UI", "CmdK result meta");
  assert(!meta.includes("distance"), "result meta should hide the distance token");
  assert(!/\b0\.\d+\b/.test(meta), "result meta should hide raw floating-point scores");
}

function testCmdKMetaDoesNotDependOnDistanceForDisplay(): void {
  const first = cmdkResultMeta(hit({ distance: 0.1 }), t);
  const second = cmdkResultMeta(hit({ distance: 0.9 }), t);

  assertEqual(first, second, "visible metadata should not change when only distance changes");
}

testCmdKMetaHidesRawDistance();
testCmdKMetaDoesNotDependOnDistanceForDisplay();
