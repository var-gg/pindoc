import type { TaskFlowRow } from "../src/api/client";
import {
  groupTaskFlowByProject,
  groupTaskFlowByStage,
  taskCardKeyAction,
  taskFlowRowsForCurrentFilter,
  taskFlowSummary,
} from "../src/reader/taskFlowViewModel";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual<T>(actual: T, expected: T, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function row(slug: string, project: string, stage: TaskFlowRow["stage"], ordinal: number): TaskFlowRow {
  return {
    project_slug: project,
    artifact_id: slug,
    slug,
    title: slug,
    area_slug: "ui",
    status: stage === "done" ? "claimed_done" : stage === "blocked" ? "blocked" : "open",
    priority: "p2",
    stage,
    ordinal,
    readiness: stage === "blocked" ? "blocked" : stage === "done" ? "done" : "ready",
    updated_at: new Date(Date.UTC(2026, 4, 1, 0, ordinal)).toISOString(),
    agent_ref: `pindoc://${slug}`,
    human_url: `/p/${project}/wiki/${slug}`,
  };
}

function testStageGroupingPreservesServerSequence(): void {
  const rows = [
    row("ready-2", "pindoc", "ready", 2),
    row("blocked-1", "pindoc", "blocked", 3),
    row("ready-1", "pindoc", "ready", 1),
    row("done-1", "pindoc", "done", 4),
  ];
  const groups = groupTaskFlowByStage(rows);
  assertEqual(groups[0].stage, "ready", "ready stage appears first");
  assertEqual(groups[0].rows[0].slug, "ready-2", "stage grouping keeps API order");
  assertEqual(groups[0].rows[1].slug, "ready-1", "stage grouping does not re-sort by due date or slug");
  assertEqual(groups[1].stage, "blocked", "blocked stage appears after ready");
  assertEqual(groups[2].stage, "done", "done stage appears after blocked");
}

function testProjectGroupingAndSummary(): void {
  const rows = [
    row("a", "beta", "ready", 1),
    row("b", "alpha", "blocked", 2),
    row("c", "alpha", "done", 3),
  ];
  const summary = taskFlowSummary(rows);
  assertEqual(summary.total, 3, "total count");
  assertEqual(summary.ready, 1, "ready count");
  assertEqual(summary.blocked, 1, "blocked count");
  assertEqual(summary.done, 1, "done count");
  assertEqual(summary.projects, 2, "project count");

  const projects = groupTaskFlowByProject(rows);
  assertEqual(projects[0].projectSlug, "alpha", "project groups sort by slug");
  assertEqual(projects[0].rows.length, 2, "project group rows");
}

function testCurrentScopeFilterUsesCurrentProjectVisibleSetOnly(): void {
  const rows = [
    row("visible", "pindoc", "ready", 1),
    row("hidden-by-badge", "pindoc", "ready", 2),
  ];
  const filtered = taskFlowRowsForCurrentFilter(rows, "current", new Set(["visible"]));
  assertEqual(filtered.length, 1, "current scope applies local artifact filter");
  assertEqual(filtered[0].slug, "visible", "keeps visible slug");
  assert(
    taskFlowRowsForCurrentFilter(rows, "visible", new Set(["visible"])).length === 2,
    "cross-project scope is not compressed by current-project badge filters",
  );
}

function testTaskFlowCardKeyboardOpenParity(): void {
  assertEqual(taskCardKeyAction("Enter", true, false), "open", "Shift+Enter opens detail");
  assertEqual(taskCardKeyAction("o", false, false), "open", "o opens detail");
  assertEqual(taskCardKeyAction("O", false, false), "open", "O opens detail");
}

function testTaskFlowCardKeyboardSelectParity(): void {
  assertEqual(taskCardKeyAction("Enter", false, false), "select", "Enter selects preview");
  assertEqual(taskCardKeyAction(" ", false, false), "select", "Space selects preview");
}

function testTaskFlowCardKeyboardIgnoresNestedControls(): void {
  assertEqual(taskCardKeyAction("Enter", true, true), null, "nested link Shift+Enter is not intercepted");
  assertEqual(taskCardKeyAction("o", false, true), null, "nested control O is not intercepted");
  assertEqual(taskCardKeyAction(" ", false, true), null, "nested control Space is not intercepted");
}

testStageGroupingPreservesServerSequence();
testProjectGroupingAndSummary();
testCurrentScopeFilterUsesCurrentProjectVisibleSetOnly();
testTaskFlowCardKeyboardOpenParity();
testTaskFlowCardKeyboardSelectParity();
testTaskFlowCardKeyboardIgnoresNestedControls();
