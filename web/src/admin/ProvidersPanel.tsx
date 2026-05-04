import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { AlertTriangle, Check, Copy, KeyRound, Loader2, Plus, ShieldCheck, Trash2, X } from "lucide-react";
import {
  api,
  type InstanceProvider,
  type InstanceProvidersResp,
  type OAuthClient,
  type OAuthClientsResp,
} from "../api/client";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { useI18n } from "../i18n";
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
  const { t } = useI18n();
  const [data, setData] = useState<InstanceProvidersResp | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [errCode, setErrCode] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [toast, setToast] = useState<Toast | null>(null);
  const [confirmDisableOpen, setConfirmDisableOpen] = useState(false);
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
        message: existing ? t("admin.providers.toast_updated") : t("admin.providers.toast_activated"),
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
    setConfirmDisableOpen(false);
    setDeleting(true);
    setToast(null);
    try {
      await api.deleteInstanceProvider(PROVIDER_NAME);
      setData((d) => ({
        providers: (d?.providers ?? []).filter((p) => p.provider_name !== PROVIDER_NAME),
      }));
      setToast({ kind: "ok", message: t("admin.providers.toast_disabled") });
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
        <Link to="/" className="ops__back">{t("admin.providers.back")}</Link>
        <h1 className="ops__title">{t("admin.providers.title")}</h1>
      </header>

      <main className="ops__main">
        {loading && (
          <div className="ops__loading" role="status" aria-live="polite">
            <Loader2 className="lucide ops__spin" aria-hidden /> {t("admin.providers.loading")}
          </div>
        )}

        {err && errCode === "INSTANCE_OWNER_REQUIRED" && (
          <section className="ops__panel" role="alert">
            <h2>{t("admin.providers.owner_only_title")}</h2>
            <p>
              {t("admin.providers.owner_only_body")}
            </p>
          </section>
        )}

        {err && errCode === "INSTANCE_KEY_MISSING" && (
          <section className="ops__panel" role="alert">
            <h2>{t("admin.providers.instance_key_title")}</h2>
            <p>
              {t("admin.providers.instance_key_body")}
            </p>
          </section>
        )}

        {err && errCode === "PROVIDERS_UNAVAILABLE" && (
          <section className="ops__panel" role="alert">
            <h2>{t("admin.providers.store_offline_title")}</h2>
            <p>{t("admin.providers.store_offline_body")}</p>
          </section>
        )}

        {err && !errCode && (
          <section className="ops__panel" role="alert">
            <h2>{t("admin.providers.failed_to_load")}</h2>
            <p>{err}</p>
          </section>
        )}

        {!err && !loading && (
          <section className="ops__panel">
            <header className="ops__panel-head">
              <ShieldCheck className="lucide" aria-hidden />
              <h2>{t("admin.providers.github_oauth")}</h2>
              {existing && existing.enabled && (
                <span className="ops__chip ops__chip--ok">{t("admin.providers.status_enabled")}</span>
              )}
              {existing && !existing.enabled && (
                <span className="ops__chip ops__chip--warn">{t("admin.providers.status_disabled")}</span>
              )}
            </header>

            <p className="ops__hint">
              {t("admin.providers.github_oauth_hint")}
            </p>

            <form onSubmit={handleSubmit} className="ops__form">
              <label className="ops__field">
                <span>{t("admin.providers.client_id")}</span>
                <input
                  type="text"
                  name="github_oauth_client_id"
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
                  {t("admin.providers.client_secret")}
                  {existing?.has_client_secret && (
                    <em> {t("admin.providers.client_secret_keep")}</em>
                  )}
                </span>
                <input
                  type="password"
                  name="github_oauth_client_secret"
                  value={form.clientSecret}
                  onChange={(e) => setForm((f) => ({ ...f, clientSecret: e.target.value }))}
                  autoComplete="new-password"
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
                <span>{t("admin.providers.enabled_label")}</span>
              </label>

              <div className="ops__form-actions">
                <button type="submit" className="ops__primary" disabled={submitting}>
                  {submitting && <Loader2 className="lucide" aria-hidden />}
                  {existing ? t("admin.providers.save_changes") : t("admin.providers.activate")}
                </button>
                {existing && (
                  <button
                    type="button"
                    className="ops__danger"
                    disabled={deleting}
                    onClick={() => setConfirmDisableOpen(true)}
                  >
                    {deleting ? (
                      <Loader2 className="lucide" aria-hidden />
                    ) : (
                      <Trash2 className="lucide" aria-hidden />
                    )}
                    {t("admin.providers.disable")}
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
            <ConfirmDialog
              open={confirmDisableOpen}
              title={t("admin.providers.disable_confirm_title")}
              body={t("admin.providers.disable_confirm_body")}
              confirmLabel={t("admin.providers.disable_confirm_confirm")}
              cancelLabel={t("admin.common.cancel")}
              confirmBusy={deleting}
              onCancel={() => setConfirmDisableOpen(false)}
              onConfirm={() => void handleDelete()}
            />
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
  const { t } = useI18n();
  const [data, setData] = useState<OAuthClientsResp | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [errCode, setErrCode] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [dcrUpdating, setDcrUpdating] = useState(false);
  const [newSecret, setNewSecret] = useState<string | null>(null);
  const [toast, setToast] = useState<Toast | null>(null);
  const [pendingDelete, setPendingDelete] = useState<OAuthClient | null>(null);
  const [suppressEnvSeed, setSuppressEnvSeed] = useState(false);
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
      setData((d) => ({ dcr_mode: d?.dcr_mode ?? "closed", clients: [...(d?.clients ?? []), resp.client] }));
      setNewSecret(resp.client_secret ?? null);
      setToast({ kind: "ok", message: t("admin.oauth_clients.toast_registered") });
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

  async function handleDelete() {
    const client = pendingDelete;
    if (!client) return;
    if (deleting) return;
    const suppress = client.created_via === "env_seed" ? suppressEnvSeed : false;
    setPendingDelete(null);
    setDeleting(client.client_id);
    setToast(null);
    try {
      await api.deleteOAuthClient(client.client_id, { suppressEnvSeed: suppress });
      setData((d) => ({
        dcr_mode: d?.dcr_mode ?? "closed",
        clients: (d?.clients ?? []).filter((item) => item.client_id !== client.client_id),
      }));
      setToast({ kind: "ok", message: t("admin.oauth_clients.toast_deleted") });
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

  async function handleDCRMode(nextOpen: boolean) {
    if (dcrUpdating) return;
    setDcrUpdating(true);
    setToast(null);
    try {
      const resp = await api.setOAuthDCRMode(nextOpen ? "open" : "closed");
      setData((d) => ({ dcr_mode: resp.dcr_mode, clients: d?.clients ?? [] }));
      setToast({
        kind: "ok",
        message: resp.dcr_mode === "open"
          ? t("admin.oauth_clients.toast_dcr_open")
          : t("admin.oauth_clients.toast_dcr_closed"),
      });
    } catch (e) {
      const tagged = e as Error & { error_code?: string };
      setToast({
        kind: "err",
        message: tagged.error_code ? `${tagged.error_code}: ${tagged.message}` : tagged.message,
      });
    } finally {
      setDcrUpdating(false);
    }
  }

  const dcrOpen = data?.dcr_mode === "open";

  return (
    <section className="ops__panel">
      <header className="ops__panel-head">
        <KeyRound className="lucide" aria-hidden />
        <h2>{t("admin.oauth_clients.title")}</h2>
      </header>

      <p className="ops__hint">
        {t("admin.oauth_clients.hint")}
      </p>

      {loading && (
        <div className="ops__loading" role="status" aria-live="polite">
          <Loader2 className="lucide ops__spin" aria-hidden /> {t("admin.oauth_clients.loading")}
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
          <div className="ops__row">
            <div>
              <strong>{t("admin.oauth_clients.dcr_title")}</strong>
              <p>
                {t("admin.oauth_clients.dcr_body", dcrOpen ? t("admin.oauth_clients.dcr_open") : t("admin.oauth_clients.dcr_closed"))}
              </p>
            </div>
            <label className="ops__field--inline">
              <input
                type="checkbox"
                checked={dcrOpen}
                disabled={loading || dcrUpdating}
                onChange={(e) => void handleDCRMode(e.currentTarget.checked)}
              />
              <span>{dcrUpdating ? t("admin.oauth_clients.saving") : dcrOpen ? t("admin.oauth_clients.open") : t("admin.oauth_clients.closed")}</span>
            </label>
          </div>

          <div className="ops__list">
            {(data?.clients ?? []).map((client) => (
              <div key={client.client_id} className="ops__row">
                <div>
                  <strong>{client.display_name || client.client_id}</strong>
                  <p>
                    <code>{client.client_id}</code> · {client.public ? t("admin.oauth_clients.public") : t("admin.oauth_clients.confidential")} · {client.created_via}
                    {client.created_remote_addr ? <> · {client.created_remote_addr}</> : null}
                  </p>
                </div>
                <button
                  type="button"
                  className="ops__danger"
                  disabled={deleting === client.client_id}
                  onClick={() => {
                    setSuppressEnvSeed(false);
                    setPendingDelete(client);
                  }}
                >
                  {deleting === client.client_id ? (
                    <Loader2 className="lucide" aria-hidden />
                  ) : (
                    <Trash2 className="lucide" aria-hidden />
                  )}
                  {t("admin.oauth_clients.delete")}
                </button>
              </div>
            ))}
            {!loading && (data?.clients ?? []).length === 0 && (
              <p className="ops__hint">{t("admin.oauth_clients.empty")}</p>
            )}
          </div>

          <form onSubmit={handleCreate} className="ops__form">
            <label className="ops__field">
              <span>{t("admin.oauth_clients.display_name")}</span>
              <input
                type="text"
                name="oauth_client_display_name"
                value={form.displayName}
                onChange={(e) => setForm((f) => ({ ...f, displayName: e.target.value }))}
                placeholder="Cursor"
                required
              />
            </label>
            <label className="ops__field">
              <span>{t("admin.oauth_clients.redirect_uris")}</span>
              <textarea
                name="oauth_client_redirect_uris"
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
              <span>{t("admin.oauth_clients.public_client")}</span>
            </label>
            <div className="ops__form-actions">
              <button type="submit" className="ops__primary" disabled={submitting}>
                {submitting ? <Loader2 className="lucide" aria-hidden /> : <Plus className="lucide" aria-hidden />}
                {t("admin.oauth_clients.register")}
              </button>
            </div>
          </form>

          {newSecret && (
            <NewClientSecretReveal secret={newSecret} onDismiss={() => setNewSecret(null)} />
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
          <ConfirmDialog
            open={pendingDelete !== null}
            title={t("admin.oauth_clients.delete_confirm_title", pendingDelete?.client_id ?? "")}
            body={
              pendingDelete?.created_via === "env_seed" ? (
                <p>{t("admin.oauth_clients.delete_env_seed_body")}</p>
              ) : (
                <p>{t("admin.oauth_clients.delete_confirm_body")}</p>
              )
            }
            checkbox={pendingDelete?.created_via === "env_seed" ? {
              checked: suppressEnvSeed,
              label: t("admin.oauth_clients.delete_suppress_reseed"),
              onChange: setSuppressEnvSeed,
            } : undefined}
            confirmLabel={t("admin.oauth_clients.delete_confirm_confirm")}
            cancelLabel={t("admin.common.cancel")}
            confirmBusy={deleting !== null}
            onCancel={() => setPendingDelete(null)}
            onConfirm={() => void handleDelete()}
          />
        </>
      )}
    </section>
  );
}

export async function copyTextToClipboard(
  text: string,
  clipboard: Pick<Clipboard, "writeText"> | null | undefined =
    typeof navigator === "undefined" ? null : navigator.clipboard,
): Promise<boolean> {
  if (!clipboard) return false;
  await clipboard.writeText(text);
  return true;
}

export function NewClientSecretReveal({ secret, onDismiss }: { secret: string; onDismiss: () => void }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);

  async function copySecret() {
    setCopyFailed(false);
    const ok = await copyTextToClipboard(secret);
    if (!ok) {
      setCopyFailed(true);
      return;
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  }

  return (
    <aside className="ops__secret-reveal" role="alert" aria-labelledby="new-client-secret-title">
      <AlertTriangle className="lucide" aria-hidden />
      <div className="ops__secret-body">
        <h3 id="new-client-secret-title">{t("admin.oauth_clients.secret_once_title")}</h3>
        <p>{t("admin.oauth_clients.secret_once_body")}</p>
        <label className="ops__field">
          <span>{t("admin.oauth_clients.client_secret")}</span>
          <div className="ops__secret-row">
            <input
              type="text"
              value={secret}
              readOnly
              spellCheck={false}
              aria-label={t("admin.oauth_clients.client_secret")}
            />
            <button type="button" className="ops__copy" onClick={() => void copySecret()}>
              {copied ? <Check className="lucide" aria-hidden /> : <Copy className="lucide" aria-hidden />}
              {copied ? t("admin.oauth_clients.copied") : t("admin.oauth_clients.copy_secret")}
            </button>
          </div>
        </label>
        {copyFailed && (
          <p className="ops__copy-error" role="status">{t("admin.oauth_clients.copy_failed")}</p>
        )}
        <div className="ops__secret-actions">
          <button type="button" className="ops__secondary" onClick={onDismiss}>
            {t("admin.oauth_clients.secret_saved_dismiss")}
          </button>
        </div>
      </div>
    </aside>
  );
}
