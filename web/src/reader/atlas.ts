import type { ArtifactRef } from "../api/client";
import type { GraphLayoutEdge } from "./graphSvg";

// AtlasAreaSummary describes one area's slice of the project. The
// numbers here are pre-computed against the user-visible artifact list
// (post-filter), so the atlas reflects exactly what the graph surface
// is showing. Cross-area edges live separately in AtlasData.
export type AtlasAreaSummary = {
  area_slug: string;
  artifact_count: number;
  type_breakdown: Record<string, number>;
  // recent_activity_score: 0..1, where 1 = at least one artifact in the
  // area updated within RECENCY_FULL_DAYS, decaying linearly to 0 at
  // RECENCY_FLOOR_DAYS. Areas with no recent updates score 0.
  recent_activity_score: number;
};

export type AtlasCrossAreaEdge = {
  from: string;
  to: string;
  count: number;
};

export type AtlasData = {
  areas: AtlasAreaSummary[];
  cross_area_edges: AtlasCrossAreaEdge[];
  total_artifacts: number;
};

const RECENCY_FULL_DAYS = 7;
const RECENCY_FLOOR_DAYS = 30;

// computeAtlas builds the aggregate snapshot consumed by AtlasMinimap.
// edges may carry relation info but we only count cross-area edges
// here — anything inside a single area is invisible at the atlas
// scale and the corridor count must always be unique-pair so the
// minimap doesn't double-count direction-pair (A→B, B→A).
export function computeAtlas(
  list: ArtifactRef[],
  edges: Array<GraphLayoutEdge & { relation?: string }>,
): AtlasData {
  const byID = new Map(list.map((a) => [a.id, a]));
  const areaMap = new Map<string, AtlasAreaSummary & { latestUpdate: number }>();
  const now = Date.now();
  for (const a of list) {
    let entry = areaMap.get(a.area_slug);
    if (!entry) {
      entry = {
        area_slug: a.area_slug,
        artifact_count: 0,
        type_breakdown: {},
        recent_activity_score: 0,
        latestUpdate: -Infinity,
      };
      areaMap.set(a.area_slug, entry);
    }
    entry.artifact_count++;
    entry.type_breakdown[a.type] = (entry.type_breakdown[a.type] ?? 0) + 1;
    const ts = new Date(a.updated_at).getTime();
    if (Number.isFinite(ts) && ts > entry.latestUpdate) {
      entry.latestUpdate = ts;
    }
  }
  for (const entry of areaMap.values()) {
    if (!Number.isFinite(entry.latestUpdate)) continue;
    const ageDays = (now - entry.latestUpdate) / (1000 * 60 * 60 * 24);
    if (ageDays <= RECENCY_FULL_DAYS) {
      entry.recent_activity_score = 1;
    } else if (ageDays <= RECENCY_FLOOR_DAYS) {
      // Linear decay between full and floor.
      entry.recent_activity_score =
        1 - (ageDays - RECENCY_FULL_DAYS) / (RECENCY_FLOOR_DAYS - RECENCY_FULL_DAYS);
    } else {
      entry.recent_activity_score = 0;
    }
  }

  // Cross-area edges keyed by ordered pair so A→B and B→A collapse into
  // one corridor entry. Direction is preserved as the lexicographically
  // smaller area slug first to keep the entry deterministic.
  const corridorMap = new Map<string, AtlasCrossAreaEdge>();
  for (const e of edges) {
    const sourceNode = byID.get(e.source);
    const targetNode = byID.get(e.target);
    if (!sourceNode || !targetNode) continue;
    if (sourceNode.area_slug === targetNode.area_slug) continue;
    const a = sourceNode.area_slug;
    const b = targetNode.area_slug;
    const [from, to] = a < b ? [a, b] : [b, a];
    const key = `${from}::${to}`;
    const entry = corridorMap.get(key);
    if (entry) {
      entry.count++;
    } else {
      corridorMap.set(key, { from, to, count: 1 });
    }
  }

  const areas = Array.from(areaMap.values())
    .sort((a, b) => a.area_slug.localeCompare(b.area_slug))
    .map((a) => ({
      area_slug: a.area_slug,
      artifact_count: a.artifact_count,
      type_breakdown: a.type_breakdown,
      recent_activity_score: a.recent_activity_score,
    }));
  const cross_area_edges = Array.from(corridorMap.values()).sort((a, b) => {
    if (a.from !== b.from) return a.from.localeCompare(b.from);
    return a.to.localeCompare(b.to);
  });

  return { areas, cross_area_edges, total_artifacts: list.length };
}

export type AtlasPlacement = {
  cx: number;
  cy: number;
  r: number;
};

// computeAtlasPlacement lays out atlas areas as bubbles on a circular
// dial. Bubble radius scales with the area's artifact share; angular
// position is determined by area_slug alphabetical order so the same
// project always shows the same layout. Cross-area corridors are then
// drawn as chords between the bubble centres.
export function computeAtlasPlacement(
  data: AtlasData,
  options?: { canvasWidth?: number; canvasHeight?: number; minR?: number; maxR?: number },
): { placements: Map<string, AtlasPlacement>; viewBox: { x: number; y: number; w: number; h: number } } {
  const canvasWidth = options?.canvasWidth ?? 240;
  const canvasHeight = options?.canvasHeight ?? 200;
  const minR = options?.minR ?? 8;
  const maxR = options?.maxR ?? 26;
  const placements = new Map<string, AtlasPlacement>();
  const cx0 = canvasWidth / 2;
  const cy0 = canvasHeight / 2;
  if (data.areas.length === 0) {
    return {
      placements,
      viewBox: { x: 0, y: 0, w: canvasWidth, h: canvasHeight },
    };
  }
  const dialRadius = Math.min(canvasWidth, canvasHeight) * 0.36;
  const total = data.total_artifacts || 1;
  // Sort by alphabetic area_slug for layout stability.
  const sorted = [...data.areas].sort((a, b) => a.area_slug.localeCompare(b.area_slug));
  for (let i = 0; i < sorted.length; i++) {
    const area = sorted[i];
    const angle = -Math.PI / 2 + (i * 2 * Math.PI) / sorted.length;
    const share = area.artifact_count / total;
    // Bubble radius scales by sqrt(share) so areas with 4× artifacts
    // look 2× wider, not 4× — visual area is what the eye reads.
    const r = Math.min(maxR, Math.max(minR, minR + Math.sqrt(share) * (maxR - minR) * 2));
    placements.set(area.area_slug, {
      cx: cx0 + Math.cos(angle) * dialRadius,
      cy: cy0 + Math.sin(angle) * dialRadius,
      r,
    });
  }
  return {
    placements,
    viewBox: { x: 0, y: 0, w: canvasWidth, h: canvasHeight },
  };
}
