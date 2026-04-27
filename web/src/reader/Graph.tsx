import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { api, type Artifact, type ArtifactRef } from "../api/client";
import { useI18n } from "../i18n";
import { EmptyState, SurfaceHeader } from "./SurfacePrimitives";
import { appendBadgeFilters, type BadgeFilter } from "./badgeFilters";
import {
  computeForceLayout,
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
  type GraphLayoutEdge,
  type GraphPoint,
  type GraphViewBox,
} from "./graphSvg";

type Props = {
  projectSlug: string;
  list: ArtifactRef[];
  allCount: number;
  selectedArea: string | null;
  selectedAreaLabel: string | null;
  selectedType: string | null;
  badgeFilters: BadgeFilter[];
};

type GraphEdge = {
  key: string;
  source: ArtifactRef;
  target: ArtifactRef;
  relation: string;
  crossArea: boolean;
};

const FLY_TO_DURATION_MS = 800;
const AMBIENT_DRIFT_IDLE_MS = 5000;
const AMBIENT_DRIFT_AMPLITUDE = 28; // px in viewBox units
const AMBIENT_DRIFT_PERIOD_MS = 32000;

export function GraphSurface({
  projectSlug,
  list,
  allCount,
  selectedArea,
  selectedAreaLabel,
  selectedType,
  badgeFilters,
}: Props) {
  const { t, lang } = useI18n();
  const navigate = useNavigate();
  const [details, setDetails] = useState<Record<string, Artifact>>({});
  const [loadingEdges, setLoadingEdges] = useState(false);
  const [focusId, setFocusId] = useState<string | null>(null);
  const [hoverId, setHoverId] = useState<string | null>(null);
  const nodeKey = useMemo(() => list.map((a) => a.slug).join("\x00"), [list]);
  const hasActiveFilters = Boolean(selectedArea || selectedType || badgeFilters.length > 0);

  // Camera state lives in a ref + a tick state so the SVG re-renders
  // each animation frame without React reconciling the whole node
  // tree. The current viewBox is read in render via cameraRef.current.
  const cameraRef = useRef<GraphViewBox>(FULL_GRAPH_FRAME);
  const cameraAnimRef = useRef<{
    from: GraphViewBox;
    to: GraphViewBox;
    startedAt: number;
    duration: number;
  } | null>(null);
  const lastUserActivityRef = useRef<number>(performance.now());
  const driftPhaseRef = useRef<number>(0);
  const [, setTick] = useState(0);

  // Fetch full artifacts so we can read relates_to. Same as the legacy
  // surface — every node detail is needed to draw edges.
  useEffect(() => {
    if (list.length === 0) {
      setDetails({});
      setLoadingEdges(false);
      return;
    }
    let cancelled = false;
    setLoadingEdges(true);
    Promise.all(list.map((a) => api.artifact(projectSlug, a.slug).catch(() => null)))
      .then((rows) => {
        if (cancelled) return;
        const next: Record<string, Artifact> = {};
        for (const row of rows) {
          if (row) next[row.id] = row;
        }
        setDetails(next);
      })
      .finally(() => {
        if (!cancelled) setLoadingEdges(false);
      });
    return () => {
      cancelled = true;
    };
  }, [projectSlug, nodeKey, list]);

  const graph = useMemo(() => {
    const byID = new Map(list.map((a) => [a.id, a]));
    const layoutEdges: GraphLayoutEdge[] = [];
    const edgeSeen = new Set<string>();
    const edges: GraphEdge[] = [];
    for (const source of list) {
      const detail = details[source.id];
      for (const edge of detail?.relates_to ?? []) {
        const target = byID.get(edge.artifact_id);
        if (!target) continue;
        const key = `${source.id}:${edge.artifact_id}:${edge.relation}`;
        if (edgeSeen.has(key)) continue;
        edgeSeen.add(key);
        const crossArea = source.area_slug !== target.area_slug;
        edges.push({ key, source, target, relation: edge.relation, crossArea });
        layoutEdges.push({ source: source.id, target: target.id });
      }
    }
    const layout = computeForceLayout({ nodes: list, edges: layoutEdges });
    return { layout, edges, layoutEdges, byID };
  }, [details, list]);

  // Pick a default focus once the layout is known. Most-connected node
  // wins; ties go to the alphabetically-first slug for determinism.
  // The user can move focus by clicking another node.
  useEffect(() => {
    if (list.length === 0) {
      if (focusId !== null) setFocusId(null);
      return;
    }
    if (focusId && graph.byID.has(focusId)) return;
    const degree = new Map<string, number>();
    for (const e of graph.layoutEdges) {
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
    if (best) setFocusId(best.id);
  }, [graph.layoutEdges, list, focusId, graph.byID]);

  const distances = useMemo(() => {
    if (!focusId) return new Map<string, number>();
    return graphHopDistances(focusId, graph.layoutEdges);
  }, [focusId, graph.layoutEdges]);

  // Drive the camera: focus changes start a fly-to, idle time drifts
  // the camera around the focus. Single rAF loop handles both — exits
  // when nothing is moving so we don't burn CPU on a static graph.
  useEffect(() => {
    if (!focusId) return;
    const target = graph.layout.positions.get(focusId);
    if (!target) return;
    cameraAnimRef.current = {
      from: cameraRef.current,
      to: frameAround(target, FOCUS_FRAME_WIDTH),
      startedAt: performance.now(),
      duration: FLY_TO_DURATION_MS,
    };
    lastUserActivityRef.current = performance.now();
    driftPhaseRef.current = 0;
    let raf = 0;
    const loop = (now: number) => {
      const anim = cameraAnimRef.current;
      let mutated = false;
      if (anim) {
        const t = (now - anim.startedAt) / anim.duration;
        const next = lerpFrame(anim.from, anim.to, t);
        cameraRef.current = next;
        mutated = true;
        if (t >= 1) {
          cameraAnimRef.current = null;
        }
      } else {
        // Ambient drift: only after idle, only when no fly-to is
        // running. Sin/cos in viewBox units shift the frame around the
        // focus point.
        const idle = now - lastUserActivityRef.current;
        if (idle >= AMBIENT_DRIFT_IDLE_MS) {
          const t = ((now - lastUserActivityRef.current - AMBIENT_DRIFT_IDLE_MS) /
            AMBIENT_DRIFT_PERIOD_MS) *
            Math.PI *
            2;
          const focusPoint = graph.layout.positions.get(focusId);
          if (focusPoint) {
            const dx = Math.cos(t) * AMBIENT_DRIFT_AMPLITUDE;
            const dy = Math.sin(t * 0.8) * (AMBIENT_DRIFT_AMPLITUDE * 0.7);
            const base = frameAround(focusPoint, FOCUS_FRAME_WIDTH);
            cameraRef.current = {
              x: base.x + dx,
              y: base.y + dy,
              w: base.w,
              h: base.h,
            };
            mutated = true;
            driftPhaseRef.current = t;
          }
        }
      }
      if (mutated) {
        setTick((n) => (n + 1) % 1_000_000);
      }
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, [focusId, graph.layout.positions]);

  const markActivity = () => {
    lastUserActivityRef.current = performance.now();
  };

  const filterSummary = graphFilterSummary(t, selectedAreaLabel, selectedType, badgeFilters);
  const emptyMessage = hasActiveFilters
    ? t("graph.empty_filtered")
    : t("graph.empty");

  // Pre-compute hover peek info — picked only when hovered node has a
  // detail loaded. Failing to find a detail still draws the node, just
  // without preview text.
  const hoverDetail = hoverId ? details[hoverId] : null;
  const hoverPos = hoverId ? graph.layout.positions.get(hoverId) : null;

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
              <span>{t("graph.edge_count", graph.edges.length)}</span>
            )}
          </div>
        </div>
        {list.length === 0 ? (
          <EmptyState message={emptyMessage} />
        ) : (
          <div
            className="graph-canvas-wrap graph-canvas-wrap--orbit"
            onMouseMove={markActivity}
            onMouseLeave={() => {
              setHoverId(null);
              markActivity();
            }}
          >
            {graph.edges.length === 0 && !loadingEdges && (
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

              {/* Bloom backdrop centred on the focus node — adds the
                  cinematic glow without blowing up GPU work. Skipped
                  when there's no focus yet (initial mount). */}
              {focusId &&
                (() => {
                  const p = graph.layout.positions.get(focusId);
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

              {graph.edges.map((edge) => {
                const start = graph.layout.positions.get(edge.source.id);
                const end = graph.layout.positions.get(edge.target.id);
                if (!start || !end) return null;
                const sd = distances.get(edge.source.id);
                const td = distances.get(edge.target.id);
                const opacity = focusId ? edgeFogFor(sd, td) : 0.55;
                const showLabel =
                  hoverId === edge.source.id ||
                  hoverId === edge.target.id ||
                  (focusId &&
                    (focusId === edge.source.id || focusId === edge.target.id) &&
                    graph.edges.length <= 24);
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

              {list.map((node) => {
                const p = graph.layout.positions.get(node.id);
                if (!p) return null;
                const distance = focusId ? distances.get(node.id) : 0;
                const fog = depthFogFor(distance);
                const isFocus = focusId === node.id;
                const isHover = hoverId === node.id;
                const href = graphNodeHref(projectSlug, node.slug, selectedArea, selectedType, badgeFilters);
                return (
                  <a
                    key={node.id}
                    href={href}
                    className={`graph-canvas__node graph-canvas__node--${graphTypeClassSuffix(node.type)}${isFocus ? " graph-canvas__node--focus" : ""}${isHover ? " graph-canvas__node--hover" : ""}`}
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
                      if (
                        event.defaultPrevented ||
                        event.button !== 0 ||
                        event.metaKey ||
                        event.ctrlKey ||
                        event.shiftKey ||
                        event.altKey
                      ) {
                        return;
                      }
                      // Click = move camera focus, not navigate. The
                      // user opens the artifact via the peek card.
                      event.preventDefault();
                      markActivity();
                      setFocusId(node.id);
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
                  href={graphNodeHref(projectSlug, hoverDetail.slug, selectedArea, selectedType, badgeFilters)}
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
                      graphNodeHref(projectSlug, hoverDetail.slug, selectedArea, selectedType, badgeFilters),
                    );
                  }}
                >
                  {t("graph.peek_open")}
                </a>
              </div>
            )}
          </div>
        )}
      </div>
    </main>
  );
}

function graphNodeHref(
  projectSlug: string,
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
  return `/p/${projectSlug}/wiki/${slug}${qs ? `?${qs}` : ""}`;
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

// peekScreenX / peekScreenY map a world point to a CSS pixel offset
// inside the wrapper, so the floating peek card lines up with its
// node even as the camera animates. The conversion mirrors the
// preserveAspectRatio="xMidYMid meet" we set on the svg.
function peekScreenX(camera: GraphViewBox, p: GraphPoint): number {
  const aspect = FULL_GRAPH_VIEWBOX.width / FULL_GRAPH_VIEWBOX.height;
  const cameraAspect = camera.w / camera.h;
  // The svg renders at its natural width/height ratio inside the
  // wrapper. We assume the wrapper letterboxes / pillar-boxes only on
  // the long axis; the short axis fills.
  const renderHeight = 540; // matches CSS height on the wrapper
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
