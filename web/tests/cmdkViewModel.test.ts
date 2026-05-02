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

function t(key: string, ...args: Array<string | number>): string {
  const labels: Record<string, string> = {
    "area.ui": "UI",
    "cmdk.updated": `Updated ${args[0]}`,
    "tasks.col_open": "Open",
    "artifact.status.published": "Published",
    "artifact.completeness.partial": "Partial",
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

function testCmdKMetaShowsSectionContextWhenAvailable(): void {
  const meta = cmdkResultMeta(hit({ heading: "Acceptance Criteria" }), t);

  assertEqual(meta, "Task · UI · Acceptance Criteria", "CmdK result meta with heading context");
}

function testCmdKMetaShowsLifecycleSignalsWhenAvailable(): void {
  const meta = cmdkResultMeta(hit({
    task_status: "open",
    task_priority: "p1",
    updated_at: "2026-05-02T12:00:00Z",
  }), t);

  assertEqual(meta, "Task · UI · Open · P1 · Updated 2026-05-02", "CmdK result meta with task signals");
}

function testCmdKMetaShowsArtifactStatusSignalsWhenAvailable(): void {
  const meta = cmdkResultMeta(hit({
    type: "Decision",
    status: "published",
    completeness: "partial",
    updated_at: "2026-05-02T12:00:00Z",
  }), t);

  assertEqual(meta, "Decision · UI · Published/Partial · Updated 2026-05-02", "CmdK result meta with artifact signals");
}

testCmdKMetaHidesRawDistance();
testCmdKMetaDoesNotDependOnDistanceForDisplay();
testCmdKMetaShowsSectionContextWhenAvailable();
testCmdKMetaShowsLifecycleSignalsWhenAvailable();
testCmdKMetaShowsArtifactStatusSignalsWhenAvailable();
