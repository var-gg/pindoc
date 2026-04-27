import { useCallback, useEffect, useMemo, useState } from "react";
import { Loader2, ShieldCheck, Trash2, X } from "lucide-react";
import {
  api,
  type ActiveInviteRow,
  type MemberRow,
  type Project,
  type UserRef,
} from "../api/client";
import { useI18n } from "../i18n";

// MembersPanel — Phase D permission management plane.
//
// Always renders. Decision `decision-auth-model-loopback-and-
// providers` retired the auth_mode framing the previous conditional
// gate ("show only when auth_mode=oauth_github") leaned on; a 1-
// person self-host operator should still see "you are the only
// member" + "issue invite to add someone" without flipping any env
// first. The panel groups two concerns the owner cares about in the
// same context as "issue an invite": who is already in the project
// (members) and which outstanding invites can still be claimed
// (active invites). Both sections expose a destructive action —
// remove a member, revoke a token — with confirmation inline rather
// than a second modal.
//
// All fetches are owner-scoped at the server; the FE permission gate
// is purely cosmetic. We still hide actions the viewer cannot perform
// so they don't see a button that always 403s.

type Props = {
  project: Project;
  // refreshNonce is incremented by the parent (InviteModal) right after
  // a successful invite issue so we re-fetch the active-invites list
  // without forcing the user to close + reopen the modal.
  refreshNonce?: number;
  users?: UserRef[] | null;
};

type Status =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "ready"; members: MemberRow[]; invites: ActiveInviteRow[]; viewerRole: string; viewerId?: string }
  | { kind: "error"; message: string };

export function MembersPanel({ project, refreshNonce = 0, users }: Props) {
  const { t, lang } = useI18n();
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [actionError, setActionError] = useState<string | null>(null);
  const [pendingAction, setPendingAction] = useState<string | null>(null);
  const [reloadCounter, setReloadCounter] = useState(0);

  const usersByID = useMemo(() => {
    const map = new Map<string, UserRef>();
    if (!users) return map;
    for (const u of users) map.set(u.id, u);
    return map;
  }, [users]);

  const reload = useCallback(() => {
    setReloadCounter((n) => n + 1);
  }, []);

  useEffect(() => {
    let cancelled = false;
    setStatus({ kind: "loading" });
    Promise.all([
      api.members(project.slug),
      api.activeInvites(project.slug).catch((err: Error & { error_code?: string }) => {
        // viewer/editor role hits PROJECT_OWNER_REQUIRED on /invites
        // — that's expected, not an error to surface.
        if (err.error_code === "PROJECT_OWNER_REQUIRED") {
          return { project_slug: project.slug, invites: [] as ActiveInviteRow[] };
        }
        throw err;
      }),
    ])
      .then(([m, inv]) => {
        if (cancelled) return;
        setStatus({
          kind: "ready",
          members: m.members,
          invites: inv.invites,
          viewerRole: m.viewer_role,
          viewerId: m.viewer_id,
        });
      })
      .catch((err: Error) => {
        if (cancelled) return;
        setStatus({ kind: "error", message: err.message });
      });
    return () => {
      cancelled = true;
    };
  }, [project.slug, refreshNonce, reloadCounter]);

  async function handleRemove(userId: string) {
    if (pendingAction) return;
    setActionError(null);
    setPendingAction(`member:${userId}`);
    try {
      await api.removeMember(project.slug, userId);
      reload();
    } catch (e) {
      const err = e as Error & { error_code?: string };
      setActionError(
        err.error_code
          ? `${err.error_code}: ${err.message}`
          : err.message,
      );
    } finally {
      setPendingAction(null);
    }
  }

  async function handleRevoke(tokenHash: string) {
    if (pendingAction) return;
    setActionError(null);
    setPendingAction(`invite:${tokenHash}`);
    try {
      await api.revokeInvite(project.slug, tokenHash);
      reload();
    } catch (e) {
      const err = e as Error & { error_code?: string };
      setActionError(
        err.error_code
          ? `${err.error_code}: ${err.message}`
          : err.message,
      );
    } finally {
      setPendingAction(null);
    }
  }

  return (
    <section className="members-panel" aria-label={t("members_panel.label")}>
      {status.kind === "loading" && (
        <div className="members-panel__loading">
          <Loader2 className="lucide members-panel__spinner" aria-hidden />
          <span>{t("members_panel.loading")}</span>
        </div>
      )}
      {status.kind === "error" && (
        <div className="members-panel__error" role="alert">
          {status.message}
        </div>
      )}
      {actionError && (
        <div className="members-panel__error" role="alert">
          {actionError}
        </div>
      )}
      {status.kind === "ready" && (
        <>
          <MembersSection
            members={status.members}
            viewerRole={status.viewerRole}
            viewerId={status.viewerId}
            usersByID={usersByID}
            pendingAction={pendingAction}
            onRemove={handleRemove}
            t={t}
            lang={lang}
          />
          {status.viewerRole === "owner" && (
            <InvitesSection
              invites={status.invites}
              usersByID={usersByID}
              pendingAction={pendingAction}
              onRevoke={handleRevoke}
              t={t}
              lang={lang}
            />
          )}
        </>
      )}
    </section>
  );
}

function MembersSection({
  members,
  viewerRole,
  viewerId,
  usersByID,
  pendingAction,
  onRemove,
  t,
  lang,
}: {
  members: MemberRow[];
  viewerRole: string;
  viewerId?: string;
  usersByID: Map<string, UserRef>;
  pendingAction: string | null;
  onRemove: (userId: string) => void;
  t: (key: string, ...args: Array<string | number>) => string;
  lang: string;
}) {
  return (
    <div className="members-panel__group">
      <div className="members-panel__group-head">
        <h3>{t("members_panel.members_heading")}</h3>
        <span className="members-panel__count">{members.length}</span>
      </div>
      {members.length === 0 ? (
        <div className="members-panel__empty">{t("members_panel.members_empty")}</div>
      ) : (
        <ul className="members-panel__list">
          {members.map((m) => {
            const isSelf = m.is_self === true || (viewerId !== undefined && m.user_id === viewerId);
            const canRemove = viewerRole === "owner" || isSelf;
            const action = canRemove ? (
              <button
                type="button"
                className="members-panel__row-action"
                onClick={() => onRemove(m.user_id)}
                disabled={pendingAction === `member:${m.user_id}`}
                aria-label={
                  isSelf
                    ? t("members_panel.leave_aria", m.display_name || m.user_id)
                    : t("members_panel.remove_aria", m.display_name || m.user_id)
                }
              >
                {pendingAction === `member:${m.user_id}` ? (
                  <Loader2 className="lucide members-panel__spinner" aria-hidden />
                ) : (
                  <Trash2 className="lucide" aria-hidden />
                )}
                <span>{isSelf ? t("members_panel.leave") : t("members_panel.remove")}</span>
              </button>
            ) : null;
            return (
              <li key={m.user_id} className="members-panel__row">
                <div className="members-panel__row-main">
                  <span className="members-panel__row-name">
                    {m.display_name || m.user_id.slice(0, 8)}
                    {isSelf && <em>{` · ${t("members_panel.self")}`}</em>}
                  </span>
                  {m.github_handle && (
                    <span className="members-panel__row-handle">@{m.github_handle}</span>
                  )}
                </div>
                <div className="members-panel__row-meta">
                  <RoleChip role={m.role} t={t} />
                  <span className="members-panel__row-time">
                    {formatJoinedAt(m.joined_at, lang, t)}
                  </span>
                </div>
                {m.invited_by_id && (
                  <div className="members-panel__row-invited-by">
                    {t("members_panel.invited_by", inviterLabel(m.invited_by_id, usersByID))}
                  </div>
                )}
                {action}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function InvitesSection({
  invites,
  usersByID,
  pendingAction,
  onRevoke,
  t,
  lang,
}: {
  invites: ActiveInviteRow[];
  usersByID: Map<string, UserRef>;
  pendingAction: string | null;
  onRevoke: (tokenHash: string) => void;
  t: (key: string, ...args: Array<string | number>) => string;
  lang: string;
}) {
  return (
    <div className="members-panel__group">
      <div className="members-panel__group-head">
        <h3>{t("members_panel.invites_heading")}</h3>
        <span className="members-panel__count">{invites.length}</span>
      </div>
      {invites.length === 0 ? (
        <div className="members-panel__empty">{t("members_panel.invites_empty")}</div>
      ) : (
        <ul className="members-panel__list">
          {invites.map((inv) => (
            <li key={inv.token_hash} className="members-panel__row">
              <div className="members-panel__row-main">
                <span className="members-panel__row-name">
                  {t(`members_panel.invite_role_${inv.role}`)}
                </span>
                <span className="members-panel__row-handle members-panel__row-token">
                  {inv.token_hash.slice(0, 12)}…
                </span>
              </div>
              <div className="members-panel__row-meta">
                <span className="members-panel__row-time">
                  {t("members_panel.invite_expires_at", formatExpiry(inv.expires_at, lang))}
                </span>
                {inv.issued_by_id && (
                  <span className="members-panel__row-handle">
                    {t("members_panel.invite_issued_by", inviterLabel(inv.issued_by_id, usersByID))}
                  </span>
                )}
              </div>
              <button
                type="button"
                className="members-panel__row-action members-panel__row-action--revoke"
                onClick={() => onRevoke(inv.token_hash)}
                disabled={pendingAction === `invite:${inv.token_hash}`}
                aria-label={t("members_panel.revoke_aria", inv.token_hash.slice(0, 8))}
              >
                {pendingAction === `invite:${inv.token_hash}` ? (
                  <Loader2 className="lucide members-panel__spinner" aria-hidden />
                ) : (
                  <X className="lucide" aria-hidden />
                )}
                <span>{t("members_panel.revoke")}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function RoleChip({
  role,
  t,
}: {
  role: string;
  t: (key: string, ...args: Array<string | number>) => string;
}) {
  return (
    <span className={`members-panel__role-chip members-panel__role-chip--${role}`}>
      {role === "owner" && <ShieldCheck className="lucide" aria-hidden />}
      {t(`members_panel.role_${role}`)}
    </span>
  );
}

function inviterLabel(id: string, usersByID: Map<string, UserRef>): string {
  const u = usersByID.get(id);
  if (u?.display_name) return u.display_name;
  return id.slice(0, 8);
}

function formatJoinedAt(
  raw: string,
  lang: string,
  t: (key: string, ...args: Array<string | number>) => string,
): string {
  if (!raw) return "";
  const dt = new Date(raw);
  if (!Number.isFinite(dt.getTime())) return raw;
  const formatted = new Intl.DateTimeFormat(
    lang === "ko" ? "ko-KR" : "en-US",
    { dateStyle: "medium" },
  ).format(dt);
  return t("members_panel.joined_at", formatted);
}

function formatExpiry(raw: string, lang: string): string {
  const dt = new Date(raw);
  if (!Number.isFinite(dt.getTime())) return raw;
  return new Intl.DateTimeFormat(lang === "ko" ? "ko-KR" : "en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(dt);
}
