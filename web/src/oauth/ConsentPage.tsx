import { useEffect, useState } from "react";
import { Check, Loader2, X } from "lucide-react";
import { api, type OAuthConsentInfo } from "../api/client";
import { useI18n } from "../i18n";
import "../styles/telemetry.css";

export function ConsentPage() {
  const { t } = useI18n();
  const query = typeof window === "undefined" ? "" : window.location.search;
  const [info, setInfo] = useState<OAuthConsentInfo | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setErr(null);
    setSubmitting(false);
    api.oauthConsent(query)
      .then((next) => {
        if (!cancelled) setInfo(next);
      })
      .catch((e) => {
        if (!cancelled) setErr((e as Error).message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [query]);

  return (
    <main className="ops">
      <section className="ops__main">
        <div className="ops__panel">
          <header className="ops__panel-head">
            <Check className="lucide" aria-hidden />
            <h1>{t("oauth.consent.title")}</h1>
          </header>

          {loading && (
            <OAuthLoadingStatus label={t("oauth.consent.loading")} />
          )}

          {err && (
            <div className="ops__toast ops__toast--err" role="alert">
              <X className="lucide" aria-hidden />
              <span>{err}</span>
            </div>
          )}

          {info && !loading && (
            <ConsentGrantPanel
              info={info}
              query={query}
              submitting={submitting}
              onSubmit={() => setSubmitting(true)}
            />
          )}
        </div>
      </section>
    </main>
  );
}

export function OAuthLoadingStatus({ label }: { label: string }) {
  return (
    <div className="ops__loading" role="status" aria-live="polite">
      <Loader2 className="lucide ops__spin" aria-hidden />
      <span>{label}</span>
    </div>
  );
}

export function ConsentGrantPanel({
  info,
  query,
  submitting = false,
  onSubmit,
}: {
  info: OAuthConsentInfo;
  query: string;
  submitting?: boolean;
  onSubmit?: () => void;
}) {
  const { t } = useI18n();
  const trust = trustSignal(info.created_via, t);
  const redirectHost = firstRedirectHost(info.redirect_uris);
  const recentLabel = recentlyRegisteredLabel(info.created_at, t);
  const scopes = info.scopes ?? [];

  return (
    <>
      <p className="ops__hint">
        {t("oauth.consent.intro", info.client_display_name)}
      </p>

      <div className="ops__trust">
        <span className={`ops__chip ${trust.className}`}>{trust.label}</span>
        <span>{trust.description}</span>
        {redirectHost && <span>{t("oauth.consent.redirect_to", redirectHost)}</span>}
        {recentLabel && <span>{recentLabel}</span>}
      </div>

      <dl className="ops__summary">
        <div>
          <dt>{t("oauth.consent.client_id")}</dt>
          <dd><code>{info.client_id}</code></dd>
        </div>
        <div>
          <dt>{t("oauth.consent.scopes")}</dt>
          <dd>
            {scopes.length === 0 ? (
              t("oauth.consent.no_scopes")
            ) : (
              <ul className="ops__scope-list">
                {scopes.map((scope) => {
                  const desc = scopeDescription(scope, t);
                  return (
                    <li key={scope}>
                      <strong>{desc.title}</strong>
                      <code>{scope}</code>
                      <p>{desc.body}</p>
                    </li>
                  );
                })}
              </ul>
            )}
          </dd>
        </div>
      </dl>

      <form
        method="post"
        action="/oauth/authorize/confirm"
        className="ops__form-actions"
        onSubmit={onSubmit}
      >
        <input type="hidden" name="query" value={query} />
        <input type="hidden" name="consent_nonce" value={info.consent_nonce} />
        <button
          type="submit"
          name="action"
          value="approve"
          className="ops__primary"
          disabled={submitting}
        >
          {submitting ? <Loader2 className="lucide ops__spin" aria-hidden /> : <Check className="lucide" aria-hidden />}
          <span>{submitting ? t("oauth.consent.submitting") : t("oauth.consent.approve")}</span>
        </button>
        <button
          type="submit"
          name="action"
          value="deny"
          className="ops__danger"
          disabled={submitting}
        >
          {submitting ? <Loader2 className="lucide ops__spin" aria-hidden /> : <X className="lucide" aria-hidden />}
          <span>{submitting ? t("oauth.consent.submitting") : t("oauth.consent.deny")}</span>
        </button>
      </form>
    </>
  );
}

type T = (key: string, ...args: Array<string | number>) => string;

function scopeDescription(scope: string, t: T): { title: string; body: string } {
  const titleKey = `oauth.scope.${scope}.title`;
  const descKey = `oauth.scope.${scope}.desc`;
  const title = t(titleKey);
  const body = t(descKey);
  if (title !== titleKey && body !== descKey) {
    return { title, body };
  }
  return {
    title: scope,
    body: t("oauth.consent.scope_unknown_desc", scope),
  };
}

function trustSignal(createdVia: string | undefined, t: T): { label: string; description: string; className: string } {
  switch (createdVia) {
    case "env_seed":
      return {
        label: t("oauth.consent.trust.env_seed.label"),
        description: t("oauth.consent.trust.env_seed.desc"),
        className: "ops__chip--ok",
      };
    case "admin_ui":
      return {
        label: t("oauth.consent.trust.admin_ui.label"),
        description: t("oauth.consent.trust.admin_ui.desc"),
        className: "ops__chip--ok",
      };
    case "dcr":
      return {
        label: t("oauth.consent.trust.dcr.label"),
        description: t("oauth.consent.trust.dcr.desc"),
        className: "ops__chip--warn",
      };
    default:
      return {
        label: t("oauth.consent.trust.unknown.label"),
        description: t("oauth.consent.trust.unknown.desc"),
        className: "ops__chip--warn",
      };
  }
}

function firstRedirectHost(redirectURIs: string[] | undefined): string | null {
  const first = redirectURIs?.[0]?.trim();
  if (!first) return null;
  try {
    return new URL(first).host;
  } catch {
    return first;
  }
}

function recentlyRegisteredLabel(createdAt: string | undefined, t: T): string | null {
  if (!createdAt) return null;
  const createdMs = Date.parse(createdAt);
  if (!Number.isFinite(createdMs)) return null;
  const elapsedHours = Math.floor((Date.now() - createdMs) / 3_600_000);
  if (elapsedHours < 0 || elapsedHours >= 24) return null;
  return t("oauth.consent.trust.recently_registered", Math.max(1, elapsedHours));
}
