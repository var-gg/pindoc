import type { TaskFlowRow, TaskFlowStage } from "../api/client";

export const TASK_FLOW_STAGES: TaskFlowStage[] = ["ready", "blocked", "done", "other"];

export type TaskFlowStageGroup = {
  stage: TaskFlowStage;
  rows: TaskFlowRow[];
};

export type TaskFlowProjectGroup = {
  projectSlug: string;
  rows: TaskFlowRow[];
};

export type TaskFlowSummary = {
  total: number;
  ready: number;
  blocked: number;
  done: number;
  projects: number;
};

export function taskFlowSummary(rows: TaskFlowRow[]): TaskFlowSummary {
  const projects = new Set<string>();
  let ready = 0;
  let blocked = 0;
  let done = 0;
  for (const row of rows) {
    projects.add(row.project_slug);
    if (row.stage === "ready") ready++;
    if (row.stage === "blocked") blocked++;
    if (row.stage === "done") done++;
  }
  return {
    total: rows.length,
    ready,
    blocked,
    done,
    projects: projects.size,
  };
}

export function groupTaskFlowByStage(rows: TaskFlowRow[]): TaskFlowStageGroup[] {
  return TASK_FLOW_STAGES.map((stage) => ({
    stage,
    rows: rows.filter((row) => row.stage === stage),
  })).filter((group) => group.rows.length > 0);
}

export function groupTaskFlowByProject(rows: TaskFlowRow[]): TaskFlowProjectGroup[] {
  const groups = new Map<string, TaskFlowRow[]>();
  for (const row of rows) {
    const list = groups.get(row.project_slug) ?? [];
    list.push(row);
    groups.set(row.project_slug, list);
  }
  return Array.from(groups, ([projectSlug, projectRows]) => ({
    projectSlug,
    rows: projectRows,
  })).sort((a, b) => a.projectSlug.localeCompare(b.projectSlug));
}

export function taskFlowRowsForCurrentFilter(
  rows: TaskFlowRow[],
  projectScope: "current" | "visible" | "list",
  visibleCurrentProjectSlugs: ReadonlySet<string>,
): TaskFlowRow[] {
  if (projectScope !== "current" || visibleCurrentProjectSlugs.size === 0) {
    return rows;
  }
  return rows.filter((row) => visibleCurrentProjectSlugs.has(row.slug));
}
