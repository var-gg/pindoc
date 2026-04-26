import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation } from "react-router";
import { ChevronLeft, ChevronRight, Languages, ListFilter } from "lucide-react";
import { api, type Artifact, type ArtifactRef } from "../api/client";
import { useI18n } from "../i18n";
import { estimateReadingTime } from "../utils/readingTime";
import { ArtifactByline } from "./ArtifactByline";
import { BadgePopoverChip } from "./BadgePopoverChip";
import { PindocMarkdown } from "./Markdown";
import { TrustCard } from "./TrustCard";
import { Tooltip } from "./Tooltip";
import { localizedAreaName } from "./areaLocale";
import type { BadgeFilter } from "./badgeFilters";
import { createReadTracker, type ReadTrackerFlushReason, type ReadTrackerSnapshot } from "./readTracker";
import { EmptyState } from "./SurfacePrimitives";
import { typeChipClass } from "./typeChip";

type Props = {
  detail: Artifact | null;
  emptyMessage: string;
  scope?: DetailScope | null;
  projectSlug?: string;
  onApplyBadgeFilter?: (filter: BadgeFilter) => void;
  onApplyAreaFilter?: (areaSlug: string) => void;
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

export function ReaderSurface({
  detail,
  emptyMessage,
  scope,
  projectSlug,
  onApplyBadgeFilter,
  onApplyAreaFilter,
}: Props) {
  const { t } = useI18n();
  const location = useLocation();
  const bodyRef = useRef<HTMLDivElement | null>(null);
  const [readSnapshot, setReadSnapshot] = useState<ReadTrackerSnapshot>(emptyReadSnapshot);
  const activeTranslateLocale = new URLSearchParams(location.search).get("translate") ?? "";
  const highlightedLocale = activeTranslateLocale || detail?.body_locale || "";
  const readingEstimate = useMemo(
    () => estimateReadingTime(detail?.body_markdown ?? "", highlightedLocale || detail?.body_locale),
    [detail?.body_markdown, detail?.body_locale, highlightedLocale],
  );
  const completionPct = Math.min(100, Math.round(readSnapshot.scrollMaxPct * 100));
  const readMinutes = formatReadMinutes(readSnapshot.activeSeconds);

  useEffect(() => {
    setReadSnapshot(emptyReadSnapshot());
    const bodyElement = bodyRef.current;
    if (!detail?.id || !projectSlug || !bodyElement) return;
    const tracker = createReadTracker({
      artifactId: detail.id,
      locale: highlightedLocale || detail.body_locale,
      bodyElement,
      onUpdate: setReadSnapshot,
      flush: async (payload, reason: ReadTrackerFlushReason) => {
        await api.readEvent(projectSlug, payload, {
          keepalive: reason === "hidden" || reason === "beforeunload",
        }).catch(() => undefined);
      },
    });
    return () => {
      tracker.stop("route");
    };
  }, [detail?.id, detail?.body_locale, highlightedLocale, projectSlug]);

  if (!detail) {
    return (
      <div className="content">
        <div className="surface-panel">
          <EmptyState message={emptyMessage} />
        </div>
      </div>
    );
  }

  const publishedAt = detail.published_at
    ? new Date(detail.published_at).toLocaleString()
    : "—";
  const areaLabel = localizedAreaName(t, detail.area_slug, detail.area_slug);
  const legendHref =
    projectSlug
      ? `/p/${projectSlug}/wiki/visual-language-reference`
      : undefined;
  const translationEdges = [
    ...(detail.relates_to ?? []),
    ...(detail.related_by ?? []),
  ].filter((edge) => edge.relation === "translation_of");
  const translateLocales = ["en", "ko", "ja", "hi"];
  const translateHref = (locale: string) => {
    const params = new URLSearchParams(location.search);
    if (locale === detail.body_locale) {
      params.delete("translate");
    } else {
      params.set("translate", locale);
    }
    const query = params.toString();
    return `${location.pathname}${query ? `?${query}` : ""}`;
  };

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
          onApplyFilter={onApplyBadgeFilter}
          legendHref={legendHref}
        />

        <div className="art-meta">
          {detail.type === "Task" && (
            <>
              <span className={`chip chip--${detail.status}`}>
                <span className={`p-dot p-dot--${detail.status}`} />
                {detail.status}
              </span>
              <span className={typeChipClass(detail.type)}>{detail.type}</span>
            </>
          )}
          <BadgePopoverChip
            label={areaLabel}
            description={t("reader.badge_area_tip", areaLabel)}
            className="chip chip--area"
            onApply={onApplyAreaFilter ? () => onApplyAreaFilter(detail.area_slug) : undefined}
            legendHref={legendHref}
          />
          {detail.body_locale ? (
            <Tooltip content={t("reader.body_language")}>
              <span className="chip chip--area">
                lang: {detail.body_locale}
              </span>
            </Tooltip>
          ) : null}
          <span className="translate-toggle" aria-label="Translation target">
            <Languages className="lucide" aria-hidden="true" />
            {translateLocales.map((locale) => (
              <Link
                key={locale}
                to={translateHref(locale)}
                className={`translate-toggle__option${highlightedLocale === locale ? " is-active" : ""}`}
                aria-current={highlightedLocale === locale ? "true" : undefined}
                aria-label={t("reader.translate_to", locale.toUpperCase())}
              >
                {locale.toUpperCase()}
              </Link>
            ))}
          </span>
          {projectSlug && translationEdges.map((edge) => (
            <Tooltip key={`translation-${edge.artifact_id}`} content={edge.title}>
              <Link
                to={`/p/${projectSlug}/wiki/${edge.slug}`}
                className="chip chip--area"
              >
                translation
              </Link>
            </Tooltip>
          ))}
          <span className="art-meta__sep">·</span>
          <ArtifactByline artifact={detail} />
          <span className="art-meta__sep">·</span>
          <span className="prov">{t("reader.published", publishedAt)}</span>
          <span className="art-meta__sep">·</span>
          <span className="prov art-reading-metrics">
            {t("reader.reading_metrics", readingEstimate.estimatedMinutes, readMinutes, completionPct)}
          </span>
        </div>

        <div className="art-body" ref={bodyRef}>
          <PindocMarkdown
            source={detail.body_markdown}
            projectSlug={projectSlug}
            collapseStructureSections
          />
        </div>
      </article>
    </main>
  );
}

function emptyReadSnapshot(): ReadTrackerSnapshot {
  return {
    startedAtMs: 0,
    endedAtMs: 0,
    activeSeconds: 0,
    idleSeconds: 0,
    scrollMaxPct: 0,
    visible: true,
    intersecting: false,
    idle: false,
  };
}

function formatReadMinutes(seconds: number): string {
  if (seconds <= 0) return "0";
  if (seconds < 60) return "<1";
  return String(Math.floor(seconds / 60));
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
            <Tooltip content={scope.prev.title}>
              <Link to={scope.prevHref} className="detail-scope-bar__button">
                <ChevronLeft className="lucide" />
                {t("reader.scope_prev")}
              </Link>
            </Tooltip>
          ) : (
            <span className="detail-scope-bar__button is-disabled">
              <ChevronLeft className="lucide" />
              {t("reader.scope_prev")}
            </span>
          )}
          {scope.next && scope.nextHref ? (
            <Tooltip content={scope.next.title}>
              <Link to={scope.nextHref} className="detail-scope-bar__button">
                {t("reader.scope_next")}
                <ChevronRight className="lucide" />
              </Link>
            </Tooltip>
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
      <span className="wiki__title">{artifact.title}</span>
    </a>
  );
}
