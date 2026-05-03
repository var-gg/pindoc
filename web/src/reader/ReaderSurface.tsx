import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation } from "react-router";
import { Building2, ChevronDown, ChevronLeft, ChevronRight, Globe2, Languages, ListFilter, Loader2, Lock } from "lucide-react";
import { api, type Artifact, type ArtifactReadState, type ArtifactRef, type VisibilityTier } from "../api/client";
import { useI18n } from "../i18n";
import { estimateReadingTime } from "../utils/readingTime";
import { ArtifactByline } from "./ArtifactByline";
import { BadgePopoverChip } from "./BadgePopoverChip";
import { PindocMarkdown } from "./Markdown";
import { TrustCard } from "./TrustCard";
import { Tooltip } from "./Tooltip";
import { ArtifactAssets } from "./ArtifactAssets";
import { localizedAreaName } from "./areaLocale";
import { projectSurfacePath } from "../readerRoutes";
import type { BadgeFilter } from "./badgeFilters";
import { createReadTracker, readerReadingMetrics, type ReadTrackerFlushReason, type ReadTrackerSnapshot } from "./readTracker";
import { EmptyState } from "./SurfacePrimitives";
import {
  VISIBILITY_TIERS,
  canEditArtifactVisibility,
  normalizeVisibilityTier,
  visibilityChipClass,
  visibilityDescriptionKey,
  visibilityLabelKey,
} from "./visibility";

type Props = {
  detail: Artifact | null;
  emptyMessage: string;
  scope?: DetailScope | null;
  projectSlug?: string;
  orgSlug?: string;
  onApplyBadgeFilter?: (filter: BadgeFilter) => void;
  onApplyAreaFilter?: (areaSlug: string) => void;
  onArtifactUpdated?: () => void;
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
  orgSlug,
  onApplyBadgeFilter,
  onApplyAreaFilter,
  onArtifactUpdated,
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
  const readingMetrics = readerReadingMetrics(readSnapshot);

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

  // Layer 2 read state for the Trust Card "human read" chip. Refetches on
  // artifact change; soft-fails because the chip is purely informational.
  const [serverReadState, setServerReadState] = useState<ArtifactReadState | undefined>();
  useEffect(() => {
    if (!detail?.id || !projectSlug) {
      setServerReadState(undefined);
      return;
    }
    let cancelled = false;
    api.artifactReadState(projectSlug, detail.id)
      .then((s) => { if (!cancelled) setServerReadState(s); })
      .catch(() => { if (!cancelled) setServerReadState(undefined); });
    return () => {
      cancelled = true;
    };
  }, [detail?.id, projectSlug]);

  if (!detail) {
    return (
      <div className="content">
        <div className="surface-panel">
          <EmptyState message={emptyMessage} />
        </div>
      </div>
    );
  }

  const areaLabel = localizedAreaName(t, detail.area_slug, detail.area_slug);
  const legendHref =
    projectSlug
      ? projectSurfacePath(projectSlug, "wiki", "visual-language-reference", orgSlug)
      : undefined;
  // Locale chips are driven by what actually exists: the artifact's own
  // body_locale plus any translation_of edges that resolve to a sibling
  // artifact. Locales without a published translation are omitted — agents
  // initiate new translations via a separate flow, not by clicking a chip
  // for a locale that doesn't yet exist.
  const translationEdges = [
    ...(detail.relates_to ?? []),
    ...(detail.related_by ?? []),
  ].filter((edge) => edge.relation === "translation_of");
  const localeOptions = (() => {
    const seen = new Set<string>();
    const out: Array<{ locale: string; slug: string; title: string; isCurrent: boolean }> = [];
    if (detail.body_locale) {
      seen.add(detail.body_locale);
      out.push({ locale: detail.body_locale, slug: detail.slug, title: detail.title, isCurrent: true });
    }
    for (const edge of translationEdges) {
      const locale = edge.body_locale ?? "";
      if (!locale || seen.has(locale)) continue;
      seen.add(locale);
      out.push({ locale, slug: edge.slug, title: edge.title, isCurrent: false });
    }
    return out;
  })();

  return (
    <main className="content">
      <article className="reader-article">
        <DetailScopeBar scope={scope ?? null} />

        <div className="crumbs">
          {projectSlug && orgSlug && (
            <>
              <span>{orgSlug}</span>
              <ChevronRight className="lucide" />
              <span>{projectSlug}</span>
              <ChevronRight className="lucide" />
            </>
          )}
          <span>{areaLabel}</span>
          <ChevronRight className="lucide" />
          <span>{detail.type}</span>
        </div>

        <h1 className="art-title">{detail.title}</h1>

        <TrustCard
          meta={detail.artifact_meta}
          pins={detail.pins}
          taskStatus={detail.type === "Task" ? detail.task_meta?.status : undefined}
          recentWarnings={detail.recent_warnings}
          readState={serverReadState}
          onApplyFilter={onApplyBadgeFilter}
          legendHref={legendHref}
        />

        <div className="art-meta">
          <BadgePopoverChip
            label={areaLabel}
            description={t("reader.badge_area_tip", areaLabel)}
            className="chip chip--area"
            onApply={onApplyAreaFilter ? () => onApplyAreaFilter(detail.area_slug) : undefined}
            legendHref={legendHref}
          />
          <VisibilityControl
            artifact={detail}
            projectSlug={projectSlug}
            onArtifactUpdated={onArtifactUpdated}
          />
          {projectSlug && localeOptions.length > 0 && (
            <span className="translate-toggle" aria-label={t("reader.body_language")}>
              <Languages className="lucide" aria-hidden="true" />
              {localeOptions.map((option) => (
                option.isCurrent ? (
                  <span
                    key={option.locale}
                    className="translate-toggle__option is-active"
                    aria-current="true"
                    title={t("reader.body_language")}
                  >
                    {option.locale.toUpperCase()}
                  </span>
                ) : (
                  <Tooltip key={option.locale} content={option.title}>
                    <Link
                      to={projectSurfacePath(projectSlug, "wiki", option.slug, orgSlug)}
                      className="translate-toggle__option"
                      aria-label={t("reader.open_translation", option.locale.toUpperCase())}
                    >
                      {option.locale.toUpperCase()}
                    </Link>
                  </Tooltip>
                )
              ))}
            </span>
          )}
          <ArtifactByline artifact={detail} />
          <span className="art-meta__sep">·</span>
          <span className="prov art-reading-metrics">
            {t("reader.reading_metrics", readingEstimate.estimatedMinutes, readingMetrics.readMinutes, readingMetrics.completionPct)}
          </span>
        </div>

        <div className="art-body" ref={bodyRef}>
          <PindocMarkdown
            source={detail.body_markdown}
            projectSlug={projectSlug}
            orgSlug={orgSlug}
            collapseStructureSections
          />
        </div>
        <ArtifactAssets assets={detail.assets ?? []} />
      </article>
    </main>
  );
}

function VisibilityControl({
  artifact,
  projectSlug,
  onArtifactUpdated,
}: {
  artifact: Artifact;
  projectSlug?: string;
  onArtifactUpdated?: () => void;
}) {
  const { t } = useI18n();
  const [visibility, setVisibility] = useState<VisibilityTier>(() => normalizeVisibilityTier(artifact.visibility));
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const editable = Boolean(projectSlug) && canEditArtifactVisibility(artifact.can_edit_visibility);
  const label = t(visibilityLabelKey(visibility));
  const description = t(visibilityDescriptionKey(visibility));
  const ariaLabel = `${t("artifact.visibility_label")}: ${label}`;

  useEffect(() => {
    setVisibility(normalizeVisibilityTier(artifact.visibility));
    setOpen(false);
    setSaving(false);
    setError("");
  }, [artifact.id, artifact.visibility]);

  async function applyVisibility(next: VisibilityTier) {
    if (!projectSlug || saving || next === visibility) {
      setOpen(false);
      return;
    }
    const previous = visibility;
    setVisibility(next);
    setOpen(false);
    setSaving(true);
    setError("");
    try {
      const resp = await api.artifactVisibilityPatch(projectSlug, artifact.slug, { visibility: next });
      setVisibility(normalizeVisibilityTier(resp.visibility));
      onArtifactUpdated?.();
    } catch (err) {
      setVisibility(previous);
      const code = (err as { error_code?: string }).error_code;
      setError(code ? `${t("artifact.visibility_update_failed")} (${code})` : t("artifact.visibility_update_failed"));
    } finally {
      setSaving(false);
    }
  }

  const chip = (
    <>
      {visibilityIcon(visibility)}
      <span>{label}</span>
      {saving ? (
        <Loader2 className="lucide visibility-chip__spinner" aria-hidden="true" />
      ) : editable ? (
        <ChevronDown className="lucide visibility-chip__chevron" aria-hidden="true" />
      ) : null}
    </>
  );

  if (!editable) {
    return (
      <Tooltip content={description}>
        <span className={`${visibilityChipClass(visibility)} is-readonly`} aria-label={ariaLabel}>
          {chip}
        </span>
      </Tooltip>
    );
  }

  return (
    <div
      className="visibility-control"
      onBlur={(event) => {
        const nextFocus = event.relatedTarget as Node | null;
        if (!nextFocus || !event.currentTarget.contains(nextFocus)) {
          setOpen(false);
        }
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          setOpen(false);
        }
      }}
    >
      <Tooltip content={description}>
        <button
          type="button"
          className={`${visibilityChipClass(visibility)} is-editable`}
          aria-label={ariaLabel}
          aria-haspopup="listbox"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          disabled={saving}
        >
          {chip}
        </button>
      </Tooltip>
      {open && (
        <div className="visibility-menu" role="listbox" aria-label={t("artifact.visibility_label")}>
          {VISIBILITY_TIERS.map((tier) => (
            <button
              key={tier}
              type="button"
              className={`visibility-menu__item${tier === visibility ? " is-active" : ""}`}
              role="option"
              aria-selected={tier === visibility}
              disabled={saving}
              onClick={() => void applyVisibility(tier)}
            >
              {visibilityIcon(tier)}
              <span className="visibility-menu__copy">
                <span>{t(visibilityLabelKey(tier))}</span>
                <small>{t(visibilityDescriptionKey(tier))}</small>
              </span>
            </button>
          ))}
        </div>
      )}
      {error && (
        <span className="visibility-toast" role="alert" aria-live="polite">
          {error}
        </span>
      )}
    </div>
  );
}

function visibilityIcon(tier: VisibilityTier) {
  const className = "lucide visibility-chip__icon";
  switch (tier) {
    case "public":
      return <Globe2 className={className} aria-hidden="true" />;
    case "private":
      return <Lock className={className} aria-hidden="true" />;
    case "org":
    default:
      return <Building2 className={className} aria-hidden="true" />;
  }
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
