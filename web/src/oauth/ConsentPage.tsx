import { useEffect, useMemo, useState } from "react";
import { Check, Loader2, X } from "lucide-react";
import { api, type OAuthConsentInfo } from "../api/client";
import "../styles/telemetry.css";

export function ConsentPage() {
  const query = window.location.search;
  const [info, setInfo] = useState<OAuthConsentInfo | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setErr(null);
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

  const scopeLabel = useMemo(() => (info?.scopes ?? []).join(", "), [info]);

  return (
    <main className="ops">
      <section className="ops__main">
        <div className="ops__panel">
          <header className="ops__panel-head">
            <Check className="lucide" aria-hidden />
            <h1>Authorize OAuth client</h1>
          </header>

          {loading && (
            <div className="ops__loading">
              <Loader2 className="lucide" aria-hidden /> Loading...
            </div>
          )}

          {err && (
            <div className="ops__toast ops__toast--err" role="alert">
              <X className="lucide" aria-hidden />
              <span>{err}</span>
            </div>
          )}

          {info && !loading && (
            <>
              <p className="ops__hint">
                <strong>{info.client_display_name}</strong> is requesting access
                to this Pindoc account.
              </p>
              <dl className="ops__summary">
                <div>
                  <dt>Client ID</dt>
                  <dd><code>{info.client_id}</code></dd>
                </div>
                <div>
                  <dt>Scopes</dt>
                  <dd>{scopeLabel || "none"}</dd>
                </div>
              </dl>

              <div className="ops__form-actions">
                <form method="post" action="/oauth/authorize/confirm">
                  <input type="hidden" name="action" value="approve" />
                  <input type="hidden" name="query" value={query} />
                  <button type="submit" className="ops__primary">
                    <Check className="lucide" aria-hidden /> Approve
                  </button>
                </form>
                <form method="post" action="/oauth/authorize/confirm">
                  <input type="hidden" name="action" value="deny" />
                  <input type="hidden" name="query" value={query} />
                  <button type="submit" className="ops__danger">
                    <X className="lucide" aria-hidden /> Deny
                  </button>
                </form>
              </div>
            </>
          )}
        </div>
      </section>
    </main>
  );
}
