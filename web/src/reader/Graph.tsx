import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { api, type Artifact, type ArtifactRef } from "../api/client";
import { useI18n } from "../i18n";
import { EmptyState, SurfaceHeader } from "./SurfacePrimitives";
import { appendBadgeFilters, type BadgeFilter } from "./badgeFilters";
import {
  FULL_GRAPH_VIEWBOX,
  graphFullPositions,
  graphRelationClass,
  graphRelationLabel,
  graphTypeClassSuffix,
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
};

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
  const [loading, setLoading] = useState(false);
  const nodeKey = useMemo(() => list.map((a) => a.slug).join("\x00"), [list]);
  const hasActiveFilters = Boolean(selectedArea || selectedType || badgeFilters.length > 0);

  useEffect(() => {
    if (list.length === 0) {
      setDetails({});
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    Promise.all(
      list.map((a) =>
        api.artifact(projectSlug, a.slug).catch(() => null),
      ),
    )
      .then((rows) => {
        if (cancelled) return;
        const next: Record<string, Artifact> = {};
        for (const row of rows) {
          if (row) next[row.id] = row;
        }
        setDetails(next);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [projectSlug, nodeKey, list]);

  const graph = useMemo(() => {
    const byID = new Map(list.map((a) => [a.id, a]));
    const positions = graphFullPositions(list.length);
    const positionByID = new Map(list.map((a, i) => [a.id, positions[i]]));
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
        edges.push({ key, source, target, relation: edge.relation });
      }
    }
    return { positions: positionByID, edges };
  }, [details, list]);

  const filterSummary = graphFilterSummary(t, selectedAreaLabel, selectedType, badgeFilters);
  const emptyMessage = hasActiveFilters
    ? t("graph.empty_filtered")
    : t("graph.empty");

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
            {loading && <span>{t("graph.loading_edges")}</span>}
            {!loading && list.length > 0 && <span>{t("graph.edge_count", graph.edges.length)}</span>}
          </div>
        </div>
        {list.length === 0 ? (
          <EmptyState message={emptyMessage} />
        ) : (
          <div className="graph-canvas-wrap">
            {graph.edges.length === 0 && !loading && (
              <div className="graph-canvas-empty">{t("graph.no_edges")}</div>
            )}
            <svg
              className="graph-canvas"
              viewBox={`0 0 ${FULL_GRAPH_VIEWBOX.width} ${FULL_GRAPH_VIEWBOX.height}`}
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
              </defs>
              {graph.edges.map((edge) => {
                const start = graph.positions.get(edge.source.id);
                const end = graph.positions.get(edge.target.id);
                if (!start || !end) return null;
                const labelX = (start.x + end.x) / 2;
                const labelY = (start.y + end.y) / 2 - 6;
                return (
                  <g key={edge.key}>
                    <line
                      x1={start.x}
                      y1={start.y}
                      x2={end.x}
                      y2={end.y}
                      className={`graph-canvas__edge graph-canvas__edge--${graphRelationClass(edge.relation)}`}
                      markerEnd="url(#graph-arrow)"
                    />
                    {graph.edges.length <= 48 && (
                      <text x={labelX} y={labelY} className="graph-canvas__edge-label">
                        {graphRelationLabel(edge.relation, lang)}
                      </text>
                    )}
                  </g>
                );
              })}
              {list.map((node) => {
                const p = graph.positions.get(node.id);
                if (!p) return null;
                const href = graphNodeHref(projectSlug, node.slug, selectedArea, selectedType, badgeFilters);
                return (
                  <a
                    key={node.id}
                    href={href}
                    className={`graph-canvas__node graph-canvas__node--${graphTypeClassSuffix(node.type)}`}
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
                      event.preventDefault();
                      navigate(href);
                    }}
                  >
                    <title>{`${node.title} (${node.type})`}</title>
                    <g transform={`translate(${p.x},${p.y})`}>
                      <rect className="graph-canvas__node-rect" x="-64" y="-25" width="128" height="50" rx="4" />
                      <text className="graph-canvas__node-type" y="-7" textAnchor="middle">
                        {node.type}
                      </text>
                      <text className="graph-canvas__node-title" y="10" textAnchor="middle">
                        {truncateRunes(node.title, 20)}
                      </text>
                    </g>
                  </a>
                );
              })}
            </svg>
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
