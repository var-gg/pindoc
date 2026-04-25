import { Link } from "react-router";
import { ChevronLeft, ChevronRight, ListFilter } from "lucide-react";
import type { Artifact, ArtifactRef } from "../api/client";
import { useI18n } from "../i18n";
import { ArtifactByline } from "./ArtifactByline";
import { PindocMarkdown } from "./Markdown";
import { TrustCard } from "./TrustCard";
import { localizedAreaName } from "./areaLocale";
import { typeChipClass } from "./typeChip";

type Props = {
  detail: Artifact | null;
  emptyMessage: string;
  scope?: DetailScope | null;
};

export type DetailScope = {
  pathLabels: string[];
  mismatch: boolean;
  listHref: string;
  prev?: ArtifactRef;
  next?: ArtifactRef;
  prevHref?: string;
  nextHref?: string;
};

export function ReaderSurface({ detail, emptyMessage, scope }: Props) {
  const { t } = useI18n();

  if (!detail) {
    return (
      <div className="content">
        <div className="surface-stub">
          <p>{emptyMessage}</p>
        </div>
      </div>
    );
  }

  const publishedAt = detail.published_at
    ? new Date(detail.published_at).toLocaleString()
    : "—";
  const areaLabel = localizedAreaName(t, detail.area_slug, detail.area_slug);
  const hasLiveSidecarData =
    Boolean(detail.superseded_by) ||
    (detail.relates_to?.length ?? 0) > 0 ||
    (detail.related_by?.length ?? 0) > 0 ||
    (detail.pins?.length ?? 0) > 0;

  return (
    <main className="content">
      <article className="reader-article">
        <DetailScopeBar scope={scope ?? null} />

        <div className="crumbs">
          <span>{areaLabel}</span>
          <ChevronRight className="lucide" />
          <span>{detail.type}</span>
          <ChevronRight className="lucide" />
          <span className="current">{detail.slug}</span>
        </div>

        <h1 className="art-title">{detail.title}</h1>

        <TrustCard
          meta={detail.artifact_meta}
          pins={detail.pins}
          taskStatus={detail.type === "Task" ? detail.task_meta?.status : undefined}
          recentWarnings={detail.recent_warnings}
        />

        <div className="art-meta">
          <span className={`chip chip--${detail.status}`}>
            <span className={`p-dot p-dot--${detail.status}`} />
            {detail.status}
          </span>
          <span className={typeChipClass(detail.type)}>{detail.type}</span>
          <span className="chip chip--area">{areaLabel}</span>
          <span className="art-meta__sep">·</span>
          <ArtifactByline artifact={detail} />
          <span className="art-meta__sep">·</span>
          <span className="prov">{t("reader.published", publishedAt)}</span>
        </div>

        <div className="art-body">
          <PindocMarkdown
            source={detail.body_markdown}
            collapseStructureSections={hasLiveSidecarData}
          />
        </div>

        <RelatedHint detail={detail} />
      </article>
    </main>
  );
}

function DetailScopeBar({ scope }: { scope: DetailScope | null }) {
  const { t } = useI18n();
  if (!scope) return null;
  const label = scope.pathLabels.join(" / ");
  return (
    <div className="detail-scope-bar">
      <div className="detail-scope-bar__path" aria-label={t("reader.scope_label")}>
        {scope.pathLabels.map((part, i) => (
          <span key={`${part}-${i}`} className="detail-scope-bar__path-part">
            {i > 0 && <ChevronRight className="lucide" aria-hidden="true" />}
            <span>{part}</span>
          </span>
        ))}
      </div>
      {scope.mismatch ? (
        <div className="detail-scope-bar__hint">
          <span>{t("reader.scope_mismatch", label)}</span>
          <Link to={scope.listHref} className="detail-scope-bar__button">
            <ListFilter className="lucide" />
            {t("reader.scope_open_list")}
          </Link>
        </div>
      ) : (
        <div className="detail-scope-bar__nav" aria-label={t("reader.scope_sibling_nav")}>
          {scope.prev && scope.prevHref ? (
            <Link to={scope.prevHref} className="detail-scope-bar__button" title={scope.prev.title}>
              <ChevronLeft className="lucide" />
              {t("reader.scope_prev")}
            </Link>
          ) : (
            <span className="detail-scope-bar__button is-disabled">
              <ChevronLeft className="lucide" />
              {t("reader.scope_prev")}
            </span>
          )}
          {scope.next && scope.nextHref ? (
            <Link to={scope.nextHref} className="detail-scope-bar__button" title={scope.next.title}>
              {t("reader.scope_next")}
              <ChevronRight className="lucide" />
            </Link>
          ) : (
            <span className="detail-scope-bar__button is-disabled">
              {t("reader.scope_next")}
              <ChevronRight className="lucide" />
            </span>
          )}
        </div>
      )}
    </div>
  );
}

function RelatedHint({ detail }: { detail: Artifact }) {
  const { t } = useI18n();
  // Real backlinks need artifact_edges table (Phase 3+). For M1 we show
  // a truthful placeholder so the visual treatment stays and the data
  // gap is obvious to the reader.
  return (
    <div className="backlinks">
      <h4>{t("reader.backlinks_empty_head")}</h4>
      <div style={{ fontSize: 13, color: "var(--fg-3)" }}>
        {t("reader.backlinks_empty", detail.slug)}
      </div>
    </div>
  );
}

export function ArtifactListRow({
  artifact,
  isActive,
  to,
}: {
  artifact: ArtifactRef;
  isActive: boolean;
  to: string;
}) {
  // Kept exported because both Reader and Tasks surfaces render lists.
  return (
    <a href={to} className={`wiki__list-row${isActive ? " is-active" : ""}`}>
      <span className="wiki__chip">{artifact.type}</span>
      <span className="wiki__title">{artifact.title}</span>
    </a>
  );
}
