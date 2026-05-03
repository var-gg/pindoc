import type { SearchHit } from "../src/api/client";
import {
  CMDK_RELEVANCE_SETTINGS,
  cmdkRelevantHits,
  cmdkResultMeta,
} from "../src/reader/cmdkViewModel";
import ko from "../src/i18n/ko.json";
import en from "../src/i18n/en.json";

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

function testCmdKFiltersIrrelevantNonsenseQueryHits(): void {
  const filtered = cmdkRelevantHits([
    hit({ artifact_id: "nonsense-1", slug: "nonsense-1", distance: 0.7291 }),
    hit({ artifact_id: "nonsense-2", slug: "nonsense-2", distance: 0.81 }),
  ]);

  assertEqual(filtered.length, 0, "irrelevant CmdK hits should be hidden");
}

function testCmdKKeepsRelevantTopResults(): void {
  const filtered = cmdkRelevantHits([
    hit({ artifact_id: "visibility-1", slug: "visibility-1", distance: 0.58 }),
    hit({ artifact_id: "visibility-2", slug: "visibility-2", distance: 0.64 }),
    hit({ artifact_id: "too-far", slug: "too-far", distance: 0.71 }),
  ]);

  assertEqual(filtered.map((item) => item.slug).join(","), "visibility-1,visibility-2", "relevant CmdK hits should remain in ranked order");
}

function testCmdKRelevanceCutoffIsCentralized(): void {
  assertEqual(CMDK_RELEVANCE_SETTINGS.maxDistance, 0.7, "CmdK relevance cutoff setting");
  assertEqual(cmdkRelevantHits([hit({ distance: 0.7 })]).length, 1, "cutoff should be inclusive");
}

function testCmdKEmptyCopyExistsInBothLocales(): void {
  assertEqual(ko["cmdk.no_hits"], "일치하는 문서가 없습니다.", "KO CmdK empty copy");
  assertEqual(en["cmdk.no_hits"], "No matching artifacts.", "EN CmdK empty copy");
}

testCmdKMetaHidesRawDistance();
testCmdKMetaDoesNotDependOnDistanceForDisplay();
testCmdKMetaShowsSectionContextWhenAvailable();
testCmdKMetaShowsLifecycleSignalsWhenAvailable();
testCmdKMetaShowsArtifactStatusSignalsWhenAvailable();
testCmdKFiltersIrrelevantNonsenseQueryHits();
testCmdKKeepsRelevantTopResults();
testCmdKRelevanceCutoffIsCentralized();
testCmdKEmptyCopyExistsInBothLocales();
