import { useMemo } from "react";
import { useI18n } from "../i18n";
import { computeAtlasPlacement, type AtlasData } from "./atlas";

type Props = {
  data: AtlasData;
  focusAreaSlug?: string | null;
  onAreaClick?: (areaSlug: string) => void;
  areaNameBySlug?: ReadonlyMap<string, string>;
};

const ATLAS_W = 240;
const ATLAS_H = 200;

export function AtlasMinimap({
  data,
  focusAreaSlug,
  onAreaClick,
  areaNameBySlug,
}: Props) {
  const { t } = useI18n();
  const placement = useMemo(
    () => computeAtlasPlacement(data, { canvasWidth: ATLAS_W, canvasHeight: ATLAS_H }),
    [data],
  );

  if (data.areas.length === 0) return null;

  return (
    <aside className="atlas-minimap" aria-label={t("graph.atlas_label")}>
      <div className="atlas-minimap__head">
        <span>{t("graph.atlas_title")}</span>
        <span className="atlas-minimap__count">{data.total_artifacts}</span>
      </div>
      <svg
        className="atlas-minimap__svg"
        viewBox={`0 0 ${ATLAS_W} ${ATLAS_H}`}
        role="img"
        aria-label={t("graph.atlas_aria")}
      >
        {/* Cross-area corridors first so they sit under bubbles. Edge
            count drives stroke opacity — a single shared edge is faint,
            five-plus reads as a substantive bridge. */}
        {data.cross_area_edges.map((edge) => {
          const from = placement.placements.get(edge.from);
          const to = placement.placements.get(edge.to);
          if (!from || !to) return null;
          const opacity = Math.min(0.9, 0.18 + edge.count * 0.12);
          return (
            <line
              key={`${edge.from}::${edge.to}`}
              x1={from.cx}
              y1={from.cy}
              x2={to.cx}
              y2={to.cy}
              className="atlas-minimap__corridor"
              style={{ opacity }}
            />
          );
        })}
        {data.areas.map((area) => {
          const p = placement.placements.get(area.area_slug);
          if (!p) return null;
          const isFocus = focusAreaSlug === area.area_slug;
          const recencyAlpha = 0.18 + area.recent_activity_score * 0.55;
          const label =
            areaNameBySlug?.get(area.area_slug) ?? area.area_slug;
          return (
            <g
              key={area.area_slug}
              className={`atlas-minimap__area${isFocus ? " atlas-minimap__area--focus" : ""}`}
              onClick={onAreaClick ? () => onAreaClick(area.area_slug) : undefined}
              role={onAreaClick ? "button" : undefined}
              tabIndex={onAreaClick ? 0 : undefined}
              onKeyDown={
                onAreaClick
                  ? (e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        onAreaClick(area.area_slug);
                      }
                    }
                  : undefined
              }
            >
              <title>
                {label} · {t("graph.atlas_artifact_count", area.artifact_count)}
              </title>
              <circle
                cx={p.cx}
                cy={p.cy}
                r={p.r}
                className="atlas-minimap__bubble"
                style={{ fillOpacity: recencyAlpha }}
              />
              <text
                x={p.cx}
                y={p.cy + 3}
                textAnchor="middle"
                className="atlas-minimap__bubble-count"
              >
                {area.artifact_count}
              </text>
            </g>
          );
        })}
      </svg>
    </aside>
  );
}
