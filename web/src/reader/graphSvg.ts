import { forceCenter, forceLink, forceManyBody, forceSimulation, forceX, forceY } from "d3-force";
import type { ArtifactRef } from "../api/client";
import { visualLabel, visualRelation, visualRelationClass, visualTypeVariant } from "./visualLanguage";

export type EgoNodeKind = "focus" | "hop1" | "bridge";

export type EgoNode = ArtifactRef & {
  ego_kind: EgoNodeKind;
  doi_score?: number;
  doi_reasons?: string[];
};

export type EgoSubgraph = {
  focusId: string;
  nodes: EgoNode[];
  edges: GraphLayoutEdge[];
  // capped is true when the bridge ring was trimmed to keep total
  // visible nodes under the cap. The trimmed ones are exposed via
  // dropped_bridges so the UI can offer "show more bridges" later.
  capped: boolean;
  dropped_bridges: ArtifactRef[];
};

export type StartFocusReason =
  | { kind: "last_focused"; slug: string }
  | { kind: "recent_meaningful"; updated_at: string }
  | { kind: "most_connected"; degree: number }
  | { kind: "fallback"; slug: string };

export type StartFocus = {
  focusId: string;
  reason: StartFocusReason;
};

const EGO_VISIBLE_CAP = 60;
const EGO_BRIDGE_CAP = 24;

export type GraphPoint = {
  x: number;
  y: number;
};

export type GraphViewBox = {
  x: number;
  y: number;
  w: number;
  h: number;
};

export const MINI_GRAPH_CENTER: GraphPoint = { x: 140, y: 100 };
export const FULL_GRAPH_VIEWBOX = { width: 1000, height: 640 } as const;

// FULL_GRAPH_FRAME is the camera-resting frame: a 1000×640 window
// centred on (0, 0). The force layout produces world coordinates around
// the origin, and the camera viewBox slides around inside this same
// world space. Default frame matches the render target so an
// "unfocused" graph looks identical to the legacy fixed layout.
export const FULL_GRAPH_FRAME: GraphViewBox = {
  x: -FULL_GRAPH_VIEWBOX.width / 2,
  y: -FULL_GRAPH_VIEWBOX.height / 2,
  w: FULL_GRAPH_VIEWBOX.width,
  h: FULL_GRAPH_VIEWBOX.height,
};

// FOCUS_FRAME_WIDTH is the camera width when zoomed onto a focus node.
// 540 keeps roughly 6–10 neighbouring nodes legible without blurring
// distant context entirely — depth fog softens the rest.
export const FOCUS_FRAME_WIDTH = 540;
export const FOCUS_FRAME_HEIGHT = (FOCUS_FRAME_WIDTH * FULL_GRAPH_VIEWBOX.height) / FULL_GRAPH_VIEWBOX.width;

export type GraphLayoutEdge = {
  source: string;
  target: string;
};

export type GraphLayoutInput = {
  nodes: ArtifactRef[];
  edges: GraphLayoutEdge[];
};

export type GraphLayout = {
  positions: Map<string, GraphPoint>;
  // bounds describes the world-space rectangle the layout occupies, so
  // an "all of it in frame" camera reset can be computed without
  // re-walking every node.
  bounds: GraphViewBox;
};

// computeForceLayout runs a one-shot d3-force simulation and returns the
// final node coordinates. We pre-seed positions on a deterministic
// sunflower spiral so identical inputs produce visually similar layouts
// across reloads even though d3-force itself is not seeded — combined
// with the high tick budget below, the result is stable enough for
// session-to-session continuity without a custom RNG.
//
// The simulation runs synchronously: alpha is set to 1, ticks 300 times,
// then stops. 200 nodes × 300 ticks measured locally at ~25ms — well
// under any frame budget concern.
export function computeForceLayout(input: GraphLayoutInput): GraphLayout {
  const { nodes, edges } = input;
  if (nodes.length === 0) {
    return { positions: new Map(), bounds: FULL_GRAPH_FRAME };
  }

  // Seed initial positions on a sunflower spiral around the origin so
  // d3-force's first tick has structure to relax instead of starting
  // from a random cloud. Same seed → same simulation trajectory for a
  // given input.
  const goldenAngle = Math.PI * (3 - Math.sqrt(5));
  const radius = Math.min(FULL_GRAPH_VIEWBOX.width, FULL_GRAPH_VIEWBOX.height) * 0.35;
  type SimNode = { id: string; x: number; y: number; vx?: number; vy?: number };
  const simNodes: SimNode[] = nodes.map((n, i) => {
    const progress = Math.sqrt((i + 0.5) / nodes.length);
    const angle = i * goldenAngle - Math.PI / 2;
    return {
      id: n.id,
      x: Math.cos(angle) * radius * progress,
      y: Math.sin(angle) * radius * progress,
    };
  });
  const simEdges = edges
    .filter((e) => simNodes.find((n) => n.id === e.source) && simNodes.find((n) => n.id === e.target))
    .map((e) => ({ source: e.source, target: e.target }));

  const linkForce = forceLink<SimNode, { source: string; target: string }>(simEdges)
    .id((n) => n.id)
    .distance(120)
    .strength(0.4);

  const sim = forceSimulation(simNodes)
    .force("link", linkForce)
    .force("charge", forceManyBody().strength(-180).distanceMax(420))
    .force("center", forceCenter(0, 0))
    .force("x", forceX(0).strength(0.04))
    .force("y", forceY(0).strength(0.04))
    .stop();

  // Manually tick — alpha decays from 1 to ~0.001 over ~300 ticks at
  // the default decay. This is one-shot; we don't run the simulation
  // on a per-frame ticker.
  for (let i = 0; i < 300; i++) sim.tick();
  sim.stop();

  const positions = new Map<string, GraphPoint>();
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const n of simNodes) {
    positions.set(n.id, { x: n.x, y: n.y });
    if (n.x < minX) minX = n.x;
    if (n.x > maxX) maxX = n.x;
    if (n.y < minY) minY = n.y;
    if (n.y > maxY) maxY = n.y;
  }

  // Pad the bounds so node rects don't touch the camera edge.
  const padX = 80;
  const padY = 60;
  const bounds: GraphViewBox = {
    x: minX - padX,
    y: minY - padY,
    w: maxX - minX + padX * 2,
    h: maxY - minY + padY * 2,
  };
  return { positions, bounds };
}

// graphHopDistances returns a depth map keyed by node id: 0 for the
// focus node, 1 for direct neighbours, 2 for next ring, etc. Node ids
// not reachable from the focus get Infinity so depth-fog opacity falls
// to its floor for them. Edges are treated as undirected for distance —
// the user thinks "A is next to B" regardless of arrow direction.
export function graphHopDistances(focus: string, edges: GraphLayoutEdge[]): Map<string, number> {
  const distances = new Map<string, number>();
  if (!focus) return distances;
  const adj = new Map<string, Set<string>>();
  for (const e of edges) {
    if (!adj.has(e.source)) adj.set(e.source, new Set());
    if (!adj.has(e.target)) adj.set(e.target, new Set());
    adj.get(e.source)!.add(e.target);
    adj.get(e.target)!.add(e.source);
  }
  distances.set(focus, 0);
  const queue: string[] = [focus];
  while (queue.length > 0) {
    const here = queue.shift()!;
    const d = distances.get(here)!;
    const neighbours = adj.get(here);
    if (!neighbours) continue;
    for (const next of neighbours) {
      if (distances.has(next)) continue;
      distances.set(next, d + 1);
      queue.push(next);
    }
  }
  return distances;
}

// viewBoxString serialises a frame to the SVG viewBox attribute format
// "x y w h". Pulled out so the same formatter is used by static frame
// rendering and the animation tick.
export function viewBoxString(v: GraphViewBox): string {
  return `${v.x} ${v.y} ${v.w} ${v.h}`;
}

// frameAround centres a frame of the given width on a world point and
// keeps the aspect ratio of FULL_GRAPH_VIEWBOX. Used both for the
// focus zoom (FOCUS_FRAME_WIDTH) and for ambient drift (slight
// per-axis offsets while the width stays at the frame default).
export function frameAround(p: GraphPoint, width = FOCUS_FRAME_WIDTH): GraphViewBox {
  const aspect = FULL_GRAPH_VIEWBOX.height / FULL_GRAPH_VIEWBOX.width;
  const w = width;
  const h = w * aspect;
  return { x: p.x - w / 2, y: p.y - h / 2, w, h };
}

// lerpFrame interpolates between two frames with the given easing. t
// outside [0,1] is clamped — callers can drive linear time and let
// this function do the curve.
export function lerpFrame(a: GraphViewBox, b: GraphViewBox, t: number): GraphViewBox {
  const k = easeOutCubic(Math.max(0, Math.min(1, t)));
  return {
    x: a.x + (b.x - a.x) * k,
    y: a.y + (b.y - a.y) * k,
    w: a.w + (b.w - a.w) * k,
    h: a.h + (b.h - a.h) * k,
  };
}

function easeOutCubic(t: number): number {
  const f = 1 - t;
  return 1 - f * f * f;
}

// graphRadialPositions kept for the mini graph (Sidecar) which still
// expects radial fixed positions for ≤6 neighbours.
export function graphRadialPositions(
  count: number,
  center: GraphPoint = MINI_GRAPH_CENTER,
  radiusX = 96,
  radiusY = 66,
): GraphPoint[] {
  if (count <= 0) return [];
  const start = -Math.PI / 2;
  return Array.from({ length: count }, (_, i) => {
    const angle = start + (i * 2 * Math.PI) / count;
    return {
      x: Math.round(center.x + Math.cos(angle) * radiusX),
      y: Math.round(center.y + Math.sin(angle) * radiusY),
    };
  });
}

export function graphTypeClassSuffix(type: string): string {
  const fallback = type.toLowerCase().replace(/[^a-z0-9]+/g, "") || "default";
  return visualTypeVariant(type) ?? fallback;
}

export function graphRelationClass(relation: string): string {
  return visualRelationClass(relation);
}

export function graphRelationLabel(relation: string, lang: string): string {
  const entry = visualRelation(relation);
  return entry ? visualLabel(entry, lang) : relation;
}

// depthFogFor returns the visual scaling factors for a node at the
// given hop distance from the focus. depth=0 is the focus itself
// (full opacity, full scale), depth=1 ring is still strong, depth>=3
// fades to the floor. Returns null when the input is the legacy
// "no focus" path so callers can skip applying fog entirely.
export function depthFogFor(distance: number | undefined): { opacity: number; scale: number } {
  if (distance === undefined || !Number.isFinite(distance)) {
    // Far / unreachable: floor.
    return { opacity: 0.18, scale: 0.78 };
  }
  if (distance <= 0) return { opacity: 1, scale: 1.06 };
  if (distance === 1) return { opacity: 0.92, scale: 1 };
  if (distance === 2) return { opacity: 0.55, scale: 0.9 };
  if (distance === 3) return { opacity: 0.32, scale: 0.82 };
  return { opacity: 0.18, scale: 0.78 };
}

// edgeFogFor mirrors depthFogFor for edges — edge opacity is the
// minimum of its two endpoints so a bright focus doesn't anchor a long
// line to a fully-faded distant node.
export function edgeFogFor(sourceDist: number | undefined, targetDist: number | undefined): number {
  const a = depthFogFor(sourceDist).opacity;
  const b = depthFogFor(targetDist).opacity;
  return Math.min(a, b);
}

// computeEgoSubgraph picks the focus + 1-hop neighbours + a DOI-ranked
// 2-hop bridge ring. The visible total is capped (default 60). Edges
// returned only connect nodes that survived the cut — callers can pass
// the result straight into computeForceLayout without re-filtering.
//
// DOI ranking factors (decision-pindoc-graph-contextual-ego-atlas):
// cross-area edge / blocker / supersedes / recently updated / type diversity.
export function computeEgoSubgraph(
  focusId: string,
  list: ArtifactRef[],
  allEdges: Array<GraphLayoutEdge & { relation?: string }>,
  options?: { visibleCap?: number; bridgeCap?: number },
): EgoSubgraph {
  const visibleCap = options?.visibleCap ?? EGO_VISIBLE_CAP;
  const bridgeCap = options?.bridgeCap ?? EGO_BRIDGE_CAP;
  const byID = new Map(list.map((a) => [a.id, a]));
  const focus = byID.get(focusId);
  if (!focus) {
    return { focusId, nodes: [], edges: [], capped: false, dropped_bridges: [] };
  }

  const adj = new Map<string, Array<{ to: string; relation?: string }>>();
  for (const e of allEdges) {
    if (!adj.has(e.source)) adj.set(e.source, []);
    if (!adj.has(e.target)) adj.set(e.target, []);
    adj.get(e.source)!.push({ to: e.target, relation: e.relation });
    adj.get(e.target)!.push({ to: e.source, relation: e.relation });
  }

  const hop1Ids = new Set<string>();
  for (const link of adj.get(focusId) ?? []) {
    if (byID.has(link.to)) hop1Ids.add(link.to);
  }

  const hop2Candidates = new Map<string, { fromHop1: string[]; relations: string[] }>();
  for (const hop1Id of hop1Ids) {
    for (const link of adj.get(hop1Id) ?? []) {
      if (link.to === focusId) continue;
      if (hop1Ids.has(link.to)) continue;
      if (!byID.has(link.to)) continue;
      const entry = hop2Candidates.get(link.to);
      if (entry) {
        entry.fromHop1.push(hop1Id);
        if (link.relation) entry.relations.push(link.relation);
      } else {
        hop2Candidates.set(link.to, {
          fromHop1: [hop1Id],
          relations: link.relation ? [link.relation] : [],
        });
      }
    }
  }

  // Score every 2-hop candidate. Higher = more important to keep as a
  // bridge. Type diversity is a soft signal — the more diverse the type
  // list already on screen, the smaller the boost. We compute that
  // after picking hop1 so the boost reflects the visible mix.
  const visibleTypes = new Set<string>();
  visibleTypes.add(focus.type);
  for (const id of hop1Ids) {
    const node = byID.get(id);
    if (node) visibleTypes.add(node.type);
  }

  const scored: Array<{
    node: ArtifactRef;
    score: number;
    reasons: string[];
  }> = [];
  for (const [id, info] of hop2Candidates) {
    const node = byID.get(id)!;
    let score = 0;
    const reasons: string[] = [];
    if (node.area_slug !== focus.area_slug) {
      score += 4;
      reasons.push("cross_area");
    }
    if (info.relations.some((r) => r === "blocks")) {
      score += 3;
      reasons.push("blocks");
    }
    if (info.relations.some((r) => r === "supersedes")) {
      score += 3;
      reasons.push("supersedes");
    }
    if (info.relations.some((r) => r === "implements")) {
      score += 2;
      reasons.push("implements");
    }
    const recencyDays = freshnessDays(node.updated_at);
    if (recencyDays !== null && recencyDays <= 7) {
      score += 2;
      reasons.push("recent");
    } else if (recencyDays !== null && recencyDays <= 30) {
      score += 1;
    }
    if (!visibleTypes.has(node.type)) {
      score += 1;
      reasons.push("new_type");
    }
    if (info.fromHop1.length >= 2) {
      score += 1;
      reasons.push("multi_path");
    }
    scored.push({ node, score, reasons });
  }
  scored.sort((a, b) => {
    if (b.score !== a.score) return b.score - a.score;
    return a.node.slug.localeCompare(b.node.slug);
  });

  // Cap calculation: we always want focus + hop1 fully on screen, so
  // bridges share whatever budget remains under visibleCap. bridgeCap
  // is a hard upper bound regardless of cap headroom.
  const hop1Count = hop1Ids.size;
  const remaining = Math.max(0, visibleCap - 1 - hop1Count);
  const bridgeSlots = Math.min(remaining, bridgeCap);
  const bridges = scored.slice(0, bridgeSlots);
  const droppedBridges = scored.slice(bridgeSlots).map((s) => s.node);

  const visibleIds = new Set<string>([focusId, ...hop1Ids]);
  for (const b of bridges) visibleIds.add(b.node.id);

  const nodes: EgoNode[] = [];
  nodes.push({ ...focus, ego_kind: "focus" });
  for (const id of hop1Ids) {
    const node = byID.get(id);
    if (node) nodes.push({ ...node, ego_kind: "hop1" });
  }
  for (const b of bridges) {
    nodes.push({ ...b.node, ego_kind: "bridge", doi_score: b.score, doi_reasons: b.reasons });
  }

  const edgeSeen = new Set<string>();
  const edges: GraphLayoutEdge[] = [];
  for (const e of allEdges) {
    if (!visibleIds.has(e.source) || !visibleIds.has(e.target)) continue;
    const key = `${e.source}::${e.target}::${e.relation ?? ""}`;
    if (edgeSeen.has(key)) continue;
    edgeSeen.add(key);
    edges.push({ source: e.source, target: e.target });
  }

  return {
    focusId,
    nodes,
    edges,
    capped: droppedBridges.length > 0,
    dropped_bridges: droppedBridges,
  };
}

// computeSemanticEgoLayout places ego nodes by Pindoc's typed-edge
// grammar instead of free force. focus sits dead-centre, hop1 nodes
// sit on a ring split into sectors by the relation type that connects
// them to focus, and bridge nodes form an outer ring sorted by area.
// Deterministic: same input → same coordinates, so re-mounting the
// component or sharing a URL never reshuffles the visible layout.
//
// Decision-pindoc-graph-contextual-ego-atlas Open issue notes which
// relation rides on the inner ring is unresolved; the fixed order below
// (implements > supersedes > blocks > references > relates_to >
// translation_of) is the first-pass answer — implements/supersedes/
// blocks earn the top half because they carry the strongest semantic
// pull on the focus's status.
const SEMANTIC_RELATION_ORDER = [
  "implements",
  "supersedes",
  "blocks",
  "references",
  "relates_to",
  "translation_of",
];

export function computeSemanticEgoLayout(
  ego: EgoSubgraph,
  edges: Array<GraphLayoutEdge & { relation?: string }>,
  options?: { hop1Radius?: number; bridgeRadius?: number },
): GraphLayout {
  if (ego.nodes.length === 0) {
    return { positions: new Map(), bounds: FULL_GRAPH_FRAME };
  }

  const hop1Radius = options?.hop1Radius ?? 220;
  const bridgeRadius = options?.bridgeRadius ?? 380;

  const positions = new Map<string, GraphPoint>();
  const focusId = ego.focusId;
  positions.set(focusId, { x: 0, y: 0 });

  const hop1Nodes = ego.nodes.filter((n) => n.ego_kind === "hop1");
  const hop1ById = new Map(hop1Nodes.map((n) => [n.id, n]));

  // Bucket every hop1 node by the relation type its focus-edge carries.
  // If multiple edges connect a hop1 to focus we pick the strongest
  // relation per SEMANTIC_RELATION_ORDER (implements beats references,
  // etc.) so the bucket assignment is unambiguous.
  const hop1Bucket = new Map<string, EgoNode[]>();
  const claimed = new Set<string>();
  const focusEdgeRelations = new Map<string, string[]>();
  for (const edge of edges) {
    if (edge.source !== focusId && edge.target !== focusId) continue;
    const otherId = edge.source === focusId ? edge.target : edge.source;
    if (!hop1ById.has(otherId)) continue;
    const list = focusEdgeRelations.get(otherId) ?? [];
    list.push(edge.relation ?? "relates_to");
    focusEdgeRelations.set(otherId, list);
  }
  for (const node of hop1Nodes) {
    const rels = focusEdgeRelations.get(node.id) ?? ["relates_to"];
    let primary = rels[0];
    let primaryRank = SEMANTIC_RELATION_ORDER.indexOf(primary);
    if (primaryRank === -1) primaryRank = SEMANTIC_RELATION_ORDER.length;
    for (const r of rels) {
      let rank = SEMANTIC_RELATION_ORDER.indexOf(r);
      if (rank === -1) rank = SEMANTIC_RELATION_ORDER.length;
      if (rank < primaryRank) {
        primary = r;
        primaryRank = rank;
      }
    }
    if (!hop1Bucket.has(primary)) hop1Bucket.set(primary, []);
    hop1Bucket.get(primary)!.push(node);
    claimed.add(node.id);
  }

  // Order the relation sectors deterministically: known relations first
  // (in fixed order), unknown trailing alphabetically.
  const usedRelations: string[] = [];
  for (const r of SEMANTIC_RELATION_ORDER) {
    if (hop1Bucket.has(r)) usedRelations.push(r);
  }
  const remainingRelations = Array.from(hop1Bucket.keys())
    .filter((r) => !usedRelations.includes(r))
    .sort();
  for (const r of remainingRelations) usedRelations.push(r);

  const sectorCount = Math.max(usedRelations.length, 1);
  const sectorWidth = (Math.PI * 2) / sectorCount;
  const startAngle = -Math.PI / 2; // 12 o'clock at top of canvas
  for (let i = 0; i < usedRelations.length; i++) {
    const sectorStart = startAngle + i * sectorWidth;
    const nodes = (hop1Bucket.get(usedRelations[i]) ?? []).slice();
    nodes.sort((a, b) => {
      if (a.area_slug !== b.area_slug) return a.area_slug.localeCompare(b.area_slug);
      return a.slug.localeCompare(b.slug);
    });
    const count = nodes.length;
    for (let j = 0; j < count; j++) {
      // Tighten the fraction so a sector with 1 node lands at its
      // centre (0.5 of the sector width) and N nodes are evenly
      // spaced inside the sector, never on the sector boundary.
      const fraction = (j + 0.5) / count;
      const angle = sectorStart + fraction * sectorWidth;
      positions.set(nodes[j].id, {
        x: Math.cos(angle) * hop1Radius,
        y: Math.sin(angle) * hop1Radius,
      });
    }
  }

  // Bridge ring: outer radius, sorted area-major, slug-minor so
  // adjacent areas cluster — same area always wraps the same arc.
  const bridges = ego.nodes.filter((n) => n.ego_kind === "bridge").slice();
  bridges.sort((a, b) => {
    if (a.area_slug !== b.area_slug) return a.area_slug.localeCompare(b.area_slug);
    return a.slug.localeCompare(b.slug);
  });
  const bridgeCount = bridges.length;
  for (let j = 0; j < bridgeCount; j++) {
    const fraction = (j + 0.5) / bridgeCount;
    const angle = startAngle + fraction * Math.PI * 2;
    positions.set(bridges[j].id, {
      x: Math.cos(angle) * bridgeRadius,
      y: Math.sin(angle) * bridgeRadius,
    });
  }

  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const p of positions.values()) {
    if (p.x < minX) minX = p.x;
    if (p.x > maxX) maxX = p.x;
    if (p.y < minY) minY = p.y;
    if (p.y > maxY) maxY = p.y;
  }
  if (!Number.isFinite(minX)) {
    return { positions, bounds: FULL_GRAPH_FRAME };
  }
  const padX = 80;
  const padY = 60;
  const bounds: GraphViewBox = {
    x: minX - padX,
    y: minY - padY,
    w: maxX - minX + padX * 2,
    h: maxY - minY + padY * 2,
  };
  return { positions, bounds };
}

function freshnessDays(updatedAt: string | undefined): number | null {
  if (!updatedAt) return null;
  const t = new Date(updatedAt).getTime();
  if (!Number.isFinite(t)) return null;
  return Math.max(0, (Date.now() - t) / (1000 * 60 * 60 * 24));
}

const START_FOCUS_STORAGE_KEY = "pindoc.reader.graph.lastFocus.v1";

// readLastFocusedSlug returns the slug we last set as the graph focus
// for this project, if any. sessionStorage scope means each tab has its
// own continuity — opening a new tab gives the Start Queue a clean
// slate instead of dropping the user back where some other tab landed.
export function readLastFocusedSlug(projectSlug: string): string | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.sessionStorage.getItem(START_FOCUS_STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Record<string, string>;
    return parsed[projectSlug] ?? null;
  } catch {
    return null;
  }
}

export function writeLastFocusedSlug(projectSlug: string, slug: string | null): void {
  if (typeof window === "undefined") return;
  try {
    const raw = window.sessionStorage.getItem(START_FOCUS_STORAGE_KEY);
    const parsed = (raw ? (JSON.parse(raw) as Record<string, string>) : {}) ?? {};
    if (slug) parsed[projectSlug] = slug;
    else delete parsed[projectSlug];
    window.sessionStorage.setItem(START_FOCUS_STORAGE_KEY, JSON.stringify(parsed));
  } catch {
    // Storage may be unavailable (private mode, full quota). The
    // session continues without continuity — Start Queue falls back
    // to the recent-meaningful path on next entry.
  }
}

// pickStartFocus implements the Start Queue ladder. last-focused has
// strongest priority: if the user just clicked away, return there.
// Otherwise the focus is the most recently meaningfully updated node
// (recent_meaningful), or — when the project is too small to compute a
// real recency signal — the most-connected node as a deterministic
// fallback. A 'fallback' kind is returned when there are zero nodes.
export function pickStartFocus(
  list: ArtifactRef[],
  edges: GraphLayoutEdge[],
  hint?: { lastFocusedSlug?: string | null },
): StartFocus | null {
  if (list.length === 0) return null;

  if (hint?.lastFocusedSlug) {
    const found = list.find((a) => a.slug === hint.lastFocusedSlug);
    if (found) {
      return {
        focusId: found.id,
        reason: { kind: "last_focused", slug: found.slug },
      };
    }
  }

  // recent_meaningful = max updated_at within the last 30 days. Old
  // projects that haven't moved in months fall through to most_connected.
  let mostRecent: ArtifactRef | null = null;
  let mostRecentTs = -Infinity;
  for (const a of list) {
    const ts = new Date(a.updated_at).getTime();
    if (!Number.isFinite(ts)) continue;
    if (ts > mostRecentTs) {
      mostRecent = a;
      mostRecentTs = ts;
    }
  }
  if (mostRecent) {
    const ageDays = (Date.now() - mostRecentTs) / (1000 * 60 * 60 * 24);
    if (ageDays <= 30) {
      return {
        focusId: mostRecent.id,
        reason: { kind: "recent_meaningful", updated_at: mostRecent.updated_at },
      };
    }
  }

  const degree = new Map<string, number>();
  for (const e of edges) {
    degree.set(e.source, (degree.get(e.source) ?? 0) + 1);
    degree.set(e.target, (degree.get(e.target) ?? 0) + 1);
  }
  let best: ArtifactRef | null = null;
  let bestScore = -1;
  for (const node of list) {
    const score = degree.get(node.id) ?? 0;
    if (
      score > bestScore ||
      (score === bestScore && best && node.slug < best.slug)
    ) {
      best = node;
      bestScore = score;
    }
  }
  if (best) {
    return {
      focusId: best.id,
      reason: { kind: "most_connected", degree: bestScore },
    };
  }
  // Defensive — list is non-empty but every node had NaN updated_at and
  // no edges. Pick the alphabetically first slug so the result is at
  // least deterministic.
  const fallback = [...list].sort((a, b) => a.slug.localeCompare(b.slug))[0];
  return {
    focusId: fallback.id,
    reason: { kind: "fallback", slug: fallback.slug },
  };
}
