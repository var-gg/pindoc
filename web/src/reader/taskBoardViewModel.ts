import type { ArtifactRef } from "../api/client";

export const TASK_REVIEW_BACKLOG_THRESHOLD = 50;
export const TASK_REVIEW_INITIAL_LIMIT = 12;
export const TASK_DEFAULT_PAGE_SIZE = 50;

const TASK_STATUS_COLUMNS = ["open", "claimed_done", "blocked", "cancelled"] as const;
const PRIORITY_RANK: Record<string, number> = {
  p0: 0,
  p1: 1,
  p2: 2,
  p3: 3,
};

export type TaskBoardSummary = {
  reviewQueue: number;
  open: number;
  blocked: number;
  recentDone: number;
};

export function groupTasksByStatus(list: ArtifactRef[]): Map<string, ArtifactRef[]> {
  const groups = new Map<string, ArtifactRef[]>();
  for (const id of TASK_STATUS_COLUMNS) groups.set(id, []);
  groups.set("no_status", []);
  for (const a of list) {
    const status = a.task_meta?.status;
    if (status && groups.has(status)) {
      groups.get(status)!.push(a);
    } else {
      groups.get("no_status")!.push(a);
    }
  }
  for (const [columnId, items] of groups) {
    groups.set(columnId, sortTaskColumnItems(columnId, items));
  }
  return groups;
}

export function visibleTaskGroups(
  groups: Map<string, ArtifactRef[]>,
  visibleCounts: Record<string, number>,
): Map<string, ArtifactRef[]> {
  const visible = new Map<string, ArtifactRef[]>();
  for (const [columnId, items] of groups) {
    visible.set(columnId, items.slice(0, visibleCounts[columnId] ?? taskColumnInitialLimit(columnId, items.length)));
  }
  return visible;
}

export function countPendingTasks(list: ArtifactRef[]): number {
  return list.filter((a) => {
    const status = a.task_meta?.status;
    return !status || status === "open";
  }).length;
}

export function orderedTaskList(groups: Map<string, ArtifactRef[]>): ArtifactRef[] {
  return [
    ...TASK_STATUS_COLUMNS.flatMap((columnId) => groups.get(columnId) ?? []),
    ...(groups.get("no_status") ?? []),
  ];
}

export function taskColumnInitialLimit(columnId: string, total: number): number {
  if (columnId === "claimed_done" && total >= TASK_REVIEW_BACKLOG_THRESHOLD) {
    return TASK_REVIEW_INITIAL_LIMIT;
  }
  return TASK_DEFAULT_PAGE_SIZE;
}

export function taskColumnPageSize(columnId: string): number {
  return columnId === "claimed_done" ? TASK_REVIEW_INITIAL_LIMIT : TASK_DEFAULT_PAGE_SIZE;
}

export function taskBoardSummary(groups: Map<string, ArtifactRef[]>): TaskBoardSummary {
  const reviewQueue = groups.get("claimed_done")?.length ?? 0;
  const open = (groups.get("open")?.length ?? 0) + (groups.get("no_status")?.length ?? 0);
  const blocked = groups.get("blocked")?.length ?? 0;
  return {
    reviewQueue,
    open,
    blocked,
    recentDone: reviewQueue,
  };
}

function sortTaskColumnItems(columnId: string, items: ArtifactRef[]): ArtifactRef[] {
  const copy = items.slice();
  copy.sort((a, b) => {
    if (columnId === "claimed_done" || columnId === "open" || columnId === "blocked") {
      const prio = priorityRank(a) - priorityRank(b);
      if (prio !== 0) return prio;
    }
    return updatedAtMs(b) - updatedAtMs(a);
  });
  return copy;
}

function priorityRank(a: ArtifactRef): number {
  const priority = a.task_meta?.priority ?? "";
  return PRIORITY_RANK[priority] ?? 99;
}

function updatedAtMs(a: ArtifactRef): number {
  const ts = Date.parse(a.updated_at);
  return Number.isFinite(ts) ? ts : 0;
}
