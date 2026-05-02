import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { CheckCircle2, CheckSquare, ChevronDown, ChevronRight, Download, Filter, GitCommit, Loader2, PanelRightOpen, Sparkles } from "lucide-react";
import { Link, useNavigate } from "react-router";
import { api, type ArtifactReadState, type ChangeGroup, type GitRepoSummary, type TodayResp } from "../api/client";
import { useI18n } from "../i18n";
import { gitCommitPath, isCommitQuery, shortSha } from "../git/routes";
import { projectSurfacePath } from "../readerRoutes";
import { EmptyState } from "./SurfacePrimitives";
import { Tooltip } from "./Tooltip";
import { buildChangeGroupCardView, buildTodayBrief } from "./todayViewModel";
import { TypeCountChip, VisualAreaChip } from "./VisualChips";
import { readStateLabel } from "./readStateLabel";

type Props = {
  projectSlug: string;
  orgSlug: string;
  selectedArea: string | null;
  areaNameBySlug: ReadonlyMap<string, string>;
  onSelectArea: (areaSlug: string) => void;
  selectedArtifactSlug: string | null;
  onSelectArtifact: (slug: string) => void;
};

type KindFilter =
  | "all"
  | "human_trigger"
  | "auto_sync"
  | "maintenance"
  | "system"
  | "verification";

export function Today({
  projectSlug,
  orgSlug,
  selectedArea,
  areaNameBySlug,
  onSelectArea,
  selectedArtifactSlug,
  onSelectArtifact,
}: Props) {
  const { t } = useI18n();
  const [data, setData] = useState<TodayResp | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<KindFilter>("all");
  const [autoOpen, setAutoOpen] = useState(false);
  const [readStates, setReadStates] = useState<Map<string, ArtifactReadState>>(new Map());
  const [gitRepos, setGitRepos] = useState<GitRepoSummary[]>([]);
  const [marking, setMarking] = useState(false);
  const streamRef = useRef<HTMLDivElement | null>(null);
  const autoMarkedRef = useRef<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    setError(null);
    setData(null);
    // UI language is presentation-only here. The server `locale` parameter
    // shapes generated summary copy, but the Reader derives visible Today
    // brief copy from i18n below so KO/EN toggles never refetch or change the
    // change-group population.
    api.changeGroups(projectSlug, { limit: 40, area: selectedArea ?? undefined })
      .then((resp) => {
        if (!cancelled) setData(resp);
      })
      .catch((e) => {
        if (!cancelled) setError(String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [projectSlug, selectedArea]);

  // Layer 2 read states for visual chips on each card. Refetch only when
  // the project changes — area/filter changes never invalidate this map.
  useEffect(() => {
    let cancelled = false;
    api.readStates(projectSlug)
      .then((resp) => {
        if (cancelled) return;
        const m = new Map<string, ArtifactReadState>();
        for (const s of resp.states) m.set(s.artifact_id, s);
        setReadStates(m);
      })
      .catch(() => {
        // Soft-fail: read states are decorative, not gating.
      });
    return () => {
      cancelled = true;
    };
  }, [projectSlug]);

  useEffect(() => {
    let cancelled = false;
    api.gitRepos(projectSlug)
      .then((resp) => { if (!cancelled) setGitRepos(resp.repos); })
      .catch(() => { if (!cancelled) setGitRepos([]); });
    return () => {
      cancelled = true;
    };
  }, [projectSlug]);

  const markAllRead = async (target: number) => {
    if (marking || target <= 0) return;
    setMarking(true);
    try {
      const resp = await api.readMark(projectSlug, target);
      setData((prev) => prev ? {
        ...prev,
        baseline: {
          ...prev.baseline,
          revision_watermark: resp.revision_watermark,
          last_seen_at: new Date().toISOString(),
          defaulted_to_days: undefined,
          fallback_used: undefined,
        },
      } : prev);
    } catch {
      // Soft-fail; user can retry.
    } finally {
      setMarking(false);
    }
  };

  // Auto-mark: when the stream is in the viewport AND there are unread
  // groups (max_revision_id > watermark), advance the watermark to max
  // after a short dwell. Idempotent guard via autoMarkedRef so we don't
  // spam the endpoint on every scroll.
  useEffect(() => {
    if (!data) return;
    const max = data.max_revision_id;
    const watermark = data.baseline.revision_watermark;
    if (max <= 0 || watermark >= max) return;
    if (autoMarkedRef.current === max) return;
    const node = streamRef.current;
    if (!node) return;
    if (typeof IntersectionObserver === "undefined") return;
    let timer: number | null = null;
    const observer = new IntersectionObserver((entries) => {
      const visible = entries.some((e) => e.isIntersecting);
      if (visible && timer === null) {
        timer = window.setTimeout(() => {
          autoMarkedRef.current = max;
          void markAllRead(max);
        }, 1500);
      } else if (!visible && timer !== null) {
        window.clearTimeout(timer);
        timer = null;
      }
    }, { threshold: 0.25 });
    observer.observe(node);
    return () => {
      observer.disconnect();
      if (timer !== null) window.clearTimeout(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data?.max_revision_id, data?.baseline.revision_watermark, projectSlug]);

  const groups = data?.groups ?? [];
  const visibleGroups = useMemo(
    () => groups.filter((group) => {
      if (filter === "all") return true;
      if (filter === "verification") {
        return group.verification_state === "unverified" || group.verification_state === "partially_verified";
      }
      return group.group_kind === filter;
    }),
    [groups, filter],
  );
  const autoGroups = visibleGroups.filter((g) => g.group_kind === "auto_sync" || g.group_kind === "maintenance");
  const primaryGroups = visibleGroups.filter((g) => g.group_kind !== "auto_sync" && g.group_kind !== "maintenance");
  const collapsedAuto = autoGroups.length > 0 && !autoOpen;
  const scopeName = selectedArea
    ? areaNameBySlug.get(selectedArea) ?? selectedArea
    : t("today.scope_all");
  const scopeMeta = selectedArea
    ? t("today.scope_area", scopeName)
    : t("today.scope_all");
  const emptyMessage = selectedArea
    ? t("today.empty_area", scopeName)
    : t("today.empty_all");
  const emptyFilteredMessage = selectedArea
    ? t("today.empty_filtered_area", scopeName)
    : t("today.empty_filtered_all");
  const brief = useMemo(() => data ? buildTodayBrief(data, t) : null, [data, t]);
  const baselineLabel = data?.baseline.last_seen_at
    ? new Date(data.baseline.last_seen_at).toLocaleString()
    : data?.baseline.defaulted_to_days
      ? `last ${data.baseline.defaulted_to_days}d`
      : "current";

  if (error) {
    return (
      <main className="content today-content">
        <div className="reader-state reader-state--error">
          <strong>{t("wiki.error_title")}</strong>
          <p>{error}</p>
        </div>
      </main>
    );
  }
  if (!data) {
    return <main className="content today-content"><div className="reader-state">{t("wiki.loading")}</div></main>;
  }

  return (
    <main className="content today-content">
      <div className="today">
        <header className="today-head">
          <div>
            <div className="today-head__eyebrow">{t("today.eyebrow")}</div>
            <h1>{t("today.title")}</h1>
            <div className="today-head__meta">
              <span>{scopeMeta}</span>
              <span>{t("today.baseline_meta", baselineLabel)}</span>
              <span>{t("today.rev_meta", data.max_revision_id)}</span>
            </div>
          </div>
          <div className="today-head__actions">
            <Tooltip content={t("today.mark_all_read")}>
              <button
                type="button"
                className="today-icon-btn"
                onClick={() => markAllRead(data.max_revision_id)}
                aria-label={t("today.mark_all_read")}
                disabled={marking || data.max_revision_id <= data.baseline.revision_watermark}
              >
                {marking ? <Loader2 className="lucide today-spin" /> : <CheckSquare className="lucide" />}
              </button>
            </Tooltip>
            <ExportButton
              url={api.exportProjectUrl(projectSlug)}
              label={t("today.export_project")}
              iconOnly
            />
          </div>
        </header>

        {brief?.fallbackHint && (
          <div className="today-fallback-hint" role="status">
            {brief.fallbackHint}
          </div>
        )}

        <section className="today-brief" aria-label={t("today.brief")}>
          <div className="today-brief__icon">
            {data.summary.source === "llm" ? <Sparkles className="lucide" /> : <CheckCircle2 className="lucide" />}
          </div>
          <div className="today-brief__body">
            <div className="today-brief__label">{brief?.sourceLabel ?? data.summary.source}</div>
            <h2>{brief?.headline ?? data.summary.headline}</h2>
            <ul>
              {(brief?.bullets ?? data.summary.bullets).slice(0, 3).map((bullet) => (
                <li key={bullet}>{bullet}</li>
              ))}
            </ul>
          </div>
        </section>

        <div className="today-filters" role="group" aria-label={t("today.filters")}>
          <Filter className="lucide today-filters__icon" />
          {(["all", "human_trigger", "verification", "auto_sync", "maintenance", "system"] as KindFilter[]).map((id) => (
            <button
              key={id}
              type="button"
              className={`today-filter${filter === id ? " is-active" : ""}`}
              onClick={() => setFilter(id)}
            >
              {filterLabel(id, t)}
            </button>
          ))}
        </div>

        <div className="today-stream" ref={streamRef}>
          {groups.length === 0 && (
            <EmptyState message={emptyMessage} />
          )}
          {primaryGroups.map((group) => (
            <ChangeGroupCard
              key={group.group_id}
              group={group}
              projectSlug={projectSlug}
              orgSlug={orgSlug}
              areaNameBySlug={areaNameBySlug}
              onSelectArea={onSelectArea}
              selectedArtifactSlug={selectedArtifactSlug}
              onSelectArtifact={onSelectArtifact}
              readState={firstArtifactReadState(group, readStates)}
              defaultRepo={gitRepos[0]}
            />
          ))}
          {autoGroups.length > 0 && (
            <section className="today-auto">
              <button type="button" className="today-auto__toggle" onClick={() => setAutoOpen((v) => !v)}>
                {collapsedAuto ? <ChevronRight className="lucide" /> : <ChevronDown className="lucide" />}
                <span>{t("today.auto_group", autoGroups.length)}</span>
              </button>
              {!collapsedAuto && autoGroups.map((group) => (
                <ChangeGroupCard
                  key={group.group_id}
                  group={group}
                  projectSlug={projectSlug}
                  orgSlug={orgSlug}
                  areaNameBySlug={areaNameBySlug}
                  onSelectArea={onSelectArea}
                  selectedArtifactSlug={selectedArtifactSlug}
                  onSelectArtifact={onSelectArtifact}
                  readState={firstArtifactReadState(group, readStates)}
                  defaultRepo={gitRepos[0]}
                  compact
                />
              ))}
            </section>
          )}
          {groups.length > 0 && visibleGroups.length === 0 && (
            <EmptyState message={emptyFilteredMessage} />
          )}
        </div>
      </div>
    </main>
  );
}

function firstArtifactReadState(
  group: ChangeGroup,
  states: Map<string, ArtifactReadState>,
): ArtifactReadState | undefined {
  const id = group.first_artifact?.id;
  if (!id) return undefined;
  return states.get(id);
}

function commitTargetFromGroup(
  group: ChangeGroup,
  defaultRepo?: GitRepoSummary,
): { repoID: string; sha: string } | null {
  const key = group.grouping_key;
  const value = key.value.trim();
  const explicit = /^([^:\s]+):([0-9a-f]{7,40})$/i.exec(value);
  if (explicit) return { repoID: explicit[1], sha: explicit[2] };
  if ((key.kind.includes("commit") || key.kind.includes("sha")) && isCommitQuery(value) && defaultRepo) {
    return { repoID: defaultRepo.id, sha: value };
  }
  if (isCommitQuery(value) && defaultRepo) {
    return { repoID: defaultRepo.id, sha: value };
  }
  return null;
}

function ChangeGroupCard({
  group,
  projectSlug,
  orgSlug,
  areaNameBySlug,
  onSelectArea,
  selectedArtifactSlug,
  onSelectArtifact,
  readState,
  defaultRepo,
  compact,
}: {
  group: ChangeGroup;
  projectSlug: string;
  orgSlug: string;
  areaNameBySlug: ReadonlyMap<string, string>;
  onSelectArea: (areaSlug: string) => void;
  selectedArtifactSlug: string | null;
  onSelectArtifact: (slug: string) => void;
  readState?: ArtifactReadState;
  defaultRepo?: GitRepoSummary;
  compact?: boolean;
}) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const firstArea = group.areas[0];
  const firstArtifact = group.first_artifact;
  const detailHref = firstArtifact ? projectSurfacePath(projectSlug, "wiki", firstArtifact.slug, orgSlug) : null;
  const isActive = Boolean(firstArtifact && selectedArtifactSlug === firstArtifact.slug);
  const isInteractive = Boolean(firstArtifact);
  const card = useMemo(() => buildChangeGroupCardView(group, t), [group, t]);
  const commitTarget = useMemo(() => commitTargetFromGroup(group, defaultRepo), [group, defaultRepo]);
  const readLabel = readStateLabel(readState?.read_state, t);
  function openDetail() {
    if (detailHref) navigate(detailHref);
  }
  function selectArtifact() {
    if (firstArtifact) onSelectArtifact(firstArtifact.slug);
  }
  function onKeyDown(e: KeyboardEvent<HTMLElement>) {
    if (!isInteractive) return;
    const target = e.target as HTMLElement | null;
    if (target?.closest("a, button, input, textarea, select, [contenteditable='true']")) return;
    const openKey = e.key.toLowerCase() === "o" || (e.key === "Enter" && e.shiftKey);
    if (openKey) {
      e.preventDefault();
      openDetail();
      return;
    }
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      selectArtifact();
    }
  }
  return (
    <article
      className={`change-card${compact ? " change-card--compact" : ""}${isInteractive ? " change-card--interactive" : ""}${isActive ? " is-active" : ""}`}
      tabIndex={isInteractive ? 0 : undefined}
      role={isInteractive ? "button" : undefined}
      aria-selected={isInteractive ? isActive : undefined}
      onClick={selectArtifact}
      onDoubleClick={openDetail}
      onKeyDown={onKeyDown}
    >
      <div className="change-card__top">
        <span className={`change-kind change-kind--${group.group_kind}`}>{card.kindLabel}</span>
        <span className={`change-importance change-importance--${group.importance.level}`}>{card.importanceLabel}</span>
        <span>{t("today.revision_count", group.revision_count)}</span>
        <span>{t("today.artifact_count", group.artifact_count)}</span>
        {group.type_counts && group.type_counts.length > 0 && (
          <span className="change-card__types" aria-label={t("today.type_distribution")}>
            {group.type_counts.slice(0, 5).map((row) => (
              <TypeCountChip key={row.type} type={row.type} count={row.count} />
            ))}
          </span>
        )}
        {firstArtifact && (
          <span className="change-card__inspect" aria-label={t("today.card_select_hint")}>
            <PanelRightOpen className="lucide" aria-hidden="true" />
          </span>
        )}
      </div>
      <h2>{card.title}</h2>
      {card.bullets.length > 0 && (
        <ul className="change-card__summary">
          {card.bullets.map((bullet) => (
            <li key={bullet}>{bullet}</li>
          ))}
        </ul>
      )}
      <div className="change-card__meta">
        <span>{new Date(group.time_end).toLocaleString()}</span>
        <span>{card.verificationLabel}</span>
        {firstArtifact && (
          <span
            className={`change-card__read-state change-card__read-state--${readState?.read_state ?? "unseen"}`}
            title={readState?.last_seen_at
              ? `${readLabel} · ${new Date(readState.last_seen_at).toLocaleString()}`
              : readLabel}
          >
            {readLabel}
          </span>
        )}
      </div>
      <div className="change-card__areas" onClick={(e) => e.stopPropagation()}>
        {group.areas.map((area) => (
          <VisualAreaChip
            key={area}
            areaSlug={area}
            label={areaNameBySlug.get(area) ?? area}
            onClick={() => onSelectArea(area)}
          />
        ))}
      </div>
      <div className="change-card__actions" onClick={(e) => e.stopPropagation()}>
        {commitTarget && (
          <Link
            className="change-card__commit-link"
            to={gitCommitPath(projectSlug, commitTarget.repoID, commitTarget.sha)}
          >
            <GitCommit className="lucide" />
            <span>{t("today.open_commit", shortSha(commitTarget.sha))}</span>
          </Link>
        )}
        {firstArea && <Link to={`${projectSurfacePath(projectSlug, "wiki", undefined, orgSlug)}?area=${encodeURIComponent(firstArea)}`}>{t("today.open_area")}</Link>}
        {firstArea && <ExportButton url={api.exportProjectUrl(projectSlug, { area: firstArea })} label={t("today.export_area")} />}
      </div>
    </article>
  );
}

function ExportButton({ url, label, iconOnly }: { url: string; label: string; iconOnly?: boolean }) {
  const [loading, setLoading] = useState(false);
  async function onClick() {
    if (loading) return;
    setLoading(true);
    try {
      const resp = await fetch(url);
      if (!resp.ok) throw new Error(`${resp.status} ${resp.statusText}`);
      const blob = await resp.blob();
      const disposition = resp.headers.get("Content-Disposition") ?? "";
      const match = /filename="?([^";]+)"?/i.exec(disposition);
      const filename = match?.[1] ?? "pindoc-export.zip";
      const objectUrl = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = objectUrl;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
    } finally {
      setLoading(false);
    }
  }
  if (iconOnly) {
    return (
      <Tooltip content={label}>
        <button type="button" className="today-icon-btn" onClick={onClick} aria-label={label} disabled={loading}>
          {loading ? <Loader2 className="lucide today-spin" /> : <Download className="lucide" />}
        </button>
      </Tooltip>
    );
  }
  return (
    <button type="button" className="today-link-btn" onClick={onClick} disabled={loading}>
      {loading ? <Loader2 className="lucide today-spin" /> : <Download className="lucide" />}
      <span>{label}</span>
    </button>
  );
}

function filterLabel(id: KindFilter, t: (key: string, ...args: Array<string | number>) => string): string {
  switch (id) {
    case "all":
      return t("today.filter_all");
    case "verification":
      return t("today.filter_verification");
    case "human_trigger":
      return t("today.kind_human_trigger");
    case "auto_sync":
      return t("today.kind_auto_sync");
    case "maintenance":
      return t("today.kind_maintenance");
    case "system":
      return t("today.kind_system");
  }
}
