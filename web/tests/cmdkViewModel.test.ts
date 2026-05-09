import type { SearchHit } from "../src/api/client";
import {
  cmdkArtifactPath,
  CMDK_RELEVANCE_SETTINGS,
  cmdkCommitRows,
  cmdkEmptyCopyKey,
  cmdkNextIndex,
  cmdkOptionId,
  cmdkOtherProjectHits,
  cmdkProjectChip,
  cmdkRelevantHits,
  cmdkResultDetailMeta,
  cmdkResultMeta,
  cmdkSections,
  cmdkTrapTabTarget,
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
    project_slug: "pindoc",
    org_slug: "default",
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

function testCmdKDetailMetaOmitsChipSignals(): void {
  const meta = cmdkResultDetailMeta(hit({
    task_status: "open",
    task_priority: "p1",
    heading: "Acceptance Criteria",
    updated_at: "2026-05-02T12:00:00Z",
  }), t);

  assertEqual(meta, "Open · P1 · Updated 2026-05-02 · Acceptance Criteria", "CmdK detail meta should omit type and area");
}

function testCmdKDetailMetaCanBeEmpty(): void {
  assertEqual(cmdkResultDetailMeta(hit(), t), "", "CmdK detail meta should be optional when only chip signals exist");
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
  assertEqual(ko["cmdk.dialog_label"], "명령 팔레트", "KO CmdK dialog label");
  assertEqual(en["cmdk.dialog_label"], "Command palette", "EN CmdK dialog label");
  assertEqual(ko["cmdk.commits_section_label"], "커밋", "KO CmdK commit section label");
  assertEqual(en["cmdk.commits_section_label"], "Commits", "EN CmdK commit section label");
  assertEqual(ko["cmdk.scope_all"], "전체 문서", "KO CmdK all scope label");
  assertEqual(en["cmdk.scope_all"], "All docs", "EN CmdK all scope label");
  assertEqual(ko["cmdk.scope_tasks"], "Task", "KO CmdK task scope label");
  assertEqual(en["cmdk.scope_tasks"], "Tasks", "EN CmdK task scope label");
  assertEqual(ko["cmdk.current_project_section_label"], "현재 프로젝트", "KO CmdK current project section label");
  assertEqual(en["cmdk.current_project_section_label"], "Current project", "EN CmdK current project section label");
  assertEqual(ko["cmdk.global_artifacts_section_label"], "다른 프로젝트", "KO CmdK global section label");
  assertEqual(en["cmdk.global_artifacts_section_label"], "Other projects", "EN CmdK global section label");
}

function testCmdKEmptyCopyUsesTrimmedQuery(): void {
  assertEqual(cmdkEmptyCopyKey("   "), "cmdk.hint", "whitespace-only query should show the initial hint");
  assertEqual(cmdkEmptyCopyKey("nonsense"), "cmdk.no_hits", "non-empty query should show no hits");
}

function testCmdKKeyboardJumpNavigation(): void {
  assertEqual(cmdkNextIndex(1, 8, "Home"), 0, "Home should move to first result");
  assertEqual(cmdkNextIndex(1, 8, "End"), 7, "End should move to last result");
  assertEqual(cmdkNextIndex(2, 8, "PageDown"), 7, "PageDown should jump five rows and clamp");
  assertEqual(cmdkNextIndex(7, 8, "PageUp"), 2, "PageUp should jump five rows");
  assertEqual(cmdkNextIndex(0, 8, "ArrowUp"), 0, "ArrowUp should clamp at first result");
  assertEqual(cmdkNextIndex(7, 8, "ArrowDown"), 7, "ArrowDown should clamp at last result");
  assertEqual(cmdkNextIndex(3, 0, "End"), 3, "navigation should be a no-op with no results");
}

function testCmdKFocusTrapTargets(): void {
  assertEqual(cmdkTrapTabTarget("input", 0, false), "input", "Tab should stay on input with no results");
  assertEqual(cmdkTrapTabTarget("input", 0, true), "input", "Shift+Tab should stay on input with no results");
  assertEqual(cmdkTrapTabTarget("input", 3, true), 2, "Shift+Tab from input should wrap to the last result");
  assertEqual(cmdkTrapTabTarget(2, 3, false), "input", "Tab from last result should wrap to input");
  assertEqual(cmdkTrapTabTarget(1, 3, false), null, "middle result Tab should use native focus order");
}

function testCmdKSectionsSeparateCommitsAndArtifacts(): void {
  const artifact = { kind: "artifact" as const, artifactScope: "current" as const, hit: hit({ artifact_id: "a2" }) };
  const globalArtifact = { kind: "artifact" as const, artifactScope: "global" as const, hit: hit({ artifact_id: "a3", project_slug: "sister" }) };
  const commit = { kind: "commit" as const, repo: { id: "repo-a", name: "repo-a", default_branch: "main" }, sha: "abcdef1" };
  const sections = cmdkSections([commit, artifact, globalArtifact]);

  assertEqual(sections.length, 3, "commit and artifact rows should render in separate sections");
  assertEqual(sections[0]?.labelKey, "cmdk.commits_section_label", "commit section label key");
  assertEqual(sections[0]?.startIndex, 0, "commit section start index");
  assertEqual(sections[1]?.labelKey, "cmdk.current_project_section_label", "current artifact section label key");
  assertEqual(sections[1]?.startIndex, 1, "artifact section start index");
  assertEqual(sections[2]?.labelKey, "cmdk.global_artifacts_section_label", "global artifact section label key");
  assertEqual(sections[2]?.startIndex, 2, "global artifact section start index");
  assertEqual(cmdkSections([artifact]).length, 1, "empty commit section should be hidden");
  assertEqual(cmdkSections([artifact])[0]?.labelKey, "cmdk.current_project_section_label", "artifact-only label key");
  assertEqual(cmdkOptionId(3), "cmdk-option-3", "stable option id");
}

function testCmdKGlobalHitsExcludeCurrentProject(): void {
  const filtered = cmdkOtherProjectHits([
    hit({ artifact_id: "current", slug: "current", project_slug: "pindoc", org_slug: "default" }),
    hit({ artifact_id: "sister", slug: "sister", project_slug: "pindoc-sample", org_slug: "default" }),
  ], "pindoc", "default");

  assertEqual(filtered.map((item) => item.slug).join(","), "sister", "global CmdK hits should exclude current project rows");
}

function testCmdKProjectChipUsesHitProjectScope(): void {
  const label = cmdkProjectChip(
    hit({ project_slug: "pindoc-sample", org_slug: "default" }),
    "pindoc",
    "default",
  );

  assertEqual(label, "pindoc-sample · default", "CmdK project chip should show project and org");
}

function testCmdKArtifactPathUsesHitProjectScope(): void {
  const path = cmdkArtifactPath(
    hit({ project_slug: "pindoc-sample", org_slug: "default", slug: "sister-doc" }),
    "pindoc",
    "default",
  );

  assertEqual(path, "/default/p/pindoc-sample/wiki/sister-doc", "CmdK artifact path should use hit project scope");
}

function testCmdKCommitRowsUseEveryMatchingRepo(): void {
  const rows = cmdkCommitRows([
    {
      repo: { id: "frontend", name: "frontend", default_branch: "main" },
      available: true,
      commit: "abcdef1234567890",
      summary: "front fix",
    },
    {
      repo: { id: "backend", name: "backend", default_branch: "main" },
      available: true,
      commit: "abcdef9999999999",
      summary: "back fix",
    },
    {
      repo: { id: "docs", name: "docs", default_branch: "main" },
      available: false,
    },
  ], "abcdef1");

  assertEqual(rows.length, 2, "only matching repos should produce commit rows");
  assertEqual(rows.map((row) => row.repo.id).join(","), "frontend,backend", "matching repos should keep individual rows");
  assertEqual(rows.map((row) => row.sha).join(","), "abcdef1234567890,abcdef9999999999", "commit rows should use resolved SHAs");
  assertEqual(cmdkCommitRows([{ repo: { id: "docs" }, available: false }], "abcdef1").length, 0, "no repo match should hide the commit candidate");
}

testCmdKMetaHidesRawDistance();
testCmdKMetaDoesNotDependOnDistanceForDisplay();
testCmdKMetaShowsSectionContextWhenAvailable();
testCmdKMetaShowsLifecycleSignalsWhenAvailable();
testCmdKMetaShowsArtifactStatusSignalsWhenAvailable();
testCmdKDetailMetaOmitsChipSignals();
testCmdKDetailMetaCanBeEmpty();
testCmdKFiltersIrrelevantNonsenseQueryHits();
testCmdKKeepsRelevantTopResults();
testCmdKRelevanceCutoffIsCentralized();
testCmdKEmptyCopyExistsInBothLocales();
testCmdKEmptyCopyUsesTrimmedQuery();
testCmdKKeyboardJumpNavigation();
testCmdKFocusTrapTargets();
testCmdKSectionsSeparateCommitsAndArtifacts();
testCmdKGlobalHitsExcludeCurrentProject();
testCmdKProjectChipUsesHitProjectScope();
testCmdKArtifactPathUsesHitProjectScope();
testCmdKCommitRowsUseEveryMatchingRepo();
