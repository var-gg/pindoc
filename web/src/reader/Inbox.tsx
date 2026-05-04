import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { Check, RefreshCw, X } from "lucide-react";
import { Link, useNavigate } from "react-router";
import { api, type ArtifactRef, type Project } from "../api/client";
import { useI18n } from "../i18n";
import { projectRoutePrefix, projectSurfacePath } from "../readerRoutes";
import { ArtifactByline } from "./ArtifactByline";
import { EmptyState, SurfaceHeader } from "./SurfacePrimitives";
import { Tooltip } from "./Tooltip";
import { ArtifactTypeChip, VisualAreaChip } from "./VisualChips";
import { formatDateTime } from "../utils/formatDateTime";

type Props = {
  projectSlug: string;
  orgSlug: string;
  enabled?: boolean;
  projectRole?: Project["current_role"];
  onCountChange?: (count: number) => void;
};

type ReviewAction = "approve" | "reject";

type PendingReview = {
  item: ArtifactRef;
  action: ReviewAction;
};

const CARD_CONTROL_SELECTOR = "a, button, input, textarea, select, [contenteditable='true']";
const DIALOG_FOCUS_SELECTOR = "button, textarea, input, select, a[href], [tabindex]:not([tabindex='-1'])";

export function inboxCardA11yPosition(index: number, total: number): { "aria-posinset": number; "aria-setsize": number } {
  return {
    "aria-posinset": index + 1,
    "aria-setsize": Math.max(total, 0),
  };
}

export async function refreshInboxAfterReview(load: () => Promise<boolean>): Promise<boolean> {
  return load();
}

export function nextInboxFocusIndex(current: number, count: number, direction: -1 | 1): number {
  if (count <= 0) return 0;
  return (current + direction + count) % count;
}

export function Inbox({
  projectSlug,
  orgSlug,
  enabled = true,
  projectRole,
  onCountChange,
}: Props) {
  const { t, lang } = useI18n();
  const navigate = useNavigate();
  const [items, setItems] = useState<ArtifactRef[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [truncated, setTruncated] = useState(false);
  const [limit, setLimit] = useState(0);
  const [loading, setLoading] = useState(() => enabled);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refreshWarning, setRefreshWarning] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [busySlug, setBusySlug] = useState<string | null>(null);
  const [pendingReview, setPendingReview] = useState<PendingReview | null>(null);
  const [reviewNote, setReviewNote] = useState("");
  const [reviewNoteError, setReviewNoteError] = useState<string | null>(null);
  const [focusIndex, setFocusIndex] = useState(0);
  const cardRefs = useRef<Array<HTMLElement | null>>([]);
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const noteRef = useRef<HTMLTextAreaElement | null>(null);

  function applyInboxResponse(resp: Awaited<ReturnType<typeof api.inbox>>) {
    const serverTotal = resp.total_count ?? resp.count;
    setItems(resp.items);
    setTotalCount(serverTotal);
    setTruncated(Boolean(resp.truncated));
    setLimit(resp.limit ?? resp.items.length);
    onCountChange?.(serverTotal);
  }

  async function load(options: { warnOnly?: boolean } = {}): Promise<boolean> {
    if (!enabled) {
      setItems([]);
      setTotalCount(0);
      setTruncated(false);
      setLimit(0);
      setLoading(false);
      setRefreshing(false);
      setError(null);
      setRefreshWarning(null);
      onCountChange?.(0);
      return true;
    }
    setRefreshing(true);
    if (!options.warnOnly) setError(null);
    try {
      const resp = await api.inbox(projectSlug);
      applyInboxResponse(resp);
      setRefreshWarning(null);
      return true;
    } catch (e) {
      console.error("Inbox refresh failed", e);
      if (options.warnOnly) {
        setRefreshWarning(t("inbox.review_refresh_warning"));
      } else {
        setError(t("inbox.error_load"));
      }
      return false;
    } finally {
      setRefreshing(false);
    }
  }

  useEffect(() => {
    if (!enabled) {
      setItems([]);
      setTotalCount(0);
      setTruncated(false);
      setLimit(0);
      setLoading(false);
      setRefreshing(false);
      setError(null);
      setRefreshWarning(null);
      setSuccess(null);
      onCountChange?.(0);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setRefreshing(false);
    setError(null);
    api.inbox(projectSlug)
      .then((resp) => {
        if (cancelled) return;
        applyInboxResponse(resp);
      })
      .catch((e) => {
        console.error("Inbox load failed", e);
        if (!cancelled) setError(t("inbox.error_load"));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [projectSlug, enabled, onCountChange, t]);

  useEffect(() => {
    setFocusIndex((current) => Math.min(current, Math.max(0, items.length - 1)));
    cardRefs.current = cardRefs.current.slice(0, items.length);
  }, [items.length]);

  useEffect(() => {
    if (!pendingReview) return;
    const frame = window.requestAnimationFrame(() => noteRef.current?.focus());
    return () => window.cancelAnimationFrame(frame);
  }, [pendingReview]);

  function beginReview(item: ArtifactRef, action: ReviewAction) {
    setPendingReview({ item, action });
    setReviewNote("");
    setReviewNoteError(null);
    setError(null);
    setRefreshWarning(null);
    setSuccess(null);
  }

  function cancelReview() {
    if (busySlug) return;
    setPendingReview(null);
    setReviewNote("");
    setReviewNoteError(null);
  }

  async function confirmReview() {
    if (!pendingReview || busySlug) return;
    const { item, action } = pendingReview;
    if (action === "reject" && reviewNote.trim() === "") {
      setReviewNoteError(t("inbox.reject_reason_required"));
      noteRef.current?.focus();
      return;
    }
    setBusySlug(item.slug);
    setError(null);
    setRefreshWarning(null);
    setReviewNoteError(null);
    try {
      await api.inboxReview(projectSlug, item.slug, action, {
        commitMsg: reviewNote,
      });
      const next = items.filter((x) => x.id !== item.id);
      setItems(next);
      onCountChange?.(next.length);
      setSuccess(t(action === "approve" ? "inbox.approved_toast" : "inbox.rejected_toast"));
      setPendingReview(null);
      setReviewNote("");
      await refreshInboxAfterReview(() => load({ warnOnly: true }));
    } catch (e) {
      console.error("Inbox review failed", e);
      setError(t("inbox.error_review"));
    } finally {
      setBusySlug(null);
    }
  }

  function submitReview(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    void confirmReview();
  }

  function focusCard(index: number) {
    if (items.length === 0) return;
    const next = Math.max(0, Math.min(index, items.length - 1));
    setFocusIndex(next);
    window.requestAnimationFrame(() => cardRefs.current[next]?.focus());
  }

  function handleCardKeyDown(e: ReactKeyboardEvent<HTMLElement>, item: ArtifactRef, index: number) {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    const target = e.target as HTMLElement | null;
    if (target && target !== e.currentTarget && target.closest(CARD_CONTROL_SELECTOR)) return;
    const key = e.key.toLowerCase();
    if (key === "arrowdown" || key === "j") {
      e.preventDefault();
      focusCard(nextInboxFocusIndex(index, items.length, 1));
      return;
    }
    if (key === "arrowup" || key === "k") {
      e.preventDefault();
      focusCard(nextInboxFocusIndex(index, items.length, -1));
      return;
    }
    if (key === "a") {
      e.preventDefault();
      beginReview(item, "approve");
      return;
    }
    if (key === "r") {
      e.preventDefault();
      beginReview(item, "reject");
      return;
    }
    if (key === "enter" || key === "o") {
      e.preventDefault();
      navigate(projectSurfacePath(projectSlug, "wiki", item.slug, orgSlug));
    }
  }

  function handleDialogKeyDown(e: ReactKeyboardEvent<HTMLDivElement>) {
    if (e.key === "Escape") {
      e.preventDefault();
      cancelReview();
      return;
    }
    if (e.key !== "Tab") return;
    const nodes = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>(DIALOG_FOCUS_SELECTOR) ?? [])
      .filter((node) => !node.hasAttribute("disabled") && node.tabIndex !== -1);
    if (nodes.length === 0) return;
    const first = nodes[0]!;
    const last = nodes[nodes.length - 1]!;
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  const disabledAction = !enabled
    ? projectRole === "owner"
      ? {
          label: t("inbox.disabled_empty_cta"),
          href: `${projectRoutePrefix(projectSlug, orgSlug)}/settings`,
        }
      : {
          label: t("inbox.disabled_empty_admin"),
        }
    : undefined;

  return (
    <main className="content">
      <div className="surface-panel inbox-surface">
        <div className="inbox-surface__head">
          <SurfaceHeader name="inbox" count={totalCount} />
          <Tooltip content={t("inbox.refresh")}>
            <button type="button" className="inbox-icon-button" onClick={() => void load()} disabled={loading || refreshing || !enabled}>
              <RefreshCw className={refreshing ? "inbox-spin" : undefined} size={15} aria-hidden="true" />
              <span className="sr-only">{t("inbox.refresh")}</span>
            </button>
          </Tooltip>
        </div>

        {error && <div className="inbox-error" role="alert">{error}</div>}
        {refreshWarning && <div className="inbox-error" role="alert">{refreshWarning}</div>}
        {success && <div className="inbox-success" role="status" aria-live="polite">{success}</div>}
        {truncated && (
          <div className="inbox-success" role="status" aria-live="polite">
            {t("inbox.truncated_notice", items.length, totalCount, limit)}
          </div>
        )}
        {loading && <EmptyState message={t("inbox.loading")} />}
        {!loading && items.length === 0 && (
          <EmptyState
            message={enabled ? t("wiki.stub_inbox") : t("inbox.disabled_empty")}
            action={enabled ? undefined : disabledAction}
          />
        )}
        {!loading && items.length > 0 && (
          <div className="inbox-list" role="list" aria-label={t("inbox.list_aria", totalCount)}>
            {items.map((item, index) => (
              <article
                key={item.id}
                ref={(node) => {
                  cardRefs.current[index] = node;
                }}
                className="inbox-card"
                tabIndex={index === focusIndex ? 0 : -1}
                onFocus={() => setFocusIndex(index)}
                onKeyDown={(e) => handleCardKeyDown(e, item, index)}
                aria-label={t("inbox.card_aria", item.title)}
                role="listitem"
                {...inboxCardA11yPosition(index, totalCount)}
              >
                <div className="inbox-card__meta">
                  <ArtifactTypeChip type={item.type} />
                  <VisualAreaChip areaSlug={item.area_slug} label={item.area_slug} />
                  <span className="inbox-card__state">{t("inbox.pending_review")}</span>
                </div>
                <h2 className="inbox-card__title">
                  <Link to={projectSurfacePath(projectSlug, "wiki", item.slug, orgSlug)}>
                    {item.title}
                  </Link>
                </h2>
                <div className="inbox-card__byline">
                  <ArtifactByline artifact={item} variant="list" />
                  <span>{formatDateTime(item.updated_at, lang)}</span>
                </div>
                <div className="inbox-card__actions">
                  <button
                    type="button"
                    className="inbox-action inbox-action--approve"
                    onClick={() => beginReview(item, "approve")}
                    disabled={busySlug === item.slug}
                  >
                    <Check size={14} aria-hidden="true" />
                    {t("inbox.approve")}
                  </button>
                  <button
                    type="button"
                    className="inbox-action inbox-action--reject"
                    onClick={() => beginReview(item, "reject")}
                    disabled={busySlug === item.slug}
                  >
                    <X size={14} aria-hidden="true" />
                    {t("inbox.reject")}
                  </button>
                </div>
              </article>
            ))}
          </div>
        )}
        {pendingReview && (
          <div className="inbox-review-dialog-backdrop" onKeyDown={handleDialogKeyDown}>
            <div
              ref={dialogRef}
              className="inbox-review-dialog"
              role="dialog"
              aria-modal="true"
              aria-labelledby="inbox-review-dialog-title"
              aria-describedby="inbox-review-dialog-desc"
            >
              <form onSubmit={submitReview}>
                <header className="inbox-review-dialog__head">
                  <h2 id="inbox-review-dialog-title">
                    {t(pendingReview.action === "approve" ? "inbox.dialog_approve_title" : "inbox.dialog_reject_title")}
                  </h2>
                  <button
                    type="button"
                    className="inbox-review-dialog__close"
                    onClick={cancelReview}
                    aria-label={t("inbox.dialog_cancel")}
                    disabled={busySlug === pendingReview.item.slug}
                  >
                    <X size={16} aria-hidden="true" />
                  </button>
                </header>
                <p id="inbox-review-dialog-desc" className="inbox-review-dialog__desc">
                  {pendingReview.item.title}
                </p>
                <label className="inbox-review-dialog__label" htmlFor="inbox-review-note">
                  {t("inbox.dialog_note_label")}
                </label>
                <textarea
                  id="inbox-review-note"
                  ref={noteRef}
                  value={reviewNote}
                  onChange={(e) => setReviewNote(e.currentTarget.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      void confirmReview();
                    }
                  }}
                  rows={4}
                  aria-invalid={Boolean(reviewNoteError)}
                  aria-describedby={reviewNoteError ? "inbox-review-note-error" : undefined}
                  required={pendingReview.action === "reject"}
                  placeholder={t(
                    pendingReview.action === "approve"
                      ? "inbox.dialog_approve_placeholder"
                      : "inbox.dialog_reject_placeholder",
                  )}
                  disabled={busySlug === pendingReview.item.slug}
                />
                {reviewNoteError && (
                  <p id="inbox-review-note-error" className="inbox-error" role="alert">
                    {reviewNoteError}
                  </p>
                )}
                <footer className="inbox-review-dialog__actions">
                  <button
                    type="button"
                    className="inbox-action"
                    onClick={cancelReview}
                    disabled={busySlug === pendingReview.item.slug}
                  >
                    {t("inbox.dialog_cancel")}
                  </button>
                  <button
                    type="submit"
                    className={`inbox-action ${pendingReview.action === "approve" ? "inbox-action--approve" : "inbox-action--reject"}`}
                    disabled={busySlug === pendingReview.item.slug}
                  >
                    {t(pendingReview.action === "approve" ? "inbox.dialog_confirm_approve" : "inbox.dialog_confirm_reject")}
                  </button>
                </footer>
              </form>
            </div>
          </div>
        )}
      </div>
    </main>
  );
}
