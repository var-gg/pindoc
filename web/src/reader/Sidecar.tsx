import { ArrowUpRight } from "lucide-react";
import type { Artifact } from "../api/client";
import { useI18n } from "../i18n";
import { agentAvatar } from "./avatars";

type Props = {
  detail: Artifact | null;
};

export function Sidecar({ detail }: Props) {
  const { t } = useI18n();
  if (!detail) {
    return (
      <aside className="sidecar">
        <div className="sidecar__head">
          <h3>{t("sidecar.this_artifact")}</h3>
        </div>
        <div className="graph-wrap">
          <div className="mini-graph">
            <span className="mini-graph__empty">{t("sidecar.no_selection")}</span>
          </div>
        </div>
      </aside>
    );
  }

  const av = agentAvatar(detail.author_id);
  const publishedAt = detail.published_at
    ? new Date(detail.published_at).toLocaleString()
    : "—";

  // Graph edges aren't derived yet (Phase 3+ pipeline populates these via
  // artifact.superseded_by + future artifact_edges). Show placeholder
  // states so the visual treatment is faithful and the data gap is
  // honest.
  const hasSupersedes = Boolean(detail.superseded_by && detail.superseded_by !== "");

  return (
    <aside className="sidecar">
      <div className="sidecar__head">
        <h3>{t("sidecar.this_artifact")}</h3>
      </div>

      <div className="graph-wrap">
        <div className="mini-graph">
          <svg viewBox="0 0 280 200" style={{ width: "100%", height: "100%" }}>
            <g stroke="currentColor" strokeWidth="1" fill="none" opacity="0.35" style={{ color: "var(--fg-3)" }}>
              <line x1="140" y1="100" x2="60" y2="60" />
              <line x1="140" y1="100" x2="220" y2="50" />
              <line x1="140" y1="100" x2="220" y2="150" />
              <line x1="140" y1="100" x2="60" y2="150" />
            </g>
            <g fontFamily="JetBrains Mono, monospace" fontSize="9">
              <g transform="translate(140,100)">
                <circle r="14" fill="var(--live-bg)" stroke="var(--live)" strokeWidth="1.5" />
                <text textAnchor="middle" dy="3" fill="var(--live)">
                  {(detail.type.slice(0, 3) || "A").toUpperCase()}
                </text>
              </g>
              <g transform="translate(60,60)">
                <circle r="7" fill="var(--bg-2)" stroke="var(--fg-4)" strokeWidth="1" />
              </g>
              <g transform="translate(220,50)">
                <circle r="7" fill="var(--bg-2)" stroke="var(--fg-4)" strokeWidth="1" />
              </g>
              <g transform="translate(220,150)">
                <circle r="7" fill="var(--bg-2)" stroke="var(--fg-4)" strokeWidth="1" />
              </g>
              <g transform="translate(60,150)">
                <circle r="7" fill="var(--bg-2)" stroke="var(--fg-4)" strokeWidth="1" />
              </g>
            </g>
          </svg>
        </div>
      </div>

      <div className="relations">
        {!hasSupersedes && (
          <div className="relations__empty">{t("sidecar.no_relations")}</div>
        )}
        {hasSupersedes && (
          <div className="relation">
            <span className="relation__label">{t("sidecar.rel_supersedes")}</span>
            <span className="relation__target">
              <ArrowUpRight className="lucide" />
              {detail.superseded_by}
            </span>
          </div>
        )}
      </div>

      <div className="provenance">
        <div className="provenance__row">
          <span className="k">{t("sidecar.prov_author")}</span>
          <span className="v" style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <span className={av.className} style={{ width: 14, height: 14, fontSize: 8 }}>
              {av.initials}
            </span>
            {detail.author_id}
            {detail.author_version ? `@${detail.author_version}` : ""}
          </span>
        </div>
        <div className="provenance__row">
          <span className="k">{t("sidecar.prov_area")}</span>
          <span className="v">{detail.area_slug}</span>
        </div>
        <div className="provenance__row">
          <span className="k">{t("sidecar.prov_status")}</span>
          <span className="v">
            {detail.status} · {detail.completeness}
          </span>
        </div>
        <div className="provenance__row">
          <span className="k">{t("sidecar.prov_published")}</span>
          <span className="v">{publishedAt}</span>
        </div>
        <div className="provenance__row">
          <span className="k">{t("sidecar.prov_id")}</span>
          <span className="v">{detail.slug}</span>
        </div>
      </div>
    </aside>
  );
}
