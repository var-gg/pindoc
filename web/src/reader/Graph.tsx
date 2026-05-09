import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { api, type ArtifactRef, type GraphEdgeRef } from "../api/client";
import { useI18n } from "../i18n";
import { projectSurfacePath } from "../readerRoutes";
import { EmptyState, SurfaceHeader } from "./SurfacePrimitives";
import { AtlasMinimap } from "./AtlasMinimap";
import { computeAtlas } from "./atlas";
import { appendBadgeFilters, type BadgeFilter } from "./badgeFilters";
import {
  computeEgoSubgraph,
  computeSemanticEgoLayout,
  depthFogFor,
  edgeFogFor,
  FOCUS_FRAME_WIDTH,
  FULL_GRAPH_FRAME,
  FULL_GRAPH_VIEWBOX,
  frameAround,
  graphHopDistances,
  graphRelationClass,
  graphRelationLabel,
  graphTypeClassSuffix,
  lerpFrame,
  viewBoxString,
  type EgoNode,
  type GraphLayoutEdge,
  type GraphPoint,
  type GraphViewBox,
} from "./graphSvg";

type Props = {
  projectSlug: string;
  orgSlug: string;
  list: ArtifactRef[];
  allCount: number;
  selectedArea: string | null;
  selectedAreaLabel: string | null;
  selectedType: string | null;
  badgeFilters: BadgeFilter[];
  // Focus is owned by ReaderShell (URL is source of truth). The graph
  // surface fires onFocusChange when the user clicks a node and
  // re-renders against the prop on every focusSlug update — this keeps
  // browser back/forward and Sidecar in lockstep without a separate
  // local copy.
  focusSlug: string | null;
  onFocusChange: (slug: string) => void;
  includeTemplates?: boolean;
  // Atlas minimap support (P2). areaNameBySlug provides display labels;
  // onSelectArea is the parent handler that pivots the surface filter
  // to the clicked area_slug. Both optional so the surface stays usable
  // outside ReaderShell (e.g. embedded preview).
  areaNameBySlug?: ReadonlyMap<string, string>;
  onSelectArea?: (areaSlug: string) => void;
};

type GraphEdge = {
  key: string;
  source: ArtifactRef;
  target: ArtifactRef;
  relation: string;
  crossArea: boolean;
};

const FLY_TO_DURATION_MS = 800;
// Quiet tour idle threshold (P3). After N ms of no input we cycle the
// camera to point at successive cross-area bridge nodes — meaning the
// drift is now an IA signal, not random panning. Disabled entirely
// when prefers-reduced-motion is set.
const QUIET_TOUR_IDLE_MS = 6000;
const QUIET_TOUR_HOLD_MS = 4000;
const QUIET_TOUR_OFFSET = 32;

export function GraphSurface({
  projectSlug,
  orgSlug,
  list,
  allCount,
  selectedArea,
  selectedAreaLabel,
  selectedType,
  badgeFilters,
  focusSlug,
  onFocusChange,
  includeTemplates = false,
  areaNameBySlug,
  onSelectArea,
}: Props) {
  const { t, lang } = useI18n();
  const navigate = useNavigate();
  const [edgeRows, setEdgeRows] = useState<GraphEdgeRef[]>([]);
  const [loadingEdges, setLoadingEdges] = useState(false);
  const [hoverId, setHoverId] = useState<string | null>(null);
  const hasActiveFilters = Boolean(selectedArea || selectedType || badgeFilters.length > 0);

  const cameraRef = useRef<GraphViewBox>(FULL_GRAPH_FRAME);
  const cameraAnimRef = useRef<{
    from: GraphViewBox;
    to: GraphViewBox;
    startedAt: number;
    duration: number;
  } | null>(null);
  const lastUserActivityRef = useRef<number>(performance.now());
  const tourCursorRef = useRef<number>(0);
  const [, setTick] = useState(0);
  const reducedMotion = useMemo(() => {
    if (typeof window === "undefined" || !window.matchMedia) return false;
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }, []);

  // Edge fetch is project-wide and light. The graph filters it against
  // the current artifact list client-side so this stays one request per
  // project instead of one detail request per node.
  useEffect(() => {
    let cancelled = false;
    setLoadingEdges(true);
    api.graphEdges(projectSlug, { includeTemplates })
      .then((resp) => {
        if (cancelled) return;
        setEdgeRows(resp.edges);
      })
      .catch(() => {
        if (!cancelled) setEdgeRows([]);
      })
      .finally(() => {
        if (!cancelled) setLoadingEdges(false);
      });
    return () => {
      cancelled = true;
    };
  }, [projectSlug, includeTemplates]);

  const focusId = useMemo(() => {
    if (!focusSlug) return null;
    return list.find((a) => a.slug === focusSlug)?.id ?? null;
  }, [focusSlug, list]);

  const allEdges = useMemo(() => {
    const byID = new Map(list.map((a) => [a.id, a]));
    const seen = new Set<string>();
    const edges: Array<GraphLayoutEdge & { relation: string; crossArea: boolean; sourceRef: ArtifactRef; targetRef: ArtifactRef }> = [];
    for (const edge of edgeRows) {
      const source = byID.get(edge.source_id);
      const target = byID.get(edge.target_id);
      if (!source || !target) continue;
      const key = `${edge.source_id}::${edge.target_id}::${edge.relation}`;
      if (seen.has(key)) continue;
      seen.add(key);
      edges.push({
        source: edge.source_id,
        target: edge.target_id,
        relation: edge.relation,
        crossArea: source.area_slug !== target.area_slug,
        sourceRef: source,
        targetRef: target,
      });
    }
    return edges;
  }, [edgeRows, list]);

  const ego = useMemo(() => {
    if (!focusId) {
      return null;
    }
    return computeEgoSubgraph(focusId, list, allEdges);
  }, [focusId, list, allEdges]);

  const visibleEdges: GraphEdge[] = useMemo(() => {
    if (!ego) return [];
    const visibleIds = new Set(ego.nodes.map((n) => n.id));
    const out: GraphEdge[] = [];
    const seen = new Set<string>();
    for (const e of allEdges) {
      if (!visibleIds.has(e.source) || !visibleIds.has(e.target)) continue;
      const key = `${e.source}::${e.target}::${e.relation}`;
      if (seen.has(key)) continue;
      seen.add(key);
      out.push({
        key,
        source: e.sourceRef,
        target: e.targetRef,
        relation: e.relation,
        crossArea: e.crossArea,
      });
    }
    return out;
  }, [ego, allEdges]);

  const layout = useMemo(() => {
    if (!ego) {
      return { positions: new Map<string, GraphPoint>(), bounds: FULL_GRAPH_FRAME };
    }
    // Semantic layout needs the relation labels — ego.edges drops them
    // for force compatibility, so we re-attach them from allEdges
    // before running the placement.
    const relationEdges = allEdges.map((e) => ({
      source: e.source,
      target: e.target,
      relation: e.relation,
    }));
    return computeSemanticEgoLayout(ego, relationEdges);
  }, [ego, allEdges]);

  const distances = useMemo(() => {
    if (!ego) return new Map<string, number>();
    return graphHopDistances(ego.focusId, ego.edges);
  }, [ego]);

  // Atlas data is computed against the full filtered list, not the ego
  // subgraph — the minimap is the "where am I" map. allEdges already
  // carries relation, but computeAtlas only needs source/target +
  // cross-area lookups so we reuse it directly.
  const atlas = useMemo(() => computeAtlas(list, allEdges), [list, allEdges]);
  const focusAreaSlug = useMemo(() => {
    if (!ego) return null;
    return ego.nodes.find((n) => n.ego_kind === "focus")?.area_slug ?? null;
  }, [ego]);

  // Camera animation (P3 — IA-bound). On focus change we fly to the
  // new focus node; afterwards an idle "quiet tour" leans the camera
  // toward each bridge node in turn so the user sees Pindoc's
  // cross-area connections instead of random pan. prefers-reduced-
  // motion disables both: the camera jumps to focus and never drifts.
  useEffect(() => {
    if (!focusId) return;
    const target = layout.positions.get(focusId);
    if (!target) return;
    if (reducedMotion) {
      cameraRef.current = frameAround(target, FOCUS_FRAME_WIDTH);
      setTick((n) => (n + 1) % 1_000_000);
      return;
    }
    cameraAnimRef.current = {
      from: cameraRef.current,
      to: frameAround(target, FOCUS_FRAME_WIDTH),
      startedAt: performance.now(),
      duration: FLY_TO_DURATION_MS,
    };
    lastUserActivityRef.current = performance.now();
    tourCursorRef.current = 0;
    const bridgeIds = ego
      ? ego.nodes.filter((n) => n.ego_kind === "bridge").map((n) => n.id)
      : [];
    let raf = 0;
    const loop = (now: number) => {
      const anim = cameraAnimRef.current;
      let mutated = false;
      if (anim) {
        const t = (now - anim.startedAt) / anim.duration;
        const next = lerpFrame(anim.from, anim.to, t);
        cameraRef.current = next;
        mutated = true;
        if (t >= 1) cameraAnimRef.current = null;
      } else {
        const idle = now - lastUserActivityRef.current;
        if (idle >= QUIET_TOUR_IDLE_MS && bridgeIds.length > 0) {
          // Cycle through bridges; each holds for QUIET_TOUR_HOLD_MS.
          // The lean is a small offset toward the bridge — never far
          // enough to lose focus context, just enough to point.
          const cyclePos = Math.floor(
            (idle - QUIET_TOUR_IDLE_MS) / QUIET_TOUR_HOLD_MS,
          );
          const targetIdx = cyclePos % bridgeIds.length;
          const targetBridgeId = bridgeIds[targetIdx];
          const focusPoint = layout.positions.get(focusId);
          const bridgePoint = layout.positions.get(targetBridgeId);
          if (focusPoint && bridgePoint) {
            const dx = bridgePoint.x - focusPoint.x;
            const dy = bridgePoint.y - focusPoint.y;
            const len = Math.hypot(dx, dy) || 1;
            const ux = dx / len;
            const uy = dy / len;
            const base = frameAround(focusPoint, FOCUS_FRAME_WIDTH);
            cameraRef.current = {
              x: base.x + ux * QUIET_TOUR_OFFSET,
              y: base.y + uy * QUIET_TOUR_OFFSET,
              w: base.w,
              h: base.h,
            };
            mutated = true;
            tourCursorRef.current = targetIdx;
          }
        }
      }
      if (mutated) setTick((n) => (n + 1) % 1_000_000);
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, [focusId, layout.positions, ego, reducedMotion]);

  const markActivity = () => {
    lastUserActivityRef.current = performance.now();
  };

  const filterSummary = graphFilterSummary(t, selectedAreaLabel, selectedType, badgeFilters);
  const emptyMessage = hasActiveFilters
    ? t("graph.empty_filtered")
    : t("graph.empty");

  const hoverDetail = hoverId ? list.find((artifact) => artifact.id === hoverId) ?? null : null;
  const hoverPos = hoverId ? layout.positions.get(hoverId) : null;

  return (
    <main className="content graph-content">
      <div className="graph-surface">
        <div className="graph-surface__head">
          <SurfaceHeader
            name="graph"
            count={list.length}
            secondary={hasActiveFilters ? { label: t("surface.all"), count: allCount } : undefined}
          />
          <div className="graph-surface__meta">
            {filterSummary}
            {loadingEdges && <span>{t("graph.loading_edges")}</span>}
            {!loadingEdges && list.length > 0 && (
              <span>{t("graph.edge_count", visibleEdges.length)}</span>
            )}
            {ego && (
              <span className="graph-surface__ego">
                {t("graph.ego_visible", ego.nodes.length)}
                {ego.capped && ` · +${ego.dropped_bridges.length}`}
              </span>
            )}
          </div>
        </div>
        {list.length === 0 ? (
          <EmptyState message={emptyMessage} />
        ) : !ego ? (
          <EmptyState message={t("graph.no_focus")} />
        ) : (
          <div
            className="graph-canvas-wrap graph-canvas-wrap--orbit"
            onMouseMove={markActivity}
            onMouseLeave={() => {
              setHoverId(null);
              markActivity();
            }}
          >
            {visibleEdges.length === 0 && !loadingEdges && (
              <div className="graph-canvas-empty">{t("graph.no_edges")}</div>
            )}
            <svg
              className="graph-canvas graph-canvas--orbit"
              viewBox={viewBoxString(cameraRef.current)}
              preserveAspectRatio="xMidYMid meet"
              role="img"
              aria-label={t("graph.aria_label")}
            >
              <defs>
                <marker
                  id="graph-arrow"
                  markerHeight="8"
                  markerWidth="8"
                  orient="auto"
                  refX="7"
                  refY="4"
                >
                  <path d="M0,0 L8,4 L0,8 Z" className="graph-canvas__arrow" />
                </marker>
                <radialGradient id="graph-bloom" cx="50%" cy="50%" r="50%">
                  <stop offset="0%" stopColor="rgba(255,255,255,0.18)" />
                  <stop offset="60%" stopColor="rgba(255,255,255,0.04)" />
                  <stop offset="100%" stopColor="rgba(255,255,255,0)" />
                </radialGradient>
              </defs>

              {focusId &&
                (() => {
                  const p = layout.positions.get(focusId);
                  if (!p) return null;
                  return (
                    <circle
                      cx={p.x}
                      cy={p.y}
                      r={210}
                      fill="url(#graph-bloom)"
                      pointerEvents="none"
                    />
                  );
                })()}

              {visibleEdges.map((edge) => {
                const start = layout.positions.get(edge.source.id);
                const end = layout.positions.get(edge.target.id);
                if (!start || !end) return null;
                const sd = distances.get(edge.source.id);
                const td = distances.get(edge.target.id);
                const opacity = focusId ? edgeFogFor(sd, td) : 0.55;
                const showLabel =
                  hoverId === edge.source.id ||
                  hoverId === edge.target.id ||
                  (focusId &&
                    (focusId === edge.source.id || focusId === edge.target.id) &&
                    visibleEdges.length <= 24);
                const labelX = (start.x + end.x) / 2;
                const labelY = (start.y + end.y) / 2 - 6;
                return (
                  <g
                    key={edge.key}
                    className={`graph-canvas__edge-group${edge.crossArea ? " graph-canvas__edge-group--cross-area" : ""}`}
                    style={{ opacity }}
                  >
                    <line
                      x1={start.x}
                      y1={start.y}
                      x2={end.x}
                      y2={end.y}
                      className={`graph-canvas__edge graph-canvas__edge--${graphRelationClass(edge.relation)}${edge.crossArea ? " graph-canvas__edge--cross-area" : ""}`}
                      markerEnd="url(#graph-arrow)"
                    />
                    {showLabel && (
                      <text x={labelX} y={labelY} className="graph-canvas__edge-label">
                        {graphRelationLabel(edge.relation, lang)}
                      </text>
                    )}
                  </g>
                );
              })}

              {ego.nodes.map((node) => {
                const p = layout.positions.get(node.id);
                if (!p) return null;
                const distance = focusId ? distances.get(node.id) : 0;
                const fog = depthFogFor(distance);
                const isFocus = focusId === node.id;
                const isHover = hoverId === node.id;
                const href = graphNodeHref(projectSlug, orgSlug, node.slug, selectedArea, selectedType, badgeFilters);
                return (
                  <a
                    key={node.id}
                    href={href}
                    className={`graph-canvas__node graph-canvas__node--${graphTypeClassSuffix(node.type)}${isFocus ? " graph-canvas__node--focus" : ""}${isHover ? " graph-canvas__node--hover" : ""}${node.ego_kind === "bridge" ? " graph-canvas__node--bridge" : ""}`}
                    style={{ opacity: fog.opacity }}
                    onMouseEnter={() => {
                      setHoverId(node.id);
                      markActivity();
                    }}
                    onMouseLeave={() => {
                      setHoverId(null);
                      markActivity();
                    }}
                    onClick={(event) => {
                      // Cmd/Ctrl/Shift/Alt or middle-click — let the
                      // browser open the artifact in a new tab/pane.
                      // Plain double-click opens the reader inline.
                      if (
                        event.button !== 0 ||
                        event.metaKey ||
                        event.ctrlKey ||
                        event.shiftKey ||
                        event.altKey
                      ) {
                        return;
                      }
                      if (event.detail >= 2) {
                        // Browser-native double click — let the click
                        // bubble through to the <a> by *not* preventing
                        // default. The first single click already shifted
                        // focus; the second navigates.
                        return;
                      }
                      event.preventDefault();
                      markActivity();
                      onFocusChange(node.slug);
                    }}
                  >
                    <title>{`${node.title} (${node.type})`}</title>
                    <g
                      transform={`translate(${p.x},${p.y}) scale(${fog.scale})`}
                      className="graph-canvas__node-inner"
                    >
                      <text
                        className="graph-canvas__node-type"
                        y={isFocus || isHover ? -16 : -10}
                        textAnchor="middle"
                      >
                        {node.type}
                      </text>
                      <text
                        className="graph-canvas__node-title"
                        y={isFocus || isHover ? 6 : 8}
                        textAnchor="middle"
                      >
                        {truncateRunes(node.title, 22)}
                      </text>
                      {(isFocus || isHover) && (
                        <rect
                          className="graph-canvas__node-rect"
                          x={-72}
                          y={-26}
                          width={144}
                          height={52}
                          rx={6}
                        />
                      )}
                    </g>
                  </a>
                );
              })}
            </svg>

            {hoverDetail && hoverPos && (
              <div
                className="graph-peek"
                style={{
                  left: peekScreenX(cameraRef.current, hoverPos),
                  top: peekScreenY(cameraRef.current, hoverPos),
                }}
              >
                <div className="graph-peek__type">{hoverDetail.type}</div>
                <div className="graph-peek__title">{hoverDetail.title}</div>
                <a
                  className="graph-peek__open"
                  href={graphNodeHref(projectSlug, orgSlug, hoverDetail.slug, selectedArea, selectedType, badgeFilters)}
                  onClick={(event) => {
                    if (
                      event.button !== 0 ||
                      event.metaKey ||
                      event.ctrlKey ||
                      event.shiftKey ||
                      event.altKey
                    )
                      return;
                    event.preventDefault();
                    navigate(
                      graphNodeHref(projectSlug, orgSlug, hoverDetail.slug, selectedArea, selectedType, badgeFilters),
                    );
                  }}
                >
                  {t("graph.peek_open")}
                </a>
              </div>
            )}
            <AtlasMinimap
              data={atlas}
              focusAreaSlug={focusAreaSlug}
              areaNameBySlug={areaNameBySlug}
              onAreaClick={onSelectArea}
            />
          </div>
        )}
      </div>
    </main>
  );
}

function graphNodeHref(
  projectSlug: string,
  orgSlug: string,
  slug: string,
  selectedArea: string | null,
  selectedType: string | null,
  badgeFilters: BadgeFilter[],
): string {
  const params = new URLSearchParams();
  if (selectedArea) params.set("area", selectedArea);
  if (selectedType) params.set("type", selectedType);
  appendBadgeFilters(params, badgeFilters);
  const qs = params.toString();
  return `${projectSurfacePath(projectSlug, "wiki", slug, orgSlug)}${qs ? `?${qs}` : ""}`;
}

function graphFilterSummary(
  t: (key: string, ...args: Array<string | number>) => string,
  selectedAreaLabel: string | null,
  selectedType: string | null,
  badgeFilters: BadgeFilter[],
): string {
  const parts = [
    selectedAreaLabel ? `${t("graph.filter_area")}: ${selectedAreaLabel}` : "",
    selectedType ? `${t("graph.filter_type")}: ${selectedType}` : "",
    ...badgeFilters.map((f) => `${f.label}`),
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" · ") : t("graph.scope_all");
}

function truncateRunes(value: string, max: number): string {
  const runes = Array.from(value.trim());
  if (runes.length <= max) return value;
  return `${runes.slice(0, max).join("")}...`;
}

function peekScreenX(camera: GraphViewBox, p: GraphPoint): number {
  const aspect = FULL_GRAPH_VIEWBOX.width / FULL_GRAPH_VIEWBOX.height;
  const cameraAspect = camera.w / camera.h;
  const renderHeight = 540;
  const renderWidth = renderHeight * aspect;
  const scale = renderWidth / camera.w;
  const offset = cameraAspect > aspect ? 0 : (renderWidth - renderHeight * cameraAspect) / 2;
  return (p.x - camera.x) * scale + offset + 14;
}

function peekScreenY(camera: GraphViewBox, p: GraphPoint): number {
  const renderHeight = 540;
  const scale = renderHeight / camera.h;
  return (p.y - camera.y) * scale - 32;
}

// re-export for convenience: the legacy import surface used to expose
// EgoNode-less typing. Keep the alias so any downstream caller compiles.
export type { EgoNode };
