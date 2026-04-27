import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy, Loader2, X } from "lucide-react";
import {
  api,
  type InviteIssueResp,
  type InviteRole,
  type Project,
  type ServerConfig,
  type UserRef,
} from "../api/client";
import { useI18n } from "../i18n";
import { MembersPanel } from "./MembersPanel";

const EXPIRY_OPTIONS = [24, 72, 168, 720] as const;

type Props = {
  project: Project;
  open: boolean;
  onClose: () => void;
  // Phase D — auth_mode + users are passed through so MembersPanel
  // (which renders below the issue form) can decide whether to show
  // itself and how to label invited_by ids. Both optional so the
  // modal still works in legacy callers / snapshot tests.
  authMode?: ServerConfig["auth_mode"];
  users?: UserRef[] | null;
};

export function InviteModal({ project, open, onClose, authMode, users }: Props) {
  const { t, lang } = useI18n();
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [role, setRole] = useState<InviteRole>("viewer");
  const [expiresInHours, setExpiresInHours] = useState<number>(24);
  const [issuing, setIssuing] = useState(false);
  const [result, setResult] = useState<InviteIssueResp | null>(null);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refreshNonce, setRefreshNonce] = useState(0);

  const formattedExpiry = useMemo(() => {
    if (!result?.expires_at) return "";
    return new Intl.DateTimeFormat(lang === "ko" ? "ko-KR" : "en-US", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(result.expires_at));
  }, [lang, result?.expires_at]);

  useEffect(() => {
    if (!open) return;
    setError(null);
    setCopied(false);
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  async function handleIssue(e: React.FormEvent) {
    e.preventDefault();
    if (issuing) return;
    setIssuing(true);
    setError(null);
    setResult(null);
    setCopied(false);
    try {
      const out = await api.issueInvite(project.slug, {
        role,
        expires_in_hours: expiresInHours,
      });
      setResult(out);
      // Tell MembersPanel to refetch — the new token belongs in the
      // active-invites list right away. Without this the user has to
      // close + reopen the modal before they see what they just issued.
      setRefreshNonce((n) => n + 1);
    } catch (e) {
      const err = e as Error & { error_code?: string };
      setError(err.error_code ? `${err.error_code}: ${err.message}` : err.message);
    } finally {
      setIssuing(false);
    }
  }

  async function copyInvite() {
    if (!result?.invite_url) return;
    await navigator.clipboard.writeText(result.invite_url);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div
      className="invite-modal"
      role="presentation"
      onMouseDown={(e) => {
        if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
          onClose();
        }
      }}
    >
      <div
        className="invite-modal__panel"
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="invite-modal-title"
      >
        <header className="invite-modal__header">
          <div>
            <p className="invite-modal__eyebrow">{project.slug}</p>
            <h2 id="invite-modal-title">{t("invite.modal.title")}</h2>
          </div>
          <button
            type="button"
            className="invite-modal__close"
            onClick={onClose}
            aria-label={t("invite.modal.close")}
          >
            <X className="lucide" />
          </button>
        </header>

        <form className="invite-modal__form" onSubmit={handleIssue}>
          <fieldset className="invite-modal__field">
            <legend>{t("invite.modal.role")}</legend>
            <div className="invite-modal__segments">
              {(["viewer", "editor"] as const).map((nextRole) => (
                <label key={nextRole} className={`invite-modal__segment${role === nextRole ? " is-active" : ""}`}>
                  <input
                    type="radio"
                    name="invite-role"
                    value={nextRole}
                    checked={role === nextRole}
                    onChange={() => setRole(nextRole)}
                  />
                  <span>{t(`invite.role.${nextRole}`)}</span>
                </label>
              ))}
            </div>
          </fieldset>

          <label className="invite-modal__field">
            <span>{t("invite.modal.expiry")}</span>
            <select
              value={expiresInHours}
              onChange={(e) => setExpiresInHours(Number(e.target.value))}
            >
              {EXPIRY_OPTIONS.map((hours) => (
                <option key={hours} value={hours}>
                  {t(`invite.expiry.${hours}`)}
                </option>
              ))}
            </select>
          </label>

          {error && <div className="invite-modal__error" role="alert">{error}</div>}

          <button type="submit" className="invite-modal__submit" disabled={issuing}>
            {issuing && <Loader2 className="lucide invite-modal__spinner" aria-hidden />}
            {issuing ? t("invite.modal.issuing") : t("invite.modal.issue")}
          </button>
        </form>

        {result && (
          <section className="invite-modal__result" aria-live="polite">
            <dl>
              <div>
                <dt>{t("invite.modal.expires_at")}</dt>
                <dd>{formattedExpiry}</dd>
              </div>
              <div>
                <dt>{t("invite.modal.url")}</dt>
                <dd>{result.invite_url}</dd>
              </div>
            </dl>
            <button type="button" className="invite-modal__copy" onClick={copyInvite}>
              {copied ? <Check className="lucide" /> : <Copy className="lucide" />}
              {copied ? t("invite.modal.copied") : t("invite.modal.copy")}
            </button>
          </section>
        )}

        <MembersPanel
          project={project}
          authMode={authMode}
          refreshNonce={refreshNonce}
          users={users}
        />
      </div>
    </div>
  );
}
