import type { SearchHit } from "../api/client";
import { localizedAreaName } from "./areaLocale";

type TFn = (key: string, ...args: Array<string | number>) => string;

export type CmdKNavigationKey = "ArrowDown" | "ArrowUp" | "Home" | "End" | "PageDown" | "PageUp";

export type CmdKItemKind = "commit" | "artifact";

export type CmdKSection<T extends { kind: CmdKItemKind }> = {
  kind: "commits" | "artifacts";
  labelKey: "cmdk.commits_section_label" | "cmdk.artifacts_section_label";
  items: T[];
  startIndex: number;
};

export type CmdKFocusTarget = "input" | number;

export const CMDK_RELEVANCE_SETTINGS = {
  maxDistance: 0.7,
} as const;

type CmdKRelevanceSettings = {
  maxDistance?: number;
};

export function cmdkRelevantHits(
  hits: SearchHit[],
  settings: CmdKRelevanceSettings = CMDK_RELEVANCE_SETTINGS,
): SearchHit[] {
  const maxDistance = settings.maxDistance ?? CMDK_RELEVANCE_SETTINGS.maxDistance;
  return hits.filter((hit) => Number.isFinite(hit.distance) && hit.distance <= maxDistance);
}

export function cmdkResultMeta(hit: SearchHit, t: TFn): string {
  // The raw embedding distance stays in the API response for ranking and
  // debugging, but the Reader command palette does not expose it.
  const parts = [hit.type, localizedAreaName(t, hit.area_slug, hit.area_slug)];
  const lifecycle = cmdkLifecycleLabel(hit, t);
  if (lifecycle) parts.push(lifecycle);
  if (hit.task_priority) parts.push(hit.task_priority.toUpperCase());
  if (hit.updated_at) parts.push(t("cmdk.updated", hit.updated_at.slice(0, 10)));
  const heading = hit.heading?.trim();
  if (heading && heading !== hit.title) {
    parts.push(heading);
  }
  return parts.join(" · ");
}

export function cmdkNextIndex(current: number, count: number, key: CmdKNavigationKey): number {
  if (count <= 0) return current;
  const bounded = Math.min(Math.max(current, 0), count - 1);
  switch (key) {
    case "ArrowDown":
      return Math.min(bounded + 1, count - 1);
    case "ArrowUp":
      return Math.max(bounded - 1, 0);
    case "Home":
      return 0;
    case "End":
      return count - 1;
    case "PageDown":
      return Math.min(bounded + 5, count - 1);
    case "PageUp":
      return Math.max(bounded - 5, 0);
  }
}

export function cmdkEmptyCopyKey(query: string): "cmdk.hint" | "cmdk.no_hits" {
  return query.trim() ? "cmdk.no_hits" : "cmdk.hint";
}

export function cmdkOptionId(index: number): string {
  return `cmdk-option-${index}`;
}

export function cmdkSections<T extends { kind: CmdKItemKind }>(items: T[]): CmdKSection<T>[] {
  const commits = items.filter((item) => item.kind === "commit");
  const artifacts = items.filter((item) => item.kind === "artifact");
  const sections: CmdKSection<T>[] = [];
  if (commits.length > 0) {
    sections.push({
      kind: "commits",
      labelKey: "cmdk.commits_section_label",
      items: commits,
      startIndex: 0,
    });
  }
  if (artifacts.length > 0) {
    sections.push({
      kind: "artifacts",
      labelKey: "cmdk.artifacts_section_label",
      items: artifacts,
      startIndex: commits.length,
    });
  }
  return sections;
}

export function cmdkTrapTabTarget(
  current: CmdKFocusTarget,
  optionCount: number,
  shiftKey: boolean,
): CmdKFocusTarget | null {
  if (optionCount <= 0) return "input";
  if (shiftKey && current === "input") return optionCount - 1;
  if (!shiftKey && current === optionCount - 1) return "input";
  return null;
}

function cmdkLifecycleLabel(hit: SearchHit, t: TFn): string | null {
  if (hit.type === "Task" && hit.task_status) {
    return taskStatusLabel(hit.task_status, t);
  }
  const status = enumLabel(`artifact.status.${hit.status ?? ""}`, hit.status, t);
  const completeness = enumLabel(`artifact.completeness.${hit.completeness ?? ""}`, hit.completeness, t);
  if (status && completeness) return `${status}/${completeness}`;
  return status || completeness || null;
}

function taskStatusLabel(value: string, t: TFn): string {
  switch (value) {
    case "open":
      return t("tasks.col_open");
    case "claimed_done":
      return t("tasks.col_claimed_done");
    case "blocked":
      return t("tasks.col_blocked");
    case "cancelled":
      return t("tasks.col_cancelled");
    case "missing_status":
      return t("tasks.col_no_status");
    default:
      return value;
  }
}

function enumLabel(key: string, value: string | undefined, t: TFn): string | null {
  if (!value) return null;
  const label = t(key);
  return label === key ? value : label;
}
