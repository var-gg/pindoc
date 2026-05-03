import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Link } from "react-router";
import { Bot, CalendarClock, GitBranch, Layers3, ListTree, PanelRightOpen, UserRound, X } from "lucide-react";
import {
  api,
  type ArtifactRef,
  type CurrentUserResp,
  type TaskFlowParams,
  type TaskFlowResp,
  type TaskFlowRow,
} from "../api/client";
import { useI18n, type Lang } from "../i18n";
import { projectSurfacePath } from "../readerRoutes";
import { EmptyState, SurfaceHeader } from "./SurfacePrimitives";
import { Tooltip } from "./Tooltip";
import { localizedAreaName } from "./areaLocale";
import { taskAssigneeLabel } from "./assigneeDisplay";
import { formatDate } from "../utils/formatDateTime";
import {
  TASK_FLOW_STAGES,
  groupTaskFlowByProject,
  taskFlowRowsForCurrentFilter,
  taskFlowSummary,
} from "./taskFlowViewModel";
import type { BadgeFilter, BadgeFilterKey } from "./badgeFilters";

type TaskFlowScope = "current" | "visible";
type TaskFlowActorMode = "all_visible" | "my" | "assignee" | "agent" | "team";

type Props = {
  projectSlug: string;
  orgSlug: string;
  list: ArtifactRef[];
  allList: ArtifactRef[];
  currentSlug: string | undefined;
  selectedArea: string | null;
  badgeFilters: BadgeFilter[];
  areaNameBySlug: ReadonlyMap<string, string>;
  selectedTaskSlug: string | null;
  onSelectTask: (slug: string) => void;
  onClearAreaFilter: () => void;
  onClearBadgeFilter: (key: BadgeFilterKey) => void;
  modeSwitch?: ReactNode;
};

type FlowState =
  | { kind: "loading" }
  | { kind: "ready"; data: TaskFlowResp }
  | { kind: "error"; message: string }
  | { kind: "needs_actor"; message: string };

type ActorConfig = {
  ready: boolean;
  params: Pick<TaskFlowParams, "actor_scope" | "actor_id" | "actor_ids" | "include_unassigned">;
};

type TaskFlowLane = {
  key: string;
  label: string;
  rows: TaskFlowRow[];
  tone: TaskFlowRow["stage"];
};

export function TaskFlowLens({
  projectSlug,
  orgSlug,
  list,
  allList,
  currentSlug,
  selectedArea,
  badgeFilters,
  areaNameBySlug,
  selectedTaskSlug,
  onSelectTask,
  onClearAreaFilter,
  onClearBadgeFilter,
  modeSwitch,
}: Props) {
  const { t } = useI18n();
  const [projectScope, setProjectScope] = useState<TaskFlowScope>("current");
  const [actorMode, setActorMode] = useState<TaskFlowActorMode>("all_visible");
  const [actorValue, setActorValue] = useState("");
  const [teamValue, setTeamValue] = useState("");
  const [showSequence, setShowSequence] = useState(true);
  const [showProjects, setShowProjects] = useState(false);
  const [currentUser, setCurrentUser] = useState<CurrentUserResp | null>(null);
  const [state, setState] = useState<FlowState>({ kind: "loading" });

  const assigneeOptions = useMemo(() => buildAssigneeOptions(allList, currentUser, t), [allList, currentUser, t]);
  const agentOptions = useMemo(() => assigneeOptions.filter((option) => option.value.startsWith("agent:")), [assigneeOptions]);
  const actorConfig = useMemo(
    () => buildActorConfig(actorMode, actorValue, teamValue, currentUser),
    [actorMode, actorValue, teamValue, currentUser],
  );

  useEffect(() => {
    let cancelled = false;
    api.currentUser()
      .then((resp) => {
        if (!cancelled) setCurrentUser(resp);
      })
      .catch(() => {
        if (!cancelled) setCurrentUser(null);
      });
    return () => {
      cancelled = true;
    };
  }, [projectSlug]);

  useEffect(() => {
    if (actorMode === "assignee" && !actorValue && assigneeOptions.length > 0) {
      setActorValue(assigneeOptions[0].value);
    }
    if (actorMode === "agent" && (!actorValue || !actorValue.startsWith("agent:")) && agentOptions.length > 0) {
      setActorValue(agentOptions[0].value);
    }
  }, [actorMode, actorValue, assigneeOptions, agentOptions]);

  useEffect(() => {
    if (!actorConfig.ready) {
      setState({ kind: "needs_actor", message: actorNeedsMessage(actorMode, t) });
      return;
    }
    let cancelled = false;
    setState({ kind: "loading" });
    api.taskFlow(projectSlug, {
      project_scope: projectScope,
      flow_scope: "all",
      area_slug: selectedArea ?? undefined,
      limit: 300,
      ...actorConfig.params,
    })
      .then((data) => {
        if (!cancelled) setState({ kind: "ready", data });
      })
      .catch((err) => {
        if (!cancelled) setState({ kind: "error", message: String(err) });
      });
    return () => {
      cancelled = true;
    };
  }, [actorConfig, actorMode, projectScope, projectSlug, selectedArea, t]);

  const currentVisibleSlugs = useMemo(
    () => new Set(list.map((artifact) => artifact.slug)),
    [list],
  );
  const rows = useMemo(() => {
    if (state.kind !== "ready") return [] as TaskFlowRow[];
    return taskFlowRowsForCurrentFilter(state.data.items, state.data.project_scope, currentVisibleSlugs);
  }, [state, currentVisibleSlugs]);
  const summary = taskFlowSummary(rows);
  const projectGroups = groupTaskFlowByProject(rows);
  const scopeLabel = selectedArea
    ? areaNameBySlug.get(selectedArea) ?? selectedArea
    : t("wiki.area_all");

  return (
    <main className="content">
      {modeSwitch}
      <div className="task-flow-head">
        <SurfaceHeader
          name="task"
          count={summary.total}
          secondary={{ label: t(projectScope === "visible" ? "tasks.flow_scope_visible" : "tasks.flow_scope_project"), count: summary.projects }}
        />
        <div className="task-flow-head__meta">
          <span>{state.kind === "ready" ? state.data.mode : "derived"}</span>
          <span>{scopeLabel}</span>
        </div>
      </div>

      <div className="task-flow-controls" aria-label={t("tasks.flow_controls")}>
        <div className="task-flow-segment" role="group" aria-label={t("tasks.flow_project_scope")}>
          <TaskFlowControlButton
            active={projectScope === "current"}
            label={t("tasks.flow_scope_project")}
            icon={<ListTree className="lucide" aria-hidden="true" />}
            onClick={() => setProjectScope("current")}
          />
          <TaskFlowControlButton
            active={projectScope === "visible"}
            label={t("tasks.flow_scope_visible")}
            icon={<Layers3 className="lucide" aria-hidden="true" />}
            onClick={() => setProjectScope("visible")}
          />
        </div>

        <select
          className="task-flow-select"
          value={actorMode}
          onChange={(e) => {
            setActorMode(e.target.value as TaskFlowActorMode);
            setActorValue("");
          }}
          aria-label={t("tasks.flow_actor_filter")}
        >
          <option value="all_visible">{t("tasks.flow_actor_all")}</option>
          <option value="my">{t("tasks.flow_actor_my")}</option>
          <option value="assignee">{t("tasks.flow_actor_assignee")}</option>
          <option value="agent">{t("tasks.flow_actor_agent")}</option>
          <option value="team">{t("tasks.flow_actor_team")}</option>
        </select>

        {actorMode === "assignee" && (
          <select
            className="task-flow-select task-flow-select--wide"
            value={actorValue}
            onChange={(e) => setActorValue(e.target.value)}
            aria-label={t("tasks.flow_actor_assignee")}
          >
            {assigneeOptions.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
        )}
        {actorMode === "agent" && (
          <select
            className="task-flow-select task-flow-select--wide"
            value={actorValue}
            onChange={(e) => setActorValue(e.target.value)}
            aria-label={t("tasks.flow_actor_agent")}
          >
            {agentOptions.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
        )}
        {actorMode === "team" && (
          <input
            className="task-flow-input"
            value={teamValue}
            onChange={(e) => setTeamValue(e.target.value)}
            placeholder={t("tasks.flow_actor_team_placeholder")}
            aria-label={t("tasks.flow_actor_team")}
          />
        )}

        <div className="task-flow-segment task-flow-segment--right" role="group" aria-label={t("tasks.flow_sections")}>
          <TaskFlowControlButton
            active={showSequence}
            label={t("tasks.flow_total_sequence")}
            icon={<GitBranch className="lucide" aria-hidden="true" />}
            onClick={() => setShowSequence((v) => !v)}
          />
          <TaskFlowControlButton
            active={showProjects}
            label={t("tasks.flow_project_sections")}
            icon={<Layers3 className="lucide" aria-hidden="true" />}
            onClick={() => setShowProjects((v) => !v)}
          />
        </div>
      </div>

      {(selectedArea || badgeFilters.length > 0) && (
        <div className="task-filter-bar task-flow-filter-bar" aria-label={t("tasks.filter_bar_label")}>
          <span className="task-filter-chip task-filter-chip--locked">
            <span className="task-filter-chip__key">Type</span>
            <span>Task</span>
          </span>
          {selectedArea && (
            <TaskFlowFilterChip
              keyLabel="Area"
              label={scopeLabel}
              removeLabel={t("tasks.filter_remove_area", scopeLabel)}
              onRemove={onClearAreaFilter}
            />
          )}
          {badgeFilters.map((filter) => (
            <TaskFlowFilterChip
              key={`${filter.key}:${filter.value}`}
              keyLabel={filter.key}
              label={filter.label}
              removeLabel={t("reader.filter_remove_badge", filter.label)}
              onRemove={() => onClearBadgeFilter(filter.key)}
            />
          ))}
        </div>
      )}

      <TaskFlowSummaryStrip summary={summary} />

      {state.kind === "loading" && <div className="reader-state">{t("wiki.loading")}</div>}
      {state.kind === "error" && <EmptyState message={state.message} />}
      {state.kind === "needs_actor" && <EmptyState message={state.message} />}
      {state.kind === "ready" && rows.length === 0 && (
        <EmptyState message={t("tasks.flow_empty")} />
      )}
      {state.kind === "ready" && rows.length > 0 && (
        <div className="task-flow-layout">
          {showSequence && (
            <TaskFlowRail
              title={t("tasks.flow_total_sequence")}
              count={rows.length}
              lanes={buildStageLanes(rows, t)}
              railKind="stage"
              projectSlug={projectSlug}
              orgSlug={orgSlug}
              currentSlug={currentSlug}
              selectedTaskSlug={selectedTaskSlug}
              areaNameBySlug={areaNameBySlug}
              onSelectTask={onSelectTask}
            />
          )}
          {showProjects && (
            <TaskFlowRail
              title={t("tasks.flow_project_sections")}
              count={rows.length}
              lanes={projectGroups.map((group) => ({
                key: group.projectSlug,
                label: group.projectSlug,
                rows: group.rows,
                tone: dominantStage(group.rows),
              }))}
              railKind="project"
              projectSlug={projectSlug}
              orgSlug={orgSlug}
              currentSlug={currentSlug}
              selectedTaskSlug={selectedTaskSlug}
              areaNameBySlug={areaNameBySlug}
              onSelectTask={onSelectTask}
            />
          )}
          {!showSequence && !showProjects && (
            <TaskFlowRail
              title={t("tasks.flow_total_sequence")}
              count={rows.length}
              lanes={buildStageLanes(rows, t)}
              railKind="stage"
              projectSlug={projectSlug}
              orgSlug={orgSlug}
              currentSlug={currentSlug}
              selectedTaskSlug={selectedTaskSlug}
              areaNameBySlug={areaNameBySlug}
              onSelectTask={onSelectTask}
            />
          )}
        </div>
      )}
    </main>
  );
}

function TaskFlowControlButton({
  active,
  label,
  icon,
  onClick,
}: {
  active: boolean;
  label: string;
  icon: ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={`task-flow-control${active ? " is-active" : ""}`}
      onClick={onClick}
      aria-pressed={active}
    >
      {icon}
      <span>{label}</span>
    </button>
  );
}

function TaskFlowFilterChip({
  keyLabel,
  label,
  removeLabel,
  onRemove,
}: {
  keyLabel: string;
  label: string;
  removeLabel: string;
  onRemove: () => void;
}) {
  return (
    <span className="task-filter-chip">
      <span className="task-filter-chip__key">{keyLabel}</span>
      <span>{label}</span>
      <Tooltip content={removeLabel}>
        <button
          type="button"
          className="task-filter-chip__remove"
          onClick={onRemove}
          aria-label={removeLabel}
        >
          <X className="lucide" />
        </button>
      </Tooltip>
    </span>
  );
}

function TaskFlowSummaryStrip({ summary }: { summary: ReturnType<typeof taskFlowSummary> }) {
  const { t } = useI18n();
  return (
    <section className="task-flow-summary" aria-label={t("tasks.flow_summary")}>
      <TaskFlowSummaryItem label={t("tasks.flow_summary_ready")} value={summary.ready} />
      <TaskFlowSummaryItem label={t("tasks.flow_summary_blocked")} value={summary.blocked} />
      <TaskFlowSummaryItem label={t("tasks.flow_summary_done")} value={summary.done} />
      <TaskFlowSummaryItem label={t("tasks.flow_summary_projects")} value={summary.projects} />
    </section>
  );
}

function TaskFlowSummaryItem({ label, value }: { label: string; value: number }) {
  return (
    <div className="task-flow-summary__item">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function TaskFlowRail({
  title,
  count,
  lanes,
  railKind,
  projectSlug,
  orgSlug,
  currentSlug,
  selectedTaskSlug,
  areaNameBySlug,
  onSelectTask,
}: {
  title: string;
  count: number;
  lanes: TaskFlowLane[];
  railKind: "stage" | "project";
  projectSlug: string;
  orgSlug: string;
  currentSlug: string | undefined;
  selectedTaskSlug: string | null;
  areaNameBySlug: ReadonlyMap<string, string>;
  onSelectTask: (slug: string) => void;
}) {
  const { t } = useI18n();
  return (
    <section className={`task-flow-section task-flow-rail task-flow-rail--${railKind}`}>
      <div className="task-flow-section__head">
        <h2>{title}</h2>
        <span>{count}</span>
      </div>
      <div className="task-flow-lanes">
        {lanes.map((lane) => (
          <div className={`task-flow-lane task-flow-lane--${lane.tone}`} key={lane.key}>
            <div className="task-flow-lane__label">
              <span>{lane.label}</span>
              <strong>{lane.rows.length}</strong>
            </div>
            <div className="task-flow-lane__track" aria-label={`${lane.label} ${lane.rows.length}`}>
              {lane.rows.length === 0 && (
                <div className="task-flow-lane__empty">{t("tasks.flow_lane_empty")}</div>
              )}
              {lane.rows.map((row) => (
                <TaskFlowRowCard
                  key={`${lane.key}:${row.project_slug}:${row.slug}`}
                  row={row}
                  projectSlug={projectSlug}
                  orgSlug={orgSlug}
                  currentSlug={currentSlug}
                  selectedTaskSlug={selectedTaskSlug}
                  areaNameBySlug={areaNameBySlug}
                  onSelectTask={onSelectTask}
                />
              ))}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function TaskFlowRowCard({
  row,
  projectSlug,
  orgSlug,
  currentSlug,
  selectedTaskSlug,
  areaNameBySlug,
  onSelectTask,
}: {
  row: TaskFlowRow;
  projectSlug: string;
  orgSlug: string;
  currentSlug: string | undefined;
  selectedTaskSlug: string | null;
  areaNameBySlug: ReadonlyMap<string, string>;
  onSelectTask: (slug: string) => void;
}) {
  const { t, lang } = useI18n();
  const isLocal = row.project_slug === projectSlug;
  const selected = isLocal && (currentSlug === row.slug || selectedTaskSlug === row.slug);
  const priorityClass = row.priority ? `prio prio--${row.priority}` : "";
  const statusPill = statusPillClass(row.status);
  const areaLabel = areaNameBySlug.get(row.area_slug) ?? localizedAreaName(t, row.area_slug, row.area_slug);
  const href = isLocal
    ? projectSurfacePath(row.project_slug, "wiki", row.slug, orgSlug)
    : row.human_url || `/p/${row.project_slug}/wiki/${row.slug}`;
  const blockers = row.blockers ?? [];
  const blocked = row.stage === "blocked" || blockers.length > 0;
  return (
    <article
      className={`task-flow-card task-flow-card--${row.stage}${blocked ? " task-flow-card--has-blockers" : ""}${selected ? " is-active" : ""}`}
      data-task-flow-row={row.slug}
      data-readiness={row.readiness}
      tabIndex={0}
      aria-selected={selected}
      onClick={(e) => {
        if (!isLocal) return;
        const target = e.target as HTMLElement | null;
        if (target?.closest("a, button, input, select")) return;
        onSelectTask(row.slug);
      }}
      onKeyDown={(e) => {
        if (!isLocal || (e.key !== "Enter" && e.key !== " ")) return;
        const target = e.target as HTMLElement | null;
        if (target?.closest("a, button, input, select")) return;
        e.preventDefault();
        onSelectTask(row.slug);
      }}
    >
      <div className="task-flow-card__body">
        <div className="task-flow-card__topline">
          <span className="task-flow-card__ordinal">{row.ordinal}</span>
          {row.due_at && (
            <span className="task-flow-card__deadline">
              <CalendarClock className="lucide" aria-hidden="true" />
              <span>{t("tasks.flow_deadline_marker")}</span>
              <time dateTime={row.due_at}>{formatTaskFlowDate(row.due_at, lang)}</time>
            </span>
          )}
        </div>
        <div className="task-flow-card__meta">
          <span className={`status-pill ${statusPill}`}>
            <span className="p-dot" />
            {taskStatusLabel(row.status, t)}
          </span>
          {priorityClass && (
            <span className={priorityClass}>
              <span className="dot" />
              {row.priority?.toUpperCase()}
            </span>
          )}
          <span className={`task-flow-readiness task-flow-readiness--${row.readiness}`}>
            {taskReadinessLabel(row.readiness, t)}
          </span>
        </div>
        <Tooltip content={t("tasks.card_open_detail_hint")}>
          <Link to={href} className="task-flow-card__title">
            {row.title}
          </Link>
        </Tooltip>
        {isLocal && (
          <span className="task-flow-card__preview-hint" aria-label={t("tasks.card_select_hint")}>
            <PanelRightOpen className="lucide" aria-hidden="true" />
            <span>{t("reader.card_preview_label")}</span>
          </span>
        )}
        <div className="task-flow-card__chips">
          <span className="task-flow-card__actor">
            {row.assignee?.startsWith("user:") || row.assignee?.startsWith("@")
              ? <UserRound className="lucide" aria-hidden="true" />
              : <Bot className="lucide" aria-hidden="true" />}
            <span>{taskAssigneeLabel(row.assignee, t)}</span>
          </span>
          <span className="chip-area">{areaLabel}</span>
          <span className="task-flow-project-chip">{row.project_slug}</span>
        </div>
        <div className="task-flow-card__foot">
          <span>{t("tasks.flow_updated_marker")}</span>
          <time dateTime={row.updated_at}>{formatTaskFlowDate(row.updated_at, lang)}</time>
        </div>
        {blocked && (
          <div className="task-flow-card__blockers">
            <GitBranch className="lucide" aria-hidden="true" />
            <span>{t("tasks.flow_blocked_by")}</span>
            {blockers.length === 0 && (
              <span className="task-flow-card__blocker-note">{taskReadinessLabel(row.readiness, t)}</span>
            )}
            {blockers.map((blocker) => (
              <Link
                key={`${blocker.project_slug}:${blocker.slug}`}
                to={blocker.project_slug === projectSlug
                  ? projectSurfacePath(blocker.project_slug, "wiki", blocker.slug, orgSlug)
                  : `/p/${blocker.project_slug}/wiki/${blocker.slug}`}
              >
                {blocker.title}
              </Link>
            ))}
          </div>
        )}
      </div>
    </article>
  );
}

function buildStageLanes(
  rows: TaskFlowRow[],
  t: (key: string, ...args: Array<string | number>) => string,
): TaskFlowLane[] {
  return TASK_FLOW_STAGES.map((stage) => ({
    key: stage,
    label: taskFlowStageLabel(stage, t),
    rows: rows.filter((row) => row.stage === stage),
    tone: stage,
  }));
}

function dominantStage(rows: TaskFlowRow[]): TaskFlowRow["stage"] {
  if (rows.some((row) => row.stage === "blocked")) return "blocked";
  if (rows.some((row) => row.stage === "ready")) return "ready";
  if (rows.some((row) => row.stage === "done")) return "done";
  return "other";
}

function formatTaskFlowDate(value: string, lang: Lang): string {
  return formatDate(value, lang);
}

function buildAssigneeOptions(
  list: ArtifactRef[],
  currentUser: CurrentUserResp | null,
  t: (key: string, ...args: Array<string | number>) => string,
): Array<{ value: string; label: string }> {
  const seen = new Set<string>();
  const out: Array<{ value: string; label: string }> = [];
  for (const id of currentUserActorIDs(currentUser)) {
    if (!seen.has(id)) {
      seen.add(id);
      out.push({ value: id, label: taskAssigneeLabel(id, t) });
    }
  }
  for (const artifact of list) {
    const assignee = artifact.task_meta?.assignee?.trim();
    if (!assignee || seen.has(assignee)) continue;
    seen.add(assignee);
    out.push({ value: assignee, label: taskAssigneeLabel(assignee, t) });
  }
  return out.sort((a, b) => a.value.localeCompare(b.value));
}

function buildActorConfig(
  mode: TaskFlowActorMode,
  actorValue: string,
  teamValue: string,
  currentUser: CurrentUserResp | null,
): ActorConfig {
  if (mode === "all_visible") {
    return { ready: true, params: { actor_scope: "all_visible", include_unassigned: true } };
  }
  if (mode === "my") {
    const ids = currentUserActorIDs(currentUser);
    return {
      ready: ids.length > 0,
      params: { actor_scope: "team", actor_ids: ids, include_unassigned: false },
    };
  }
  if (mode === "assignee") {
    const id = actorValue.trim();
    return {
      ready: id.length > 0,
      params: { actor_scope: "team", actor_ids: id ? [id] : [], include_unassigned: false },
    };
  }
  if (mode === "agent") {
    const id = actorValue.trim();
    return {
      ready: id.length > 0,
      params: { actor_scope: "agent", actor_id: id, include_unassigned: false },
    };
  }
  const ids = teamValue.split(",").map((v) => v.trim()).filter(Boolean);
  return {
    ready: ids.length > 0,
    params: { actor_scope: "team", actor_ids: ids, include_unassigned: false },
  };
}

function currentUserActorIDs(currentUser: CurrentUserResp | null): string[] {
  const user = currentUser?.user;
  if (!user?.id) return [];
  const ids = [`user:${user.id}`];
  if (user.display_name) {
    ids.push(`user:${user.display_name}`);
  }
  if (user.github_handle) {
    const handle = user.github_handle.startsWith("@") ? user.github_handle : `@${user.github_handle}`;
    ids.push(handle);
  }
  return ids;
}

function actorNeedsMessage(mode: TaskFlowActorMode, t: (key: string, ...args: Array<string | number>) => string): string {
  if (mode === "my") return t("tasks.flow_no_current_user");
  return t("tasks.flow_actor_required");
}

function taskFlowStageLabel(stage: TaskFlowRow["stage"], t: (key: string, ...args: Array<string | number>) => string): string {
  switch (stage) {
    case "ready":
      return t("tasks.flow_stage_ready");
    case "blocked":
      return t("tasks.flow_stage_blocked");
    case "done":
      return t("tasks.flow_stage_done");
    default:
      return t("tasks.flow_stage_other");
  }
}

function taskReadinessLabel(readiness: TaskFlowRow["readiness"], t: (key: string, ...args: Array<string | number>) => string): string {
  switch (readiness) {
    case "ready":
      return t("tasks.flow_ready");
    case "blocked":
      return t("tasks.flow_blocked_edge");
    case "blocked_status":
      return t("tasks.flow_blocked_status");
    case "done":
      return t("tasks.flow_done");
    default:
      return t("tasks.flow_other");
  }
}

function taskStatusLabel(status: TaskFlowRow["status"], t: (key: string, ...args: Array<string | number>) => string): string {
  switch (status) {
    case "open":
      return t("tasks.col_open");
    case "claimed_done":
      return t("tasks.col_claimed_done");
    case "blocked":
      return t("tasks.col_blocked");
    case "cancelled":
      return t("tasks.col_cancelled");
    case "missing_status":
      return t("tasks.col_no_status");
    default:
      return status;
  }
}

function statusPillClass(status: TaskFlowRow["status"]): string {
  switch (status) {
    case "claimed_done":
      return "status-pill--done";
    case "blocked":
      return "status-pill--blocked";
    case "cancelled":
      return "status-pill--archived";
    default:
      return "status-pill--todo";
  }
}
