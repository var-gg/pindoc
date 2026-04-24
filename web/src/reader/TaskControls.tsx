// TaskControls renders the operational-metadata edit lane for Task
// artifacts — the visible half of Decision agent-only-write-분할. Status
// is intentionally absent; it stays on pindoc.task.transition /
// pindoc.artifact.verify so the acceptance-checklist and
// Implementer ≠ Verifier gates keep working.
//
// Edit contract:
//   - only renders when detail.type === "Task"
//   - auth_mode="trusted_local" → inline-editable; anything else → read-only
//   - each change does POST /api/p/{project}/artifacts/{slug}/task-meta
//     which ships a revision_shape=meta_patch revision under the hood
//   - expected_version comes from detail.revision_number (added Phase M1.x)
//
// The UI is intentionally minimal: priority as four pills, assignee as a
// short-list of canonical agent handles + a free-text input, due_at as a
// native <input type="datetime-local">. Users dropdown / keyboard
// shortcuts / bulk-assign live on follow-up tasks (Decision Open
// questions).

import { useEffect, useMemo, useState } from "react";
import { CalendarClock, CircleUserRound, Flag, Loader2 } from "lucide-react";
import {
  api,
  type Artifact,
  type TaskMetaPatchInput,
  type UserRef,
} from "../api/client";
import type { Aggregate } from "./useReaderData";
import { useI18n } from "../i18n";

type Props = {
  projectSlug: string;
  detail: Artifact;
  authMode?: string;
  // agents is the project's author_id aggregate (see ReaderData.agents).
  // Rendered as one optgroup in the assignee dropdown so the user can
  // hand off to any agent that's previously written in this project.
  agents: Aggregate[];
  // users is the instance users table projection. Second optgroup in
  // the dropdown — empty array renders as a hidden group.
  users: UserRef[];
  onUpdated: () => void;
};

const PRIORITIES = ["p0", "p1", "p2", "p3"] as const;
type Priority = (typeof PRIORITIES)[number];

// M1 has no /api/user/current endpoint, so edits are attributed to a
// stable web-originated identity. When V1.5 ships the user-identity
// endpoint this becomes dynamic.
const WEB_AUTHOR_ID = "user:web-reader";

// Assignee value conventions (migration 0010 task_meta shape):
//   agent:<author_id>   — e.g. agent:claude-code
//   user:<display_name> — local display name, V1 single-user default
//   @<github_handle>    — once V1.5 OAuth fills github_handle
//   ""                  — unassigned
// We emit `agent:` or `user:` prefixed values by default so the kanban
// card can style them differently without parsing heuristics.
type AssigneeOption = { value: string; label: string; group: "agents" | "users" };

// Agents we always keep visible even if they haven't written in the
// current project yet — typical hand-off targets for a fresh project.
const BOOTSTRAP_AGENTS = ["agent:claude-code", "agent:codex"] as const;

function buildAssigneeOptions(agents: Aggregate[], users: UserRef[]): AssigneeOption[] {
  const agentValues = new Set<string>();
  // existing agents from the aggregate — prefix author_id with "agent:"
  // unless the author_id is already a full "user:..." form (identity-
  // dual artifacts). Authored-by-user rows end up in the users group.
  for (const a of agents) {
    if (a.key.startsWith("user:")) continue;
    agentValues.add(a.key.startsWith("agent:") ? a.key : `agent:${a.key}`);
  }
  for (const name of BOOTSTRAP_AGENTS) agentValues.add(name);

  const agentOptions: AssigneeOption[] = Array.from(agentValues)
    .sort((a, b) => a.localeCompare(b))
    .map((v) => ({ value: v, label: v, group: "agents" }));

  const userOptions: AssigneeOption[] = users
    .slice()
    .sort((a, b) => a.display_name.localeCompare(b.display_name))
    .map((u) => ({
      value: u.github_handle ? `@${u.github_handle}` : `user:${u.display_name}`,
      label: u.github_handle
        ? `${u.display_name} (@${u.github_handle})`
        : u.display_name,
      group: "users",
    }));

  return [...agentOptions, ...userOptions];
}

export function TaskControls({ projectSlug, detail, authMode, agents, users, onUpdated }: Props) {
  const { t } = useI18n();
  const taskMeta = detail.task_meta ?? {};
  const readOnly = authMode !== "trusted_local";

  const [assignee, setAssignee] = useState<string>(taskMeta.assignee ?? "");
  const [priority, setPriority] = useState<Priority | "">((taskMeta.priority as Priority | undefined) ?? "");
  const [dueAt, setDueAt] = useState<string>(() => toDatetimeLocal(taskMeta.due_at));
  const [saving, setSaving] = useState(false);
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const options = useMemo(() => buildAssigneeOptions(agents, users), [agents, users]);
  const agentOptions = options.filter((o) => o.group === "agents");
  const userOptions = options.filter((o) => o.group === "users");

  // If the current assignee isn't in the options list (legacy free-text
  // or hand-set via MCP), we still want the select to display it. Inject
  // it as a transient extra option so round-tripping preserves the value
  // instead of silently resetting to unassigned.
  const extraOption =
    assignee && !options.some((o) => o.value === assignee)
      ? { value: assignee, label: `${assignee} (current)`, group: "agents" as const }
      : null;

  // Re-sync local state when the artifact refetches (e.g. after a save
  // triggers onUpdated). Without this, the inputs would stay anchored
  // to the pre-save values until the user clicked the field.
  useEffect(() => {
    setAssignee(taskMeta.assignee ?? "");
    setPriority((taskMeta.priority as Priority | undefined) ?? "");
    setDueAt(toDatetimeLocal(taskMeta.due_at));
    setErrorCode(null);
    setErrorMsg(null);
  }, [detail.revision_number, taskMeta.assignee, taskMeta.priority, taskMeta.due_at]);

  const dirty = useMemo(() => {
    const currentAssignee = taskMeta.assignee ?? "";
    const currentPriority = taskMeta.priority ?? "";
    const currentDue = toDatetimeLocal(taskMeta.due_at);
    return (
      assignee.trim() !== currentAssignee.trim() ||
      priority !== currentPriority ||
      dueAt !== currentDue
    );
  }, [assignee, priority, dueAt, taskMeta]);

  async function saveOne(field: "assignee" | "priority" | "due_at", value: string) {
    if (detail.revision_number == null) {
      setErrorCode("NO_REVISION");
      setErrorMsg(t("task_controls.err_no_revision"));
      return;
    }
    setSaving(true);
    setErrorCode(null);
    setErrorMsg(null);
    const input: TaskMetaPatchInput = {
      expected_version: detail.revision_number,
      commit_msg: commitMsgFor(field, value),
      author_id: WEB_AUTHOR_ID,
    };
    if (field === "assignee") input.assignee = value;
    if (field === "priority") input.priority = value as Priority;
    if (field === "due_at") input.due_at = value;
    try {
      await api.taskMetaPatch(projectSlug, detail.slug, input);
      onUpdated();
    } catch (e) {
      const err = e as Error & { error_code?: string };
      setErrorCode(err.error_code ?? "ERROR");
      setErrorMsg(err.message);
    } finally {
      setSaving(false);
    }
  }

  async function saveAll() {
    if (detail.revision_number == null) {
      setErrorCode("NO_REVISION");
      setErrorMsg(t("task_controls.err_no_revision"));
      return;
    }
    setSaving(true);
    setErrorCode(null);
    setErrorMsg(null);
    const input: TaskMetaPatchInput = {
      expected_version: detail.revision_number,
      commit_msg: t("task_controls.commit_bulk"),
      author_id: WEB_AUTHOR_ID,
    };
    const trimmedAssignee = assignee.trim();
    if (trimmedAssignee !== (taskMeta.assignee ?? "").trim()) {
      input.assignee = trimmedAssignee;
    }
    if (priority && priority !== taskMeta.priority) {
      input.priority = priority;
    }
    const currentDue = toDatetimeLocal(taskMeta.due_at);
    if (dueAt && dueAt !== currentDue) {
      input.due_at = fromDatetimeLocal(dueAt);
    }
    try {
      await api.taskMetaPatch(projectSlug, detail.slug, input);
      onUpdated();
    } catch (e) {
      const err = e as Error & { error_code?: string };
      setErrorCode(err.error_code ?? "ERROR");
      setErrorMsg(err.message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="task-controls" style={{
      display: "flex", flexDirection: "column", gap: 10,
      paddingBottom: 10, borderBottom: "1px solid var(--border-1)",
    }}>
      <div
        style={{
          fontFamily: "var(--font-mono)",
          fontSize: 10,
          color: "var(--fg-3)",
          textTransform: "uppercase",
          letterSpacing: "0.04em",
        }}
      >
        {t("task_controls.header")}
        {readOnly && (
          <span style={{ marginLeft: 6, color: "var(--fg-4)", textTransform: "none" }}>
            · {t("task_controls.read_only")}
          </span>
        )}
      </div>

      {/* Priority — inline pill group, commits on click */}
      <div style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
        <Flag className="lucide" style={{ width: 13, height: 13, color: "var(--fg-3)" }} />
        <span style={{ fontSize: 11, color: "var(--fg-3)", minWidth: 58 }}>
          {t("task_controls.priority")}
        </span>
        {PRIORITIES.map((p) => {
          const active = priority === p;
          return (
            <button
              key={p}
              type="button"
              disabled={readOnly || saving}
              onClick={() => {
                setPriority(p);
                if (!readOnly) void saveOne("priority", p);
              }}
              style={{
                fontFamily: "var(--font-mono)",
                fontSize: 10,
                padding: "2px 7px",
                borderRadius: "var(--r-1)",
                border: "1px solid var(--border-1)",
                background: active ? "var(--live-bg)" : "transparent",
                color: active ? "var(--live)" : "var(--fg-2)",
                cursor: readOnly || saving ? "default" : "pointer",
                textTransform: "uppercase",
                letterSpacing: "0.03em",
              }}
            >
              {p}
            </button>
          );
        })}
      </div>

      {/* Assignee — real dropdown grouped by agents / users (Decision
          agent-only-write-분할 AC). Free-text entry is deliberately
          dropped; an empty options list is a sign of a setup problem,
          not a cue to fall through to untyped strings. */}
      <div style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
        <CircleUserRound className="lucide" style={{ width: 13, height: 13, color: "var(--fg-3)" }} />
        <span style={{ fontSize: 11, color: "var(--fg-3)", minWidth: 58 }}>
          {t("task_controls.assignee")}
        </span>
        <select
          value={assignee}
          disabled={readOnly || saving}
          onChange={(e) => {
            const v = e.target.value;
            setAssignee(v);
            if (!readOnly) void saveOne("assignee", v);
          }}
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: 11,
            padding: "3px 6px",
            borderRadius: "var(--r-1)",
            border: "1px solid var(--border-1)",
            background: "var(--bg-2)",
            color: "var(--fg-1)",
            minWidth: 200,
          }}
        >
          <option value="">{t("task_controls.assignee_unassigned")}</option>
          {extraOption && (
            <option value={extraOption.value}>{extraOption.label}</option>
          )}
          {agentOptions.length > 0 && (
            <optgroup label={t("task_controls.assignee_group_agents")}>
              {agentOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </optgroup>
          )}
          {userOptions.length > 0 && (
            <optgroup label={t("task_controls.assignee_group_users")}>
              {userOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </optgroup>
          )}
        </select>
      </div>

      {/* due_at — native datetime-local for zero-dep pickers */}
      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
        <CalendarClock className="lucide" style={{ width: 13, height: 13, color: "var(--fg-3)" }} />
        <span style={{ fontSize: 11, color: "var(--fg-3)", minWidth: 58 }}>
          {t("task_controls.due_at")}
        </span>
        <input
          type="datetime-local"
          value={dueAt}
          disabled={readOnly || saving}
          onChange={(e) => setDueAt(e.target.value)}
          onBlur={() => {
            if (readOnly) return;
            const currentDue = toDatetimeLocal(taskMeta.due_at);
            if (dueAt && dueAt !== currentDue) {
              void saveOne("due_at", fromDatetimeLocal(dueAt));
            }
          }}
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: 11,
            padding: "3px 6px",
            borderRadius: "var(--r-1)",
            border: "1px solid var(--border-1)",
            background: "var(--bg-2)",
            color: "var(--fg-1)",
          }}
        />
      </div>

      {/* Save-all button for free-text assignee + dirty datetime. Priority
          pills commit on click so this is mostly for the text inputs. */}
      {!readOnly && dirty && (
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <button
            type="button"
            disabled={saving}
            onClick={() => void saveAll()}
            style={{
              fontSize: 11,
              padding: "4px 10px",
              borderRadius: "var(--r-1)",
              border: "1px solid var(--live)",
              background: "var(--live-bg)",
              color: "var(--live)",
              cursor: saving ? "default" : "pointer",
              display: "inline-flex",
              alignItems: "center",
              gap: 4,
            }}
          >
            {saving && <Loader2 className="lucide" style={{ width: 11, height: 11 }} />}
            {t("task_controls.save")}
          </button>
        </div>
      )}

      {errorMsg && (
        <div
          role="alert"
          style={{
            fontSize: 11,
            fontFamily: "var(--font-mono)",
            color: "var(--danger, #d33)",
            background: "var(--danger-bg, rgba(211,51,51,0.08))",
            padding: "4px 6px",
            borderRadius: "var(--r-1)",
          }}
        >
          {errorCode ? `[${errorCode}] ` : ""}{errorMsg}
        </div>
      )}
    </div>
  );
}

// RFC3339 ↔ datetime-local (YYYY-MM-DDTHH:mm) converters. The native
// input doesn't accept the seconds / timezone suffix, so we trim for
// display and re-attach a UTC suffix on write. Good enough for M1;
// timezone-aware pickers arrive with the proper schedule UI.
function toDatetimeLocal(rfc?: string): string {
  if (!rfc) return "";
  const d = new Date(rfc);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T` +
    `${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}

function fromDatetimeLocal(local: string): string {
  if (!local) return "";
  const d = new Date(local);
  if (Number.isNaN(d.getTime())) return local;
  return d.toISOString();
}

function commitMsgFor(field: "assignee" | "priority" | "due_at", value: string): string {
  const display = value || "(unset)";
  return `UI TaskControls: set ${field}=${display}`;
}
