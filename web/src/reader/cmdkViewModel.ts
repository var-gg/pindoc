import type { SearchHit } from "../api/client";
import { localizedAreaName } from "./areaLocale";
import { projectSurfacePath } from "../readerRoutes";

type TFn = (key: string, ...args: Array<string | number>) => string;

export type CmdKNavigationKey = "ArrowDown" | "ArrowUp" | "Home" | "End" | "PageDown" | "PageUp";

export type CmdKItemKind = "commit" | "artifact";
export type CmdKArtifactScope = "current" | "global";

export type CmdKSection<T extends { kind: CmdKItemKind; artifactScope?: CmdKArtifactScope }> = {
  kind: "commits" | "current_artifacts" | "global_artifacts";
  labelKey:
    | "cmdk.commits_section_label"
    | "cmdk.current_project_section_label"
    | "cmdk.global_artifacts_section_label";
  items: T[];
  startIndex: number;
};

export type CmdKFocusTarget = "input" | number;

export type CmdKCommitLookup<TRepo> = {
  repo: TRepo;
  available: boolean;
  commit?: string;
  summary?: string;
};

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

export function cmdkOtherProjectHits(
  hits: SearchHit[],
  currentProjectSlug: string,
  currentOrgSlug: string,
): SearchHit[] {
  return hits.filter((hit) => {
    const projectSlug = hit.project_slug || currentProjectSlug;
    const orgSlug = hit.org_slug || currentOrgSlug;
    return projectSlug !== currentProjectSlug || orgSlug !== currentOrgSlug;
  });
}

export function cmdkProjectChip(
  hit: SearchHit,
  fallbackProjectSlug: string,
  fallbackOrgSlug: string,
): string {
  const projectSlug = (hit.project_slug || fallbackProjectSlug).trim();
  const orgSlug = (hit.org_slug || fallbackOrgSlug).trim();
  return orgSlug ? `${projectSlug} · ${orgSlug}` : projectSlug;
}

export function cmdkArtifactPath(
  hit: SearchHit,
  fallbackProjectSlug: string,
  fallbackOrgSlug: string,
): string {
  return projectSurfacePath(
    hit.project_slug || fallbackProjectSlug,
    "wiki",
    hit.slug,
    hit.org_slug || fallbackOrgSlug,
  );
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

export function cmdkSections<T extends { kind: CmdKItemKind; artifactScope?: CmdKArtifactScope }>(items: T[]): CmdKSection<T>[] {
  const commits = items.filter((item) => item.kind === "commit");
  const currentArtifacts = items.filter((item) => item.kind === "artifact" && item.artifactScope !== "global");
  const globalArtifacts = items.filter((item) => item.kind === "artifact" && item.artifactScope === "global");
  const sections: CmdKSection<T>[] = [];
  if (commits.length > 0) {
    sections.push({
      kind: "commits",
      labelKey: "cmdk.commits_section_label",
      items: commits,
      startIndex: 0,
    });
  }
  if (currentArtifacts.length > 0) {
    sections.push({
      kind: "current_artifacts",
      labelKey: "cmdk.current_project_section_label",
      items: currentArtifacts,
      startIndex: commits.length,
    });
  }
  if (globalArtifacts.length > 0) {
    sections.push({
      kind: "global_artifacts",
      labelKey: "cmdk.global_artifacts_section_label",
      items: globalArtifacts,
      startIndex: commits.length + currentArtifacts.length,
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

export function cmdkCommitRows<TRepo>(
  lookups: Array<CmdKCommitLookup<TRepo>>,
  query: string,
): Array<{ kind: "commit"; repo: TRepo; sha: string; summary?: string }> {
  return lookups
    .filter((lookup) => lookup.available)
    .map((lookup) => ({
      kind: "commit",
      repo: lookup.repo,
      sha: lookup.commit || query,
      summary: lookup.summary,
    }));
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
