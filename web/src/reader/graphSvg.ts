import { forceCenter, forceLink, forceManyBody, forceSimulation, forceX, forceY } from "d3-force";
import type { ArtifactRef } from "../api/client";
import { visualLabel, visualRelation, visualRelationClass, visualTypeVariant } from "./visualLanguage";

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
