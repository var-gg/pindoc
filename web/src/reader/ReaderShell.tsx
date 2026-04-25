import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { X } from "lucide-react";
import type { Aggregate } from "./useReaderData";
import { api, type Artifact, type ArtifactRef, type Area } from "../api/client";
import { useI18n } from "../i18n";
import { CmdK } from "./CmdK";
import { ReaderSurface, type DetailScope } from "./ReaderSurface";
import { Sidebar } from "./Sidebar";
import { Sidecar } from "./Sidecar";
import { TopNav } from "./TopNav";
import { initTheme, setTheme, type Theme } from "./theme";
import { useReaderData } from "./useReaderData";
import { typeChipClass } from "./typeChip";
import { initReaderWidth, setReaderWidth as applyReaderWidth, type ReaderWidth } from "./readerWidth";
import { localizedAreaName } from "./areaLocale";
import {
  appendBadgeFilters,
  artifactMatchesBadgeFilters,
  badgeFilterKeyLabel,
  clearAllBadgeFilterParams,
  clearBadgeFilterParam,
  readBadgeFilters,
  setBadgeFilterParam,
  type BadgeFilter,
  type BadgeFilterKey,
} from "./badgeFilters";
import "../styles/reader.css";

export type ReaderView = "reader" | "inbox" | "graph" | "tasks";

// surfaceAllows decides which artifacts a Surface's natural set contains
// (Decision `decision-reader-ia-hierarchy`). Wiki = everything except Task;
// Tasks = only Task; Inbox/Graph are stubs and currently pass through the
// full list (real Inbox queue lands with the Review/Risk split; Graph's
// sub-graph filtering ships with the React-ification in M1.5).
function surfaceAllows(view: ReaderView, a: ArtifactRef): boolean {
  if (view === "tasks") return a.type === "Task";
  if (view === "reader") return a.type !== "Task";
  return true;
}

// inboxStubCount returns 0 (the Inbox surface is a stub) and logs once so
// engineers diffing the nav badge realise the zero is structural, not a
// load bug. Replace with a real count when the Review/Risk surfaces ship
// (Task `reader-trust-card-...` Open issue — split from this component).
let inboxStubWarned = false;
function inboxStubCount(): number {
  if (!inboxStubWarned) {
    inboxStubWarned = true;
    // eslint-disable-next-line no-console
    console.warn(
      "[pindoc] ReaderShell inboxCount is hard-coded to 0 — Inbox surface is a stub. See Task reader-trust-card-*.",
    );
  }
  return 0;
}

type Props = {
  view: ReaderView;
};

export function ReaderShell({ view }: Props) {
  const { project = "", locale = "", slug } = useParams<{ project: string; locale?: string; slug?: string }>();
  const { t } = useI18n();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [showTemplates, setShowTemplates] = useState(false);
  const state = useReaderData(project, slug, showTemplates);
  const [theme, setThemeState] = useState<Theme>(() => initTheme());
  const [readerWidth, setReaderWidthState] = useState<ReaderWidth>(() => initReaderWidth());
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [taskInspectorSlug, setTaskInspectorSlug] = useState<string | null>(null);
  const [taskInspectorDetail, setTaskInspectorDetail] = useState<Artifact | null>(null);
  const [taskInspectorLoading, setTaskInspectorLoading] = useState(false);
  const [taskInspectorReloadNonce, setTaskInspectorReloadNonce] = useState(0);
  // Surface·Type·Area 3축: Surface is owned by the URL segment (wiki|tasks|
  // graph|inbox — already carried in `view` prop). Area and Type are the
  // secondary filters layered on top and survive round-trips through the
  // URL search params (`?area=ui&type=Decision`) so links are shareable.
  // In Tasks surface Type is locked to "Task" (see surfaceAllows) and the
  // search param is not written.
  const [selectedArea, setSelectedArea] = useState<string | null>(
    () => searchParams.get("area"),
  );
  const [selectedType, setSelectedTypeState] = useState<string | null>(
    () => (view === "tasks" ? null : searchParams.get("type")),
  );
  const badgeFilters = useMemo(
    () => readBadgeFilters(searchParams, t),
    [searchParams, t],
  );

  // Surface transition policy (Decision `decision-reader-ia-hierarchy`):
  // Area carries across surfaces (exploration continuity — "UI area in
  // Tasks → UI area in Wiki"). Type resets because it's meaningful only
  // within the Wiki surface; Tasks surface locks it to "Task" implicitly.
  useEffect(() => {
    setSelectedTypeState(view === "tasks" ? null : searchParams.get("type"));
    // intentionally watching view only: hydrating from searchParams here
    // would fight the sync effect below on every param write.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view]);

  // URL search param sync (acceptance #10). Keep `?area=` / `?type=` in
  // lockstep with state so bookmarking or sharing a URL restores the exact
  // filter set. We use replace so filter toggles don't pollute browser
  // history.
  useEffect(() => {
    const next = new URLSearchParams(searchParams);
    if (selectedArea) next.set("area", selectedArea);
    else next.delete("area");
    if (selectedType && view !== "tasks") next.set("type", selectedType);
    else next.delete("type");
    if (next.toString() !== searchParams.toString()) {
      setSearchParams(next, { replace: true });
    }
  }, [selectedArea, selectedType, view, searchParams, setSearchParams]);

  const baseRoute = `/p/${project}/${locale}/${view === "tasks" ? "tasks" : "wiki"}`;

  function writeSearchParams(next: URLSearchParams, opts?: { toList?: boolean }) {
    const qs = next.toString();
    if (opts?.toList && slug) {
      navigate(`${baseRoute}${qs ? `?${qs}` : ""}`, { replace: true });
      return;
    }
    setSearchParams(next, { replace: true });
  }

  // In Wiki detail mode, Area selection is a live scope for the detail
  // header and sibling nav; the article stays mounted. Other surfaces
  // still return to their list/board so the filter effect is obvious.
  function handleSelectArea(next: string | null) {
    setSelectedArea(next);
    if (slug && view !== "reader") navigate(baseRoute);
  }
  function handleSelectType(next: string | null) {
    setSelectedTypeState((prev) => (prev === next ? null : next));
    if (slug) navigate(baseRoute);
  }
  function clearAreaFilter() {
    setSelectedArea(null);
    const next = new URLSearchParams(searchParams);
    next.delete("area");
    writeSearchParams(next, { toList: true });
  }
  function clearTypeFilter() {
    setSelectedTypeState(null);
    const next = new URLSearchParams(searchParams);
    next.delete("type");
    writeSearchParams(next, { toList: true });
  }
  function clearFilters() {
    setSelectedArea(null);
    setSelectedTypeState(null);
    const next = new URLSearchParams(searchParams);
    next.delete("area");
    next.delete("type");
    clearAllBadgeFilterParams(next);
    writeSearchParams(next, { toList: true });
  }
  function applyBadgeFilter(filter: BadgeFilter) {
    const next = new URLSearchParams(searchParams);
    setBadgeFilterParam(next, filter);
    if (selectedArea) next.set("area", selectedArea);
    if (selectedType && view !== "tasks") next.set("type", selectedType);
    writeSearchParams(next, { toList: true });
  }
  function clearBadgeFilter(key: BadgeFilterKey) {
    const next = new URLSearchParams(searchParams);
    clearBadgeFilterParam(next, key);
    writeSearchParams(next);
  }
  function applyAreaFilterFromBadge(areaSlug: string) {
    setSelectedArea(areaSlug);
    const next = new URLSearchParams(searchParams);
    next.set("area", areaSlug);
    if (selectedType && view !== "tasks") next.set("type", selectedType);
    writeSearchParams(next, { toList: true });
  }

  // ⌘K listener — global-level so palette opens from any surface.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setPaletteOpen(true);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    if (view !== "tasks") return;
    function onKey(e: KeyboardEvent) {
      if (paletteOpen || e.key !== "Escape") return;
      const target = e.target as HTMLElement | null;
      if (target?.closest("input, textarea, select, [contenteditable='true']")) return;
      if (!selectedArea && !selectedType && badgeFilters.length === 0) return;
      e.preventDefault();
      clearFilters();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [view, paletteOpen, selectedArea, selectedType, badgeFilters.length, slug, baseRoute, navigate]);

  const toggleTheme = () => {
    const next: Theme = theme === "dark" ? "light" : "dark";
    setTheme(next);
    setThemeState(next);
  };

  const changeReaderWidth = (next: ReaderWidth) => {
    applyReaderWidth(next);
    setReaderWidthState(next);
  };

  useEffect(() => {
    if (view !== "tasks" || slug || !taskInspectorSlug) {
      setTaskInspectorDetail(null);
      setTaskInspectorLoading(false);
      return;
    }
    let cancelled = false;
    setTaskInspectorLoading(true);
    api.artifact(project, taskInspectorSlug)
      .then((artifact) => {
        if (!cancelled) setTaskInspectorDetail(artifact);
      })
      .catch(() => {
        if (!cancelled) setTaskInspectorDetail(null);
      })
      .finally(() => {
        if (!cancelled) setTaskInspectorLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [view, slug, project, taskInspectorSlug, taskInspectorReloadNonce]);

  // Hooks must stay at top level (rules-of-hooks). Filter against a
  // possibly-empty list when we haven't loaded yet so we never call
  // this hook conditionally.
  //
  // Pipeline ordering (Decision `decision-reader-ia-hierarchy`):
  //   baseList → surfaceList (Surface defines the natural set)
  //            → filteredArtifacts (Type + Area + Badge layered on top)
  // Counters derive from surfaceList with the *other* axis still applied
  // so the selected axis behaves as "show everything" — Linear/Issues
  // convention.
  const baseList =
    state.kind === "ready" ? state.data.artifacts : ([] as ArtifactRef[]);
  const surfaceList = useMemo(
    () => baseList.filter((a) => surfaceAllows(view, a)),
    [baseList, view],
  );
  const filteredArtifacts = useMemo(
    () =>
      surfaceList.filter((a) => {
        if (selectedArea && a.area_slug !== selectedArea) return false;
        if (selectedType && a.type !== selectedType) return false;
        if (!artifactMatchesBadgeFilters(a, badgeFilters)) return false;
        return true;
      }),
    [surfaceList, selectedArea, selectedType, badgeFilters],
  );
  const areaCountMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const a of surfaceList) {
      if (selectedType && a.type !== selectedType) continue;
      if (!artifactMatchesBadgeFilters(a, badgeFilters)) continue;
      map.set(a.area_slug, (map.get(a.area_slug) ?? 0) + 1);
    }
    return map;
  }, [surfaceList, selectedType, badgeFilters]);
  const typeCounts = useMemo<Aggregate[]>(() => {
    const map = new Map<string, number>();
    for (const a of surfaceList) {
      if (selectedArea && a.area_slug !== selectedArea) continue;
      if (!artifactMatchesBadgeFilters(a, badgeFilters)) continue;
      map.set(a.type, (map.get(a.type) ?? 0) + 1);
    }
    return Array.from(map, ([key, count]) => ({ key, count })).sort(
      (a, b) => b.count - a.count,
    );
  }, [surfaceList, selectedArea, badgeFilters]);

  if (state.kind === "loading") {
    return <div className="reader-state">{t("wiki.loading")}</div>;
  }
  if (state.kind === "error") {
    return (
      <div className="reader-state reader-state--error">
        <strong>{t("wiki.error_title")}</strong>
        <p>{state.message}</p>
        <p>
          {t("wiki.error_hint_prefix")} <code>{t("wiki.error_hint_cmd")}</code>{" "}
          {t("wiki.error_hint_suffix")}
        </p>
      </div>
    );
  }

  const { project: projectData, areas, detail, agents, users, authMode } = state.data;
  const reload = state.reload;
  const sidecarDetail =
    view === "reader"
      ? detail
      : view === "tasks"
        ? detail ?? taskInspectorDetail
        : null;
  const sidecarEmptyMessage =
    view === "tasks" && !detail
      ? taskInspectorLoading
        ? t("tasks.inspector_loading")
        : t("tasks.inspector_empty")
      : undefined;
  function handleTaskInspectorUpdated() {
    reload();
    setTaskInspectorReloadNonce((n) => n + 1);
  }
  // Override area.artifact_count with the Surface-aware recomputation so
  // the sidebar badge matches the list the user is actually looking at.
  // Acceptance #6/#7: counters respect current Surface + the *other* filter
  // axis. Area rows missing from the map (zero-hit in this Surface) get 0
  // rather than being dropped — keeping them visible so the user can still
  // click into them if they reset Type.
  const displayAreas: Area[] = areas.map((a) => ({
    ...a,
    artifact_count: areaCountMap.get(a.slug) ?? 0,
  }));
  const areaNameBySlug = new Map(
    displayAreas.map((a) => [a.slug, localizedAreaName(t, a.slug, a.name)]),
  );
  const areaPathBySlug = buildAreaPathMap(displayAreas, areaNameBySlug);

  return (
    <div className="app-shell">
      <TopNav
        project={projectData}
        theme={theme}
        onToggleTheme={toggleTheme}
        onOpenPalette={() => setPaletteOpen(true)}
        onToggleMenu={() => setMenuOpen((v) => !v)}
        inboxCount={inboxStubCount()}
        readerWidth={readerWidth}
        onChangeReaderWidth={changeReaderWidth}
      />
      <div className="main">
        <Sidebar
          areas={displayAreas}
          types={typeCounts}
          agents={agents}
          selectedArea={selectedArea}
          onSelectArea={handleSelectArea}
          selectedType={selectedType}
          onSelectType={handleSelectType}
          typeFilterLocked={view === "tasks"}
          open={menuOpen}
          showTemplates={showTemplates}
          onToggleTemplates={() => setShowTemplates((v) => !v)}
        />
        <Body
          view={view}
          projectSlug={project}
          projectLocale={locale}
          detail={detail}
          list={filteredArtifacts}
          allList={surfaceList}
          currentSlug={slug}
          selectedArea={selectedArea}
          selectedType={selectedType}
          badgeFilters={badgeFilters}
          areaNameBySlug={areaNameBySlug}
          areaPathBySlug={areaPathBySlug}
          selectedTaskSlug={taskInspectorSlug}
          onSelectTask={setTaskInspectorSlug}
          onClearAreaFilter={clearAreaFilter}
          onClearTypeFilter={clearTypeFilter}
          onClearBadgeFilter={clearBadgeFilter}
          onClearFilters={clearFilters}
          onApplyBadgeFilter={applyBadgeFilter}
          onApplyAreaFilter={applyAreaFilterFromBadge}
        />
        <Sidecar
          projectSlug={project}
          detail={sidecarDetail}
          emptyMessage={sidecarEmptyMessage}
          authMode={authMode}
          agents={agents}
          users={users}
          onArtifactUpdated={view === "tasks" ? handleTaskInspectorUpdated : reload}
        />
      </div>
      <CmdK projectSlug={project} open={paletteOpen} onClose={() => setPaletteOpen(false)} />
    </div>
  );
}

function buildAreaPathMap(
  areas: Area[],
  areaNameBySlug: ReadonlyMap<string, string>,
): ReadonlyMap<string, string[]> {
  const bySlug = new Map(areas.map((a) => [a.slug, a]));
  const out = new Map<string, string[]>();
  for (const area of areas) {
    const path: string[] = [];
    const seen = new Set<string>();
    let current: Area | undefined = area;
    while (current && !seen.has(current.slug)) {
      seen.add(current.slug);
      path.unshift(areaNameBySlug.get(current.slug) ?? current.name ?? current.slug);
      current = current.parent_slug ? bySlug.get(current.parent_slug) : undefined;
    }
    out.set(area.slug, path.length > 0 ? path : [areaNameBySlug.get(area.slug) ?? area.slug]);
  }
  return out;
}

function buildDetailScope({
  detail,
  list,
  selectedArea,
  selectedType,
  badgeFilters,
  areaNameBySlug,
  areaPathBySlug,
  baseRoute,
}: {
  detail: Artifact;
  list: ArtifactRef[];
  selectedArea: string | null;
  selectedType: string | null;
  badgeFilters: BadgeFilter[];
  areaNameBySlug: ReadonlyMap<string, string>;
  areaPathBySlug: ReadonlyMap<string, string[]>;
  baseRoute: string;
}): DetailScope {
  const scopeArea = selectedArea ?? detail.area_slug;
  const pathLabels =
    areaPathBySlug.get(scopeArea) ?? [areaNameBySlug.get(scopeArea) ?? scopeArea];
  const mismatch = Boolean(selectedArea && detail.area_slug !== selectedArea);
  const listHref = filteredReaderHref(baseRoute, undefined, selectedArea ?? scopeArea, selectedType, badgeFilters);
  if (mismatch) {
    return { pathLabels, mismatch, listHref };
  }

  const siblings = list
    .filter((a) => a.area_slug === scopeArea)
    .slice()
    .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime());
  const idx = siblings.findIndex((a) => a.slug === detail.slug);
  const prev = idx > 0 ? siblings[idx - 1] : undefined;
  const next = idx >= 0 && idx < siblings.length - 1 ? siblings[idx + 1] : undefined;
  return {
    pathLabels,
    mismatch: false,
    listHref,
    prev,
    next,
    prevHref: prev ? filteredReaderHref(baseRoute, prev.slug, selectedArea, selectedType, badgeFilters) : undefined,
    nextHref: next ? filteredReaderHref(baseRoute, next.slug, selectedArea, selectedType, badgeFilters) : undefined,
  };
}

function filteredReaderHref(
  baseRoute: string,
  slug: string | undefined,
  selectedArea: string | null,
  selectedType: string | null,
  badgeFilters: BadgeFilter[] = [],
): string {
  const params = new URLSearchParams();
  if (selectedArea) params.set("area", selectedArea);
  if (selectedType) params.set("type", selectedType);
  appendBadgeFilters(params, badgeFilters);
  const qs = params.toString();
  return `${baseRoute}${slug ? `/${slug}` : ""}${qs ? `?${qs}` : ""}`;
}

function Body({
  view,
  projectSlug,
  projectLocale,
  detail,
  list,
  allList,
  currentSlug,
  selectedArea,
  selectedType,
  badgeFilters,
  areaNameBySlug,
  areaPathBySlug,
  selectedTaskSlug,
  onSelectTask,
  onClearAreaFilter,
  onClearTypeFilter,
  onClearBadgeFilter,
  onClearFilters,
  onApplyBadgeFilter,
  onApplyAreaFilter,
}: {
  view: ReaderView;
  projectSlug: string;
  projectLocale: string;
  detail: Artifact | null;
  list: ArtifactRef[];
  allList: ArtifactRef[];
  currentSlug: string | undefined;
  selectedArea: string | null;
  selectedType: string | null;
  badgeFilters: BadgeFilter[];
  areaNameBySlug: ReadonlyMap<string, string>;
  areaPathBySlug: ReadonlyMap<string, string[]>;
  selectedTaskSlug: string | null;
  onSelectTask: (slug: string) => void;
  onClearAreaFilter: () => void;
  onClearTypeFilter: () => void;
  onClearBadgeFilter: (key: BadgeFilterKey) => void;
  onClearFilters: () => void;
  onApplyBadgeFilter: (filter: BadgeFilter) => void;
  onApplyAreaFilter: (areaSlug: string) => void;
}) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const baseRoute = `/p/${projectSlug}/${projectLocale}/${view === "tasks" ? "tasks" : "wiki"}`;
  const detailScope = detail && view === "reader"
    ? buildDetailScope({
        detail,
        list: allList,
        selectedArea,
        selectedType,
        badgeFilters,
        areaNameBySlug,
        areaPathBySlug,
        baseRoute,
      })
    : null;

  useEffect(() => {
    if (view !== "reader" || !detailScope || detailScope.mismatch) return;
    const scope = detailScope;
    function onKey(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      if (target?.closest("input, textarea, select, [contenteditable='true']")) return;
      if (e.key === "[" && scope.prevHref) {
        e.preventDefault();
        navigate(scope.prevHref);
      }
      if (e.key === "]" && scope.nextHref) {
        e.preventDefault();
        navigate(scope.nextHref);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [view, detailScope, navigate]);

  if (view === "graph") {
    return (
      <main className="content">
        <div className="surface-stub">
          <h1>{t("nav.graph")}</h1>
          <p>{t("wiki.stub_graph")}</p>
          <p>
            <Link to="/ui/reader">{t("wiki.stub_graph_preview")}</Link>
          </p>
        </div>
      </main>
    );
  }
  if (view === "inbox") {
    return (
      <main className="content">
        <div className="surface-stub">
          <h1>{t("nav.inbox")}</h1>
          <p>{t("wiki.stub_inbox")}</p>
        </div>
      </main>
    );
  }

  // When an artifact is selected (detail != null) we render the Reader
  // surface to match the handoff prototype. Otherwise fall back to a
  // compact list so the user can pick one — Claude Design's original
  // flow expects ⌘K navigation here, but a visible list keeps the
  // empty-first-render less mysterious.
  if (detail) {
    return (
      <ReaderSurface
        detail={detail}
        emptyMessage={view === "tasks" ? t("wiki.empty_tasks_detail") : t("wiki.empty_detail")}
        scope={detailScope}
        projectSlug={projectSlug}
        projectLocale={projectLocale}
        onApplyBadgeFilter={onApplyBadgeFilter}
        onApplyAreaFilter={onApplyAreaFilter}
      />
    );
  }

  const empty =
    view === "tasks"
      ? t("wiki.empty_tasks")
      : selectedType
      ? t("reader.empty_filtered", selectedType)
      : t("wiki.empty_list");

  if (view === "tasks") {
    return (
      <TasksKanban
        projectSlug={projectSlug}
        projectLocale={projectLocale}
        list={list}
        allList={allList}
        currentSlug={currentSlug}
        empty={empty}
        selectedArea={selectedArea}
        badgeFilters={badgeFilters}
        areaNameBySlug={areaNameBySlug}
        selectedTaskSlug={selectedTaskSlug}
        onSelectTask={onSelectTask}
        onClearAreaFilter={onClearAreaFilter}
        onClearBadgeFilter={onClearBadgeFilter}
        onClearFilters={onClearFilters}
      />
    );
  }

  return (
    <main className="content">
      <div className="reader-article">
        <div className="side-section" style={{ padding: "0 0 12px" }}>
          {t("wiki.section_artifacts")} · {list.length}
        </div>
        {(selectedArea || selectedType || badgeFilters.length > 0) && (
          <AppliedFilterBar
            selectedArea={selectedArea}
            selectedAreaLabel={selectedArea ? areaNameBySlug.get(selectedArea) ?? selectedArea : null}
            selectedType={selectedType}
            badgeFilters={badgeFilters}
            onClearAreaFilter={onClearAreaFilter}
            onClearTypeFilter={onClearTypeFilter}
            onClearBadgeFilter={onClearBadgeFilter}
          />
        )}
        {list.length === 0 ? (
          <div style={{ color: "var(--fg-3)", fontSize: 13 }}>{empty}</div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {list.map((a) => {
              const linkBase = `/p/${projectSlug}/${projectLocale}/wiki`;
              const isActive = currentSlug === a.slug;
              return (
                <Link
                  key={a.id}
                  to={filteredReaderHref(linkBase, a.slug, selectedArea, selectedType, badgeFilters)}
                  className={`backlink${isActive ? " is-active" : ""}`}
                  style={isActive ? {
                    borderColor: "var(--accent)",
                    background: "color-mix(in oklch, var(--accent) 10%, transparent)",
                  } : undefined}
                >
                  <div className="backlink__head">
                    <span className={typeChipClass(a.type)}>{a.type}</span>
                    <span>{a.title}</span>
                  </div>
                  <div className="backlink__excerpt">
                    {(areaNameBySlug.get(a.area_slug) ?? a.area_slug)} · {a.author_id} · {new Date(a.updated_at).toLocaleDateString()}
                  </div>
                </Link>
              );
            })}
          </div>
        )}
      </div>
    </main>
  );
}

// Kanban-lite grouping: columns by task_meta.status. Artifacts without
// task_meta fall into a "no status" column so they're still visible and
// promote-able. Keep vertical list inside each column — full kanban
// drag-drop is out of scope; Pindoc's write model is agent-only, and
// users should request status transitions via agent, not mouse.
//
// Migration 0013 replaced the Jira-style todo/in_progress/done/blocked/
// cancelled enum with a two-phase completion lifecycle: open → claimed_done
// (implementer self-attest) → verified (different agent files a
// VerificationReport). Cancelled and blocked remain as lateral states.
// Each column header carries a status-pill variant — the CSS sits in
// reader.css and the visual language follows the design system kit
// `ui_kits/reader/tasks.html`. `pill` is the status-pill modifier class
// (todo/in_progress/done/blocked/archived) — *not* the enum id, because
// the design kit was authored before the Jira→two-phase rename.
type TaskColumnSpec = {
  id: string;
  labelKey: string;
  pill: "todo" | "in_progress" | "done" | "blocked" | "archived";
};
const TASK_COLUMNS: TaskColumnSpec[] = [
  { id: "open", labelKey: "tasks.col_open", pill: "todo" },
  { id: "claimed_done", labelKey: "tasks.col_claimed_done", pill: "in_progress" },
  { id: "verified", labelKey: "tasks.col_verified", pill: "done" },
  { id: "blocked", labelKey: "tasks.col_blocked", pill: "blocked" },
];

const PRIORITY_CLASS: Record<string, string> = {
  p0: "prio prio--p0",
  p1: "prio prio--p1",
  p2: "prio prio--p2",
  p3: "prio prio--p3",
};

function TasksKanban({
  projectSlug,
  projectLocale,
  list,
  allList,
  currentSlug,
  empty,
  selectedArea,
  badgeFilters,
  areaNameBySlug,
  selectedTaskSlug,
  onSelectTask,
  onClearAreaFilter,
  onClearBadgeFilter,
  onClearFilters,
}: {
  projectSlug: string;
  projectLocale: string;
  list: ArtifactRef[];
  allList: ArtifactRef[];
  currentSlug: string | undefined;
  empty: string;
  selectedArea: string | null;
  badgeFilters: BadgeFilter[];
  areaNameBySlug: ReadonlyMap<string, string>;
  selectedTaskSlug: string | null;
  onSelectTask: (slug: string) => void;
  onClearAreaFilter: () => void;
  onClearBadgeFilter: (key: BadgeFilterKey) => void;
  onClearFilters: () => void;
}) {
  const { t } = useI18n();

  const groups = groupTasksByStatus(list);
  const allGroups = groupTasksByStatus(allList);
  const orderedTasks = orderedTaskList(groups);
  const hasActiveFilters = Boolean(selectedArea || badgeFilters.length > 0);
  const scopeLabel = selectedArea
    ? areaNameBySlug.get(selectedArea) ?? selectedArea
    : t("wiki.area_all");
  const filteredPendingCount = countPendingTasks(list);
  const allPendingCount = countPendingTasks(allList);

  useEffect(() => {
    if (orderedTasks.length === 0) return;
    function onKey(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
      const target = e.target as HTMLElement | null;
      if (target?.closest("input, textarea, select, button, a, [contenteditable='true']")) return;
      e.preventDefault();
      const current = selectedTaskSlug
        ? orderedTasks.findIndex((a) => a.slug === selectedTaskSlug)
        : -1;
      const delta = e.key === "ArrowDown" ? 1 : -1;
      const nextIndex =
        current < 0
          ? (delta > 0 ? 0 : orderedTasks.length - 1)
          : Math.max(0, Math.min(orderedTasks.length - 1, current + delta));
      const next = orderedTasks[nextIndex];
      if (next) onSelectTask(next.slug);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [orderedTasks, selectedTaskSlug, onSelectTask]);

  if (allList.length === 0) {
    return (
      <main className="content">
        <div className="reader-article">
          <TaskBoardHeader
            scopeLabel={scopeLabel}
            totalCount={0}
            filteredPendingCount={0}
            selectedArea={selectedArea}
            badgeFilters={badgeFilters}
            onClearAreaFilter={onClearAreaFilter}
            onClearBadgeFilter={onClearBadgeFilter}
          />
          <div className="task-empty-state">
            <div className="task-empty-state__title">{empty}</div>
          </div>
        </div>
      </main>
    );
  }

  return (
    <main className="content">
      <TaskBoardHeader
        scopeLabel={scopeLabel}
        totalCount={allList.length}
        filteredPendingCount={filteredPendingCount}
        selectedArea={selectedArea}
        badgeFilters={badgeFilters}
        onClearAreaFilter={onClearAreaFilter}
        onClearBadgeFilter={onClearBadgeFilter}
      />
      {list.length === 0 && hasActiveFilters && (
        <TaskFilterEmptyState
          allPendingCount={allPendingCount}
          onClearFilters={onClearFilters}
        />
      )}
      <div className="kanban">
        {TASK_COLUMNS.map((col) => (
          <TaskColumn
            key={col.id}
            label={t(col.labelKey)}
            pill={col.pill}
            items={groups.get(col.id) ?? []}
            allCount={allGroups.get(col.id)?.length ?? 0}
            hasActiveFilters={hasActiveFilters}
            onClearFilters={onClearFilters}
            projectSlug={projectSlug}
            projectLocale={projectLocale}
            currentSlug={currentSlug}
            selectedTaskSlug={selectedTaskSlug}
            onSelectTask={onSelectTask}
            areaNameBySlug={areaNameBySlug}
          />
        ))}
      </div>
      {(groups.get("no_status")?.length ?? 0) > 0 && (
        <div className="kanban__extra">
          <TaskColumn
            label={t("tasks.col_no_status")}
            pill="todo"
            items={groups.get("no_status") ?? []}
            allCount={allGroups.get("no_status")?.length ?? 0}
            hasActiveFilters={hasActiveFilters}
            onClearFilters={onClearFilters}
            projectSlug={projectSlug}
            projectLocale={projectLocale}
            currentSlug={currentSlug}
            selectedTaskSlug={selectedTaskSlug}
            onSelectTask={onSelectTask}
            subtle
            areaNameBySlug={areaNameBySlug}
          />
        </div>
      )}
      {(groups.get("cancelled")?.length ?? 0) > 0 && (
        <div className="kanban__extra">
          <TaskColumn
            label={t("tasks.col_cancelled")}
            pill="archived"
            items={groups.get("cancelled") ?? []}
            allCount={allGroups.get("cancelled")?.length ?? 0}
            hasActiveFilters={hasActiveFilters}
            onClearFilters={onClearFilters}
            projectSlug={projectSlug}
            projectLocale={projectLocale}
            currentSlug={currentSlug}
            selectedTaskSlug={selectedTaskSlug}
            onSelectTask={onSelectTask}
            subtle
            areaNameBySlug={areaNameBySlug}
          />
        </div>
      )}
    </main>
  );
}

function groupTasksByStatus(list: ArtifactRef[]): Map<string, ArtifactRef[]> {
  const groups = new Map<string, ArtifactRef[]>();
  for (const col of TASK_COLUMNS) groups.set(col.id, []);
  groups.set("no_status", []);
  groups.set("cancelled", []);
  for (const a of list) {
    const s = a.task_meta?.status;
    if (s === "cancelled") {
      groups.get("cancelled")!.push(a);
    } else if (s && groups.has(s)) {
      groups.get(s)!.push(a);
    } else {
      groups.get("no_status")!.push(a);
    }
  }
  return groups;
}

function countPendingTasks(list: ArtifactRef[]): number {
  return list.filter((a) => {
    const status = a.task_meta?.status;
    return !status || status === "open";
  }).length;
}

function orderedTaskList(groups: Map<string, ArtifactRef[]>): ArtifactRef[] {
  return [
    ...TASK_COLUMNS.flatMap((col) => groups.get(col.id) ?? []),
    ...(groups.get("no_status") ?? []),
    ...(groups.get("cancelled") ?? []),
  ];
}

function TaskBoardHeader({
  scopeLabel,
  totalCount,
  filteredPendingCount,
  selectedArea,
  badgeFilters,
  onClearAreaFilter,
  onClearBadgeFilter,
}: {
  scopeLabel: string;
  totalCount: number;
  filteredPendingCount: number;
  selectedArea: string | null;
  badgeFilters: BadgeFilter[];
  onClearAreaFilter: () => void;
  onClearBadgeFilter: (key: BadgeFilterKey) => void;
}) {
  const { t } = useI18n();
  const title = selectedArea
    ? t("tasks.scope_title_filtered", scopeLabel, filteredPendingCount, totalCount)
    : t("tasks.scope_title_all", totalCount);
  return (
    <div className="task-board-head">
      <div>
        <div className="task-board-head__eyebrow">{t("nav.tasks")}</div>
        <h1 className="task-board-head__title">{title}</h1>
      </div>
      <div className="task-filter-bar" aria-label={t("tasks.filter_bar_label")}>
        <span className="task-filter-chip task-filter-chip--locked">
          <span className="task-filter-chip__key">Type</span>
          <span>Task</span>
        </span>
        {selectedArea && (
          <span className="task-filter-chip">
            <span className="task-filter-chip__key">Area</span>
            <span>{scopeLabel}</span>
            <button
              type="button"
              className="task-filter-chip__remove"
              onClick={onClearAreaFilter}
              aria-label={t("tasks.filter_remove_area", scopeLabel)}
              title={t("tasks.filter_remove_area", scopeLabel)}
            >
              <X className="lucide" />
            </button>
          </span>
        )}
        {badgeFilters.map((filter) => (
          <FilterChip
            key={`${filter.key}:${filter.value}`}
            keyLabel={badgeFilterKeyLabel(filter.key, t)}
            label={filter.label}
            onRemove={() => onClearBadgeFilter(filter.key)}
            removeLabel={t("reader.filter_remove_badge", filter.label)}
          />
        ))}
      </div>
    </div>
  );
}

function AppliedFilterBar({
  selectedArea,
  selectedAreaLabel,
  selectedType,
  badgeFilters,
  onClearAreaFilter,
  onClearTypeFilter,
  onClearBadgeFilter,
}: {
  selectedArea: string | null;
  selectedAreaLabel: string | null;
  selectedType: string | null;
  badgeFilters: BadgeFilter[];
  onClearAreaFilter: () => void;
  onClearTypeFilter: () => void;
  onClearBadgeFilter: (key: BadgeFilterKey) => void;
}) {
  const { t } = useI18n();
  return (
    <div className="task-filter-bar reader-filter-bar" aria-label={t("reader.filter_bar_label")}>
      {selectedType && (
        <FilterChip
          keyLabel="Type"
          label={selectedType}
          onRemove={onClearTypeFilter}
          removeLabel={t("reader.filter_remove_type", selectedType)}
        />
      )}
      {selectedArea && selectedAreaLabel && (
        <FilterChip
          keyLabel="Area"
          label={selectedAreaLabel}
          onRemove={onClearAreaFilter}
          removeLabel={t("tasks.filter_remove_area", selectedAreaLabel)}
        />
      )}
      {badgeFilters.map((filter) => (
        <FilterChip
          key={`${filter.key}:${filter.value}`}
          keyLabel={badgeFilterKeyLabel(filter.key, t)}
          label={filter.label}
          onRemove={() => onClearBadgeFilter(filter.key)}
          removeLabel={t("reader.filter_remove_badge", filter.label)}
        />
      ))}
    </div>
  );
}

function FilterChip({
  keyLabel,
  label,
  onRemove,
  removeLabel,
}: {
  keyLabel: string;
  label: string;
  onRemove: () => void;
  removeLabel: string;
}) {
  return (
    <span className="task-filter-chip">
      <span className="task-filter-chip__key">{keyLabel}</span>
      <span>{label}</span>
      <button
        type="button"
        className="task-filter-chip__remove"
        onClick={onRemove}
        aria-label={removeLabel}
        title={removeLabel}
      >
        <X className="lucide" />
      </button>
    </span>
  );
}

function TaskFilterEmptyState({
  allPendingCount,
  onClearFilters,
}: {
  allPendingCount: number;
  onClearFilters: () => void;
}) {
  const { t } = useI18n();
  return (
    <div className="task-empty-state task-empty-state--filtered">
      <div className="task-empty-state__title">{t("tasks.empty_filtered_head")}</div>
      <div>{t("tasks.empty_filtered_total_pending", allPendingCount)}</div>
      <button type="button" className="task-clear-filter" onClick={onClearFilters}>
        {t("tasks.clear_filters")}
        <span className="kbd">esc</span>
      </button>
    </div>
  );
}

function TaskColumn({
  label,
  pill,
  items,
  allCount,
  hasActiveFilters,
  onClearFilters,
  projectSlug,
  projectLocale,
  currentSlug,
  selectedTaskSlug,
  onSelectTask,
  subtle,
  areaNameBySlug,
}: {
  label: string;
  pill: TaskColumnSpec["pill"];
  items: ArtifactRef[];
  allCount: number;
  hasActiveFilters: boolean;
  onClearFilters: () => void;
  projectSlug: string;
  projectLocale: string;
  currentSlug: string | undefined;
  selectedTaskSlug: string | null;
  onSelectTask: (slug: string) => void;
  subtle?: boolean;
  areaNameBySlug: ReadonlyMap<string, string>;
}) {
  const { t } = useI18n();
  return (
    <div className={`kanban-col${subtle ? " kanban-col--subtle" : ""}`}>
      <div className="kanban-col__head">
        <span className={`status-pill status-pill--${pill}`}>
          <span className="p-dot" />
          {label}
        </span>
        <span className="kanban-col__count">{items.length}</span>
      </div>
      <div className="kanban-col__list">
        {items.map((a) => (
          <TaskCard
            key={a.id}
            artifact={a}
            projectSlug={projectSlug}
            projectLocale={projectLocale}
            isActive={currentSlug === a.slug}
            isSelected={selectedTaskSlug === a.slug}
            onSelect={onSelectTask}
            areaNameBySlug={areaNameBySlug}
          />
        ))}
        {items.length === 0 && (
          hasActiveFilters ? (
            <div className="kanban-col__empty kanban-col__empty--filtered">
              <div>{t("tasks.empty_filtered_column", label)}</div>
              <div>{t("tasks.empty_total_for_column", allCount, label)}</div>
              <button type="button" className="task-clear-filter task-clear-filter--compact" onClick={onClearFilters}>
                {t("tasks.clear_filters")}
              </button>
            </div>
          ) : (
            <div className="kanban-col__empty">—</div>
          )
        )}
      </div>
    </div>
  );
}

// TaskCard renders a single artifact tile in the kanban. Priority chip
// (P0-P3 OKLCH palette) is the primary left-rail marker; area-chip sits
// below the title so the area taxonomy reads as metadata rather than
// competing with priority. task_meta.status=blocked adds a blocked-banner
// across the card top — the detailed blocks-target list lives in the
// Sidecar on the artifact detail page to avoid an N+1 fetch here.
function TaskCard({
  artifact: a,
  projectSlug,
  projectLocale,
  isActive,
  isSelected,
  onSelect,
  areaNameBySlug,
}: {
  artifact: ArtifactRef;
  projectSlug: string;
  projectLocale: string;
  isActive: boolean;
  isSelected: boolean;
  onSelect: (slug: string) => void;
  areaNameBySlug: ReadonlyMap<string, string>;
}) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const priority = a.task_meta?.priority;
  const prioClass = priority ? PRIORITY_CLASS[priority] : undefined;
  const blocked = a.task_meta?.status === "blocked";
  const areaLabel = areaNameBySlug.get(a.area_slug) ?? localizedAreaName(t, a.area_slug, a.area_slug);
  const detailHref = `/p/${projectSlug}/${projectLocale}/wiki/${a.slug}`;
  const selected = isActive || isSelected;
  return (
    <article
      tabIndex={0}
      data-task-card-slug={a.slug}
      aria-selected={selected}
      className={`task-card${selected ? " is-active" : ""}`}
      onClick={() => onSelect(a.slug)}
      onDoubleClick={() => navigate(detailHref)}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          navigate(detailHref);
        }
        if (e.key === " ") {
          e.preventDefault();
          onSelect(a.slug);
        }
      }}
    >
      {blocked && (
        <div className="blocked-banner">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="lucide" aria-hidden="true">
            <path d="M12 9v4" />
            <path d="M12 17h.01" />
            <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
          </svg>
          <div className="blocked-banner__body">
            <div className="blocked-banner__head">{t("tasks.blocked_head")}</div>
            {t("tasks.blocked_hint")}
          </div>
        </div>
      )}
      <div className="task-card__meta">
        {prioClass && (
          <span className={prioClass} title={`priority ${priority}`}>
            <span className="dot" />
            {priority?.toUpperCase()}
          </span>
        )}
        <span className="chip-area">{areaLabel}</span>
      </div>
      <Link
        to={detailHref}
        className="task-card__title task-card__title-link"
        onClick={(e) => e.stopPropagation()}
      >
        {a.title}
      </Link>
      <div className="task-card__foot">
        {a.task_meta?.assignee && <span>{a.task_meta.assignee}</span>}
        {a.task_meta?.due_at && (
          <span>~{new Date(a.task_meta.due_at).toLocaleDateString()}</span>
        )}
        <span>{new Date(a.updated_at).toLocaleDateString()}</span>
      </div>
    </article>
  );
}
