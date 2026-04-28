import type { SearchHit } from "../api/client";
import { localizedAreaName } from "./areaLocale";

type TFn = (key: string, ...args: Array<string | number>) => string;

export function cmdkResultMeta(hit: SearchHit, t: TFn): string {
  // The raw embedding distance stays in the API response for ranking and
  // debugging, but the Reader command palette does not expose it.
  return `${hit.type} · ${localizedAreaName(t, hit.area_slug, hit.area_slug)}`;
}
