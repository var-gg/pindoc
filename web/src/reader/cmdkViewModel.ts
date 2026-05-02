import type { SearchHit } from "../api/client";
import { localizedAreaName } from "./areaLocale";

type TFn = (key: string, ...args: Array<string | number>) => string;

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
