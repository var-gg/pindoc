import { useEffect, useState } from "react";
import { Check, RefreshCw, X } from "lucide-react";
import { Link } from "react-router";
import { api, type ArtifactRef } from "../api/client";
import { useI18n } from "../i18n";
import { ArtifactByline } from "./ArtifactByline";
import { EmptyState, SurfaceHeader } from "./SurfacePrimitives";
import { Tooltip } from "./Tooltip";
import { ArtifactTypeChip, VisualAreaChip } from "./VisualChips";

type Props = {
  projectSlug: string;
  enabled?: boolean;
  onCountChange?: (count: number) => void;
};

type ReviewAction = "approve" | "reject";

export function Inbox({ projectSlug, enabled = true, onCountChange }: Props) {
  const { t } = useI18n();
  const [items, setItems] = useState<ArtifactRef[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busySlug, setBusySlug] = useState<string | null>(null);

  async function load() {
    if (!enabled) {
      setItems([]);
      setLoading(false);
      setError(null);
      onCountChange?.(0);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const resp = await api.inbox(projectSlug);
      setItems(resp.items);
      onCountChange?.(resp.count);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (!enabled) {
      setItems([]);
      setLoading(false);
      setError(null);
      onCountChange?.(0);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    api.inbox(projectSlug)
      .then((resp) => {
        if (cancelled) return;
        setItems(resp.items);
        onCountChange?.(resp.count);
      })
      .catch((e) => {
        if (!cancelled) setError(String(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [projectSlug, enabled, onCountChange]);

  async function review(item: ArtifactRef, action: ReviewAction) {
    setBusySlug(item.slug);
    setError(null);
    try {
      await api.inboxReview(projectSlug, item.slug, action);
      const next = items.filter((x) => x.id !== item.id);
      setItems(next);
      onCountChange?.(next.length);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusySlug(null);
    }
  }

  return (
    <main className="content">
      <div className="surface-panel inbox-surface">
        <div className="inbox-surface__head">
          <SurfaceHeader name="inbox" count={items.length} />
          <Tooltip content={t("inbox.refresh")}>
            <button type="button" className="inbox-icon-button" onClick={load} disabled={loading || !enabled}>
              <RefreshCw size={15} aria-hidden="true" />
              <span className="sr-only">{t("inbox.refresh")}</span>
            </button>
          </Tooltip>
        </div>

        {error && <div className="inbox-error">{error}</div>}
        {loading && <EmptyState message={t("inbox.loading")} />}
        {!loading && items.length === 0 && (
          <EmptyState message={enabled ? t("wiki.stub_inbox") : t("inbox.disabled_empty")} />
        )}
        {!loading && items.length > 0 && (
          <div className="inbox-list">
            {items.map((item) => (
              <article key={item.id} className="inbox-card">
                <div className="inbox-card__meta">
                  <ArtifactTypeChip type={item.type} />
                  <VisualAreaChip areaSlug={item.area_slug} label={item.area_slug} />
                  <span className="inbox-card__state">{t("inbox.pending_review")}</span>
                </div>
                <h2 className="inbox-card__title">
                  <Link to={`/p/${projectSlug}/wiki/${encodeURIComponent(item.slug)}`}>
                    {item.title}
                  </Link>
                </h2>
                <div className="inbox-card__byline">
                  <ArtifactByline artifact={item} variant="list" />
                  <span>{new Date(item.updated_at).toLocaleString()}</span>
                </div>
                <div className="inbox-card__actions">
                  <button
                    type="button"
                    className="inbox-action inbox-action--approve"
                    onClick={() => review(item, "approve")}
                    disabled={busySlug === item.slug}
                  >
                    <Check size={14} aria-hidden="true" />
                    {t("inbox.approve")}
                  </button>
                  <button
                    type="button"
                    className="inbox-action inbox-action--reject"
                    onClick={() => review(item, "reject")}
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
      </div>
    </main>
  );
}
