import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import type { Aggregate } from "./useReaderData";
import type { Artifact, ArtifactRef, Area } from "../api/client";
import { useI18n } from "../i18n";
import { CmdK } from "./CmdK";
import { ReaderSurface } from "./ReaderSurface";
import { Sidebar } from "./Sidebar";
import { Sidecar } from "./Sidecar";
import { TopNav } from "./TopNav";
import { initTheme, setTheme, type Theme } from "./theme";
import { useReaderData } from "./useReaderData";
import { typeChipClass } from "./typeChip";
import { initReaderWidth, setReaderWidth as applyReaderWidth, type ReaderWidth } from "./readerWidth";
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

  // When the user filters via the sidebar we drop the currently-selected
  // artifact so the filter effect is visible (otherwise the reader body
  // would paper over the list). Filter-while-detail-shown is a future
  // UX improvement that needs in-place scroll to next matching artifact.
  function handleSelectArea(next: string | null) {
    setSelectedArea(next);
    if (slug) navigate(baseRoute);
  }
  function handleSelectType(next: string | null) {
    setSelectedTypeState((prev) => (prev === next ? null : next));
    if (slug) navigate(baseRoute);
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

  const toggleTheme = () => {
    const next: Theme = theme === "dark" ? "light" : "dark";
    setTheme(next);
    setThemeState(next);
  };

  const changeReaderWidth = (next: ReaderWidth) => {
    applyReaderWidth(next);
    setReaderWidthState(next);
  };

  // Hooks must stay at top level (rules-of-hooks). Filter against a
  // possibly-empty list when we haven't loaded yet so we never call
  // this hook conditionally.
  //
  // Pipeline ordering (Decision `decision-reader-ia-hierarchy`):
  //   baseList → surfaceList (Surface defines the natural set)
  //            → filteredArtifacts (Type + Area layered on top)
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
        return true;
      }),
    [surfaceList, selectedArea, selectedType],
  );
  const areaCountMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const a of surfaceList) {
      if (selectedType && a.type !== selectedType) continue;
      map.set(a.area_slug, (map.get(a.area_slug) ?? 0) + 1);
    }
    return map;
  }, [surfaceList, selectedType]);
  const typeCounts = useMemo<Aggregate[]>(() => {
    const map = new Map<string, number>();
    for (const a of surfaceList) {
      if (selectedArea && a.area_slug !== selectedArea) continue;
      map.set(a.type, (map.get(a.type) ?? 0) + 1);
    }
    return Array.from(map, ([key, count]) => ({ key, count })).sort(
      (a, b) => b.count - a.count,
    );
  }, [surfaceList, selectedArea]);

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
          currentSlug={slug}
          selectedType={selectedType}
        />
        <Sidecar
          projectSlug={project}
          detail={view === "reader" || view === "tasks" ? detail : null}
          authMode={authMode}
          agents={agents}
          users={users}
          onArtifactUpdated={reload}
        />
      </div>
      <CmdK projectSlug={project} open={paletteOpen} onClose={() => setPaletteOpen(false)} />
    </div>
  );
}

function Body({
  view,
  projectSlug,
  projectLocale,
  detail,
  list,
  currentSlug,
  selectedType,
}: {
  view: ReaderView;
  projectSlug: string;
  projectLocale: string;
  detail: Artifact | null;
  list: ArtifactRef[];
  currentSlug: string | undefined;
  selectedType: string | null;
}) {
  const { t } = useI18n();

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
    return <TasksKanban projectSlug={projectSlug} projectLocale={projectLocale} list={list} currentSlug={currentSlug} empty={empty} />;
  }

  return (
    <main className="content">
      <div className="reader-article">
        <div className="side-section" style={{ padding: "0 0 12px" }}>
          {t("wiki.section_artifacts")} · {list.length}
        </div>
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
                  to={`${linkBase}/${a.slug}`}
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
                    {a.area_slug} · {a.author_id} · {new Date(a.updated_at).toLocaleDateString()}
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
  currentSlug,
  empty,
}: {
  projectSlug: string;
  projectLocale: string;
  list: ArtifactRef[];
  currentSlug: string | undefined;
  empty: string;
}) {
  const { t } = useI18n();

  const groups = new Map<string, ArtifactRef[]>();
  for (const col of TASK_COLUMNS) groups.set(col.id, []);
  const noStatus: ArtifactRef[] = [];
  const cancelled: ArtifactRef[] = [];

  for (const a of list) {
    const s = a.task_meta?.status;
    if (s === "cancelled") {
      cancelled.push(a);
    } else if (s && groups.has(s)) {
      groups.get(s)!.push(a);
    } else {
      noStatus.push(a);
    }
  }

  if (list.length === 0) {
    return (
      <main className="content">
        <div className="reader-article">
          <div style={{ color: "var(--fg-3)", fontSize: 13 }}>{empty}</div>
        </div>
      </main>
    );
  }

  return (
    <main className="content">
      <div className="kanban">
        {TASK_COLUMNS.map((col) => (
          <TaskColumn
            key={col.id}
            label={t(col.labelKey)}
            pill={col.pill}
            items={groups.get(col.id) ?? []}
            projectSlug={projectSlug}
            projectLocale={projectLocale}
            currentSlug={currentSlug}
          />
        ))}
      </div>
      {noStatus.length > 0 && (
        <div className="kanban__extra">
          <TaskColumn
            label={t("tasks.col_no_status")}
            pill="todo"
            items={noStatus}
            projectSlug={projectSlug}
            projectLocale={projectLocale}
            currentSlug={currentSlug}
            subtle
          />
        </div>
      )}
      {cancelled.length > 0 && (
        <div className="kanban__extra">
          <TaskColumn
            label={t("tasks.col_cancelled")}
            pill="archived"
            items={cancelled}
            projectSlug={projectSlug}
            projectLocale={projectLocale}
            currentSlug={currentSlug}
            subtle
          />
        </div>
      )}
    </main>
  );
}

function TaskColumn({
  label,
  pill,
  items,
  projectSlug,
  projectLocale,
  currentSlug,
  subtle,
}: {
  label: string;
  pill: TaskColumnSpec["pill"];
  items: ArtifactRef[];
  projectSlug: string;
  projectLocale: string;
  currentSlug: string | undefined;
  subtle?: boolean;
}) {
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
          />
        ))}
        {items.length === 0 && <div className="kanban-col__empty">—</div>}
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
}: {
  artifact: ArtifactRef;
  projectSlug: string;
  projectLocale: string;
  isActive: boolean;
}) {
  const { t } = useI18n();
  const priority = a.task_meta?.priority;
  const prioClass = priority ? PRIORITY_CLASS[priority] : undefined;
  const blocked = a.task_meta?.status === "blocked";
  return (
    <Link
      to={`/p/${projectSlug}/${projectLocale}/tasks/${a.slug}`}
      className={`task-card${isActive ? " is-active" : ""}`}
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
        <span className="chip-area">{a.area_slug}</span>
      </div>
      <div className="task-card__title">{a.title}</div>
      <div className="task-card__foot">
        {a.task_meta?.assignee && <span>{a.task_meta.assignee}</span>}
        {a.task_meta?.due_at && (
          <span>~{new Date(a.task_meta.due_at).toLocaleDateString()}</span>
        )}
        <span>{new Date(a.updated_at).toLocaleDateString()}</span>
      </div>
    </Link>
  );
}
