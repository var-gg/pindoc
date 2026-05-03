import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { Check, KeyRound, Loader2, Plus, ShieldCheck, Trash2, X } from "lucide-react";
import {
  api,
  type InstanceProvider,
  type InstanceProvidersResp,
  type OAuthClient,
  type OAuthClientsResp,
} from "../api/client";
import "../styles/telemetry.css";

// ProvidersPanel — instance-level identity provider admin.
//
// Decision decision-auth-model-loopback-and-providers § 3 + task-
// providers-admin-ui: env seeds the boot-time IdP list, this page
// mutates the DB row at runtime so credential rotation / IdP toggling
// works without a daemon restart. Loopback principal only — non-
// loopback callers see INSTANCE_OWNER_REQUIRED.
//
// V1 supports the GitHub IdP only. The framework is ready for more
// (`provider_name` is a string, the BE allow-list is one map);
// adding google/passkey is a follow-up commit on top of this surface.

type ToastKind = "ok" | "err";
type Toast = { kind: ToastKind; message: string };

type FormState = {
  clientId: string;
  clientSecret: string;
  enabled: boolean;
};

const PROVIDER_NAME = "github";

export function ProvidersPanel() {
  const [data, setData] = useState<InstanceProvidersResp | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [errCode, setErrCode] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [toast, setToast] = useState<Toast | null>(null);
  const [form, setForm] = useState<FormState>({ clientId: "", clientSecret: "", enabled: true });

  const existing = useMemo<InstanceProvider | null>(() => {
    if (!data?.providers) return null;
    return data.providers.find((p) => p.provider_name === PROVIDER_NAME) ?? null;
  }, [data]);

  const load = useCallback(async () => {
    setLoading(true);
    setErr(null);
    setErrCode(null);
    try {
      const resp = await api.instanceProviders();
      setData(resp);
    } catch (e) {
      const tagged = e as Error & { error_code?: string };
      setErr(tagged.message);
      setErrCode(tagged.error_code ?? null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // Pre-fill the form when an existing row arrives. Secret stays blank
  // — operators rotate by typing a fresh value, the BE preserves the
  // stored ciphertext on empty input.
  useEffect(() => {
    if (existing) {
      setForm({ clientId: existing.client_id, clientSecret: "", enabled: existing.enabled });
    } else {
      setForm({ clientId: "", clientSecret: "", enabled: true });
    }
  }, [existing]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (submitting) return;
    setSubmitting(true);
    setToast(null);
    try {
      const resp = await api.upsertInstanceProvider({
        provider_name: PROVIDER_NAME,
        client_id: form.clientId.trim(),
        client_secret: form.clientSecret,
        enabled: form.enabled,
      });
      setToast({
        kind: "ok",
        message: existing ? "GitHub IdP credentials updated." : "GitHub IdP activated.",
      });
      setForm((f) => ({ ...f, clientSecret: "" }));
      // Splice the response row into local state so the panel reflects
      // the new row without a second round-trip.
      setData((d) => {
        if (!resp.provider) return d;
        const next = (d?.providers ?? []).filter((p) => p.provider_name !== resp.provider!.provider_name);
        next.push(resp.provider);
        return { providers: next };
      });
    } catch (e) {
      const tagged = e as Error & { error_code?: string };
      setToast({
        kind: "err",
        message: tagged.error_code ? `${tagged.error_code}: ${tagged.message}` : tagged.message,
      });
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete() {
    if (!existing || deleting) return;
    if (!window.confirm("Disable GitHub IdP and remove stored credentials?")) return;
    setDeleting(true);
    setToast(null);
    try {
      await api.deleteInstanceProvider(PROVIDER_NAME);
      setData((d) => ({
        providers: (d?.providers ?? []).filter((p) => p.provider_name !== PROVIDER_NAME),
      }));
      setToast({ kind: "ok", message: "GitHub IdP disabled." });
    } catch (e) {
      const tagged = e as Error & { error_code?: string };
      setToast({
        kind: "err",
        message: tagged.error_code ? `${tagged.error_code}: ${tagged.message}` : tagged.message,
      });
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div className="ops">
      <header className="ops__bar">
        <Link to="/" className="ops__back">◀ Reader</Link>
        <h1 className="ops__title">Identity providers</h1>
      </header>

      <main className="ops__main">
        {loading && (
          <div className="ops__loading">
            <Loader2 className="lucide" aria-hidden /> Loading…
          </div>
        )}

        {err && errCode === "INSTANCE_OWNER_REQUIRED" && (
          <section className="ops__panel" role="alert">
            <h2>Instance owner only</h2>
            <p>
              This admin surface is reachable from the loopback (127.0.0.1) only.
              Open the Reader from the same machine that runs the daemon, or
              configure providers via <code>PINDOC_AUTH_PROVIDERS</code> and the
              GitHub OAuth env vars.
            </p>
          </section>
        )}

        {err && errCode === "INSTANCE_KEY_MISSING" && (
          <section className="ops__panel" role="alert">
            <h2>Instance key required</h2>
            <p>
              Set <code>PINDOC_INSTANCE_KEY</code> to a 32-byte base64-encoded
              value before storing IdP credentials. Generate one with
              <code> openssl rand -base64 32 </code> and restart the daemon.
            </p>
          </section>
        )}

        {err && errCode === "PROVIDERS_UNAVAILABLE" && (
          <section className="ops__panel" role="alert">
            <h2>Provider store offline</h2>
            <p>The instance provider store is not configured for this build.</p>
          </section>
        )}

        {err && !errCode && (
          <section className="ops__panel" role="alert">
            <h2>Failed to load</h2>
            <p>{err}</p>
          </section>
        )}

        {!err && !loading && (
          <section className="ops__panel">
            <header className="ops__panel-head">
              <ShieldCheck className="lucide" aria-hidden />
              <h2>GitHub OAuth</h2>
              {existing && existing.enabled && (
                <span className="ops__chip ops__chip--ok">enabled</span>
              )}
              {existing && !existing.enabled && (
                <span className="ops__chip ops__chip--warn">disabled</span>
              )}
            </header>

            <p className="ops__hint">
              Activating GitHub OAuth lets external collaborators sign in with
              their GitHub identity. The client secret is encrypted at rest
              (<code>PINDOC_INSTANCE_KEY</code>); changes apply on the next
              request — no restart needed.
            </p>

            <form onSubmit={handleSubmit} className="ops__form">
              <label className="ops__field">
                <span>Client ID</span>
                <input
                  type="text"
                  value={form.clientId}
                  onChange={(e) => setForm((f) => ({ ...f, clientId: e.target.value }))}
                  placeholder="Iv1.xxxxxxxxxxxxxxxx"
                  autoComplete="off"
                  spellCheck={false}
                  required
                />
              </label>
              <label className="ops__field">
                <span>
                  Client secret
                  {existing?.has_client_secret && (
                    <em> (leave blank to keep the stored value)</em>
                  )}
                </span>
                <input
                  type="password"
                  value={form.clientSecret}
                  onChange={(e) => setForm((f) => ({ ...f, clientSecret: e.target.value }))}
                  autoComplete="off"
                  spellCheck={false}
                  required={!existing}
                />
              </label>
              <label className="ops__field ops__field--inline">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
                />
                <span>Enabled</span>
              </label>

              <div className="ops__form-actions">
                <button type="submit" className="ops__primary" disabled={submitting}>
                  {submitting && <Loader2 className="lucide" aria-hidden />}
                  {existing ? "Save changes" : "Activate"}
                </button>
                {existing && (
                  <button
                    type="button"
                    className="ops__danger"
                    disabled={deleting}
                    onClick={handleDelete}
                  >
                    {deleting ? (
                      <Loader2 className="lucide" aria-hidden />
                    ) : (
                      <Trash2 className="lucide" aria-hidden />
                    )}
                    Disable
                  </button>
                )}
              </div>
            </form>

            {toast && (
              <div
                className={`ops__toast ops__toast--${toast.kind}`}
                role={toast.kind === "err" ? "alert" : "status"}
              >
                {toast.kind === "err" ? (
                  <X className="lucide" aria-hidden />
                ) : (
                  <ShieldCheck className="lucide" aria-hidden />
                )}
                <span>{toast.message}</span>
              </div>
            )}
          </section>
        )}
        <OAuthClientsPanel />
      </main>
    </div>
  );
}

type ClientFormState = {
  displayName: string;
  redirectUris: string;
  public: boolean;
};

function OAuthClientsPanel() {
  const [data, setData] = useState<OAuthClientsResp | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [errCode, setErrCode] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [newSecret, setNewSecret] = useState<string | null>(null);
  const [toast, setToast] = useState<Toast | null>(null);
  const [form, setForm] = useState<ClientFormState>({
    displayName: "",
    redirectUris: "http://127.0.0.1:3846/callback",
    public: true,
  });

  const load = useCallback(async () => {
    setLoading(true);
    setErr(null);
    setErrCode(null);
    try {
      setData(await api.oauthClients());
    } catch (e) {
      const tagged = e as Error & { error_code?: string };
      setErr(tagged.message);
      setErrCode(tagged.error_code ?? null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (submitting) return;
    setSubmitting(true);
    setToast(null);
    setNewSecret(null);
    try {
      const redirectUris = form.redirectUris
        .split(/\r?\n|,/)
        .map((item) => item.trim())
        .filter(Boolean);
      const resp = await api.createOAuthClient({
        display_name: form.displayName.trim(),
        redirect_uris: redirectUris,
        public: form.public,
      });
      setData((d) => ({ clients: [...(d?.clients ?? []), resp.client] }));
      setNewSecret(resp.client_secret ?? null);
      setToast({ kind: "ok", message: "OAuth client registered." });
      setForm((f) => ({ ...f, displayName: "" }));
    } catch (e) {
      const tagged = e as Error & { error_code?: string };
      setToast({
        kind: "err",
        message: tagged.error_code ? `${tagged.error_code}: ${tagged.message}` : tagged.message,
      });
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(client: OAuthClient) {
    if (deleting) return;
    if (!window.confirm(`Delete OAuth client ${client.client_id}?`)) {
      return;
    }
    const suppress = client.created_via === "env_seed"
      ? window.confirm("Suppress boot-time reseed for this env-seeded client?")
      : false;
    setDeleting(client.client_id);
    setToast(null);
    try {
      await api.deleteOAuthClient(client.client_id, { suppressEnvSeed: suppress });
      setData((d) => ({
        clients: (d?.clients ?? []).filter((item) => item.client_id !== client.client_id),
      }));
      setToast({ kind: "ok", message: "OAuth client deleted." });
    } catch (e) {
      const tagged = e as Error & { error_code?: string };
      setToast({
        kind: "err",
        message: tagged.error_code ? `${tagged.error_code}: ${tagged.message}` : tagged.message,
      });
    } finally {
      setDeleting(null);
    }
  }

  return (
    <section className="ops__panel">
      <header className="ops__panel-head">
        <KeyRound className="lucide" aria-hidden />
        <h2>OAuth clients</h2>
      </header>

      <p className="ops__hint">
        MCP clients can register dynamically via <code>/oauth/register</code>.
        Operators can also create a client here; confidential client secrets are
        shown once and stored only as hashes.
      </p>

      {loading && (
        <div className="ops__loading">
          <Loader2 className="lucide" aria-hidden /> Loading...
        </div>
      )}

      {err && (
        <div className="ops__toast ops__toast--err" role="alert">
          <X className="lucide" aria-hidden />
          <span>{errCode ? `${errCode}: ${err}` : err}</span>
        </div>
      )}

      {!err && (
        <>
          <div className="ops__list">
            {(data?.clients ?? []).map((client) => (
              <div key={client.client_id} className="ops__row">
                <div>
                  <strong>{client.display_name || client.client_id}</strong>
                  <p>
                    <code>{client.client_id}</code> · {client.public ? "public" : "confidential"} · {client.created_via}
                  </p>
                </div>
                <button
                  type="button"
                  className="ops__danger"
                  disabled={deleting === client.client_id}
                  onClick={() => void handleDelete(client)}
                >
                  {deleting === client.client_id ? (
                    <Loader2 className="lucide" aria-hidden />
                  ) : (
                    <Trash2 className="lucide" aria-hidden />
                  )}
                  Delete
                </button>
              </div>
            ))}
            {!loading && (data?.clients ?? []).length === 0 && (
              <p className="ops__hint">No OAuth clients are registered.</p>
            )}
          </div>

          <form onSubmit={handleCreate} className="ops__form">
            <label className="ops__field">
              <span>Display name</span>
              <input
                type="text"
                value={form.displayName}
                onChange={(e) => setForm((f) => ({ ...f, displayName: e.target.value }))}
                placeholder="Cursor"
                required
              />
            </label>
            <label className="ops__field">
              <span>Redirect URIs</span>
              <textarea
                value={form.redirectUris}
                onChange={(e) => setForm((f) => ({ ...f, redirectUris: e.target.value }))}
                rows={3}
                required
              />
            </label>
            <label className="ops__field ops__field--inline">
              <input
                type="checkbox"
                checked={form.public}
                onChange={(e) => setForm((f) => ({ ...f, public: e.target.checked }))}
              />
              <span>Public client</span>
            </label>
            <div className="ops__form-actions">
              <button type="submit" className="ops__primary" disabled={submitting}>
                {submitting ? <Loader2 className="lucide" aria-hidden /> : <Plus className="lucide" aria-hidden />}
                Register client
              </button>
            </div>
          </form>

          {newSecret && (
            <div className="ops__toast ops__toast--ok" role="status">
              <Check className="lucide" aria-hidden />
              <span>Client secret: <code>{newSecret}</code></span>
            </div>
          )}

          {toast && (
            <div
              className={`ops__toast ops__toast--${toast.kind}`}
              role={toast.kind === "err" ? "alert" : "status"}
            >
              {toast.kind === "err" ? (
                <X className="lucide" aria-hidden />
              ) : (
                <ShieldCheck className="lucide" aria-hidden />
              )}
              <span>{toast.message}</span>
            </div>
          )}
        </>
      )}
    </section>
  );
}
