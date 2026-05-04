// IdentitySetup — first-time identity flow (agent-era onboarding).
//
// Decision agent-only-write-분할 + Decision decision-auth-model-loopback-
// and-providers § 2: a fresh install must not require env edits to
// attribute work. This page is the loopback-only form a Reader user
// fills exactly once on a clean instance; the BE binds the resulting
// users.id to server_settings.default_loopback_user_id atomically.
//
// On success the page flips to a setup card with three copy targets
// the operator pastes into Claude Code / Codex / Cursor:
//   1. just the MCP URL
//   2. the full `.mcp.json` block
//   3. an agent-ready prompt that asks the agent to register + verify
//
// Standalone layout (no Reader chrome) so it works before the
// project-scoped UI is reachable.

import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { Check, Copy, Loader2, ShieldCheck } from "lucide-react";
import { api, type OnboardingIdentityResp } from "../api/client";
import { notifyFirstRunConfigChanged } from "../firstRunConfig";
import { useI18n } from "../i18n";
import "../styles/reader.css";

type CopyTarget = "url" | "mcp_json" | "agent_prompt";

export function IdentitySetup() {
  const navigate = useNavigate();
  const { t } = useI18n();
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [githubHandle, setGithubHandle] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [created, setCreated] = useState<OnboardingIdentityResp | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (submitting) return;
    setErrorCode(null);
    setErrorMsg(null);
    setSubmitting(true);
    try {
      const out = await api.setupIdentity({
        display_name: displayName.trim(),
        email: email.trim(),
        github_handle: githubHandle.trim() || undefined,
      });
      notifyFirstRunConfigChanged({ identity_required: false, onboarding_required: false });
      setCreated(out);
    } catch (e) {
      const err = e as Error & { error_code?: string };
      setErrorCode(err.error_code ?? "UNKNOWN");
      setErrorMsg(err.message);
    } finally {
      setSubmitting(false);
    }
  }

  if (created) {
    return (
      <IdentitySuccess
        result={created}
        onContinue={() => navigate(created.project.url, { replace: true })}
        t={t}
      />
    );
  }

  return (
    <div className="cp-page">
      <div className="cp-welcome">
        <p className="cp-welcome__step">{t("onboarding.identity.welcome.step")}</p>
        <h2 className="cp-welcome__title">{t("onboarding.identity.welcome.title")}</h2>
        <p className="cp-welcome__sub">{t("onboarding.identity.welcome.subtitle")}</p>
      </div>
      <header className="cp-header">
        <h1>{t("onboarding.identity.title")}</h1>
        <p className="cp-subtitle">{t("onboarding.identity.subtitle")}</p>
      </header>

      <form className="cp-form" onSubmit={handleSubmit} noValidate>
        <label className="cp-field">
          <span className="cp-field__label">{t("onboarding.identity.field.display_name.label")}</span>
          <input
            type="text"
            className="cp-field__input"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder={t("onboarding.identity.field.display_name.placeholder")}
            autoComplete="off"
            spellCheck={false}
            required
          />
          <span className="cp-field__hint">{t("onboarding.identity.field.display_name.hint")}</span>
        </label>
        <label className="cp-field">
          <span className="cp-field__label">{t("onboarding.identity.field.email.label")}</span>
          <input
            type="email"
            className="cp-field__input"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder={t("onboarding.identity.field.email.placeholder")}
            autoComplete="off"
            spellCheck={false}
            required
          />
          <span className="cp-field__hint">{t("onboarding.identity.field.email.hint")}</span>
        </label>
        <label className="cp-field">
          <span className="cp-field__label">{t("onboarding.identity.field.github_handle.label")}</span>
          <input
            type="text"
            className="cp-field__input"
            value={githubHandle}
            onChange={(e) => setGithubHandle(e.target.value)}
            placeholder={t("onboarding.identity.field.github_handle.placeholder")}
            autoComplete="off"
            spellCheck={false}
          />
          <span className="cp-field__hint">{t("onboarding.identity.field.github_handle.hint")}</span>
        </label>

        {errorCode && (
          <div className="cp-error" role="alert">
            {identityErrorMessage(t, errorCode, errorMsg)}
          </div>
        )}

        <button
          type="submit"
          className="cp-link-primary"
          disabled={submitting}
        >
          {submitting && <Loader2 className="lucide" aria-hidden />}
          {submitting ? t("onboarding.identity.submitting") : t("onboarding.identity.submit")}
        </button>
      </form>

      <p className="cp-snippet__harness">{t("onboarding.identity.harness_hint")}</p>
    </div>
  );
}

export function IdentitySuccess({
  result,
  onContinue,
  t,
}: {
  result: OnboardingIdentityResp;
  onContinue: () => void;
  t: (k: string, ...args: Array<string | number>) => string;
}) {
  const [copied, setCopied] = useState<CopyTarget | null>(null);

  async function copy(value: string, target: CopyTarget) {
    await navigator.clipboard.writeText(value);
    setCopied(target);
    window.setTimeout(() => setCopied((c) => (c === target ? null : c)), 2000);
  }

  return (
    <div className="cp-page cp-page--success">
      <header className="cp-header">
        <h1 className="cp-success-title">
          <ShieldCheck className="lucide" aria-hidden /> {t("onboarding.identity.success.title")}
        </h1>
        <p className="cp-subtitle">
          {t("onboarding.identity.success.subtitle.prefix")}{" "}
          <strong>{result.display_name}</strong>.{" "}
          {t("onboarding.identity.success.subtitle.project_prefix")}{" "}
          <code>{result.project.slug}</code>{" "}
          {t("onboarding.identity.success.subtitle.project_suffix")}
        </p>
      </header>

      <section className="cp-snippet">
        <header className="cp-snippet__intro">
          <h2>{t("onboarding.identity.success.connect_title")}</h2>
          <p>{t("onboarding.identity.success.connect_body")}</p>
        </header>

        <CopyBlock
          title={t("onboarding.identity.success.copy.url.title")}
          subtitle={t("onboarding.identity.success.copy.url.subtitle")}
          value={result.mcp_connect.url}
          monospace
          copied={copied === "url"}
          onCopy={() => copy(result.mcp_connect.url, "url")}
          copyLabel={t("onboarding.identity.copy")}
          copiedLabel={t("onboarding.identity.copied")}
        />

        <CopyBlock
          title={t("onboarding.identity.success.copy.mcp_json.title")}
          subtitle={t("onboarding.identity.success.copy.mcp_json.subtitle")}
          value={result.mcp_connect.mcp_json}
          monospace
          multiline
          copied={copied === "mcp_json"}
          onCopy={() => copy(result.mcp_connect.mcp_json, "mcp_json")}
          copyLabel={t("onboarding.identity.copy")}
          copiedLabel={t("onboarding.identity.copied")}
        />

        <CopyBlock
          title={t("onboarding.identity.success.copy.agent_prompt.title")}
          subtitle={t("onboarding.identity.success.copy.agent_prompt.subtitle")}
          value={result.mcp_connect.agent_prompt}
          multiline
          copied={copied === "agent_prompt"}
          onCopy={() => copy(result.mcp_connect.agent_prompt, "agent_prompt")}
          copyLabel={t("onboarding.identity.copy")}
          copiedLabel={t("onboarding.identity.copied")}
        />
      </section>

      <div className="cp-actions">
        <button type="button" className="cp-link-primary" onClick={onContinue}>
          {t("onboarding.identity.success.continue", result.project.slug)}
        </button>
        <Link className="cp-link-secondary" to="/admin/providers">
          {t("onboarding.identity.success.activate_oauth")}
        </Link>
      </div>
    </div>
  );
}

function CopyBlock({
  title,
  subtitle,
  value,
  monospace,
  multiline,
  copied,
  onCopy,
  copyLabel,
  copiedLabel,
}: {
  title: string;
  subtitle: string;
  value: string;
  monospace?: boolean;
  multiline?: boolean;
  copied: boolean;
  onCopy: () => void;
  copyLabel: string;
  copiedLabel: string;
}) {
  return (
    <div className="cp-snippet__box" style={{ marginBottom: "var(--space-4, 16px)" }}>
      <header style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: 12 }}>
        <div>
          <strong>{title}</strong>
          <p className="cp-subtitle" style={{ margin: "4px 0 0" }}>{subtitle}</p>
        </div>
        <button
          type="button"
          className="cp-snippet__copy"
          onClick={onCopy}
          aria-label={`${copyLabel} ${title}`}
        >
          {copied ? <Check className="lucide" /> : <Copy className="lucide" />}
          {copied ? copiedLabel : copyLabel}
        </button>
      </header>
      {multiline ? (
        <pre
          className="cp-snippet__pre"
          style={{ marginTop: 8, fontFamily: monospace ? "var(--font-mono, monospace)" : "inherit" }}
        >
          {value}
        </pre>
      ) : (
        <code className="cp-snippet__pre" style={{ marginTop: 8, display: "block", padding: "8px 12px" }}>
          {value}
        </code>
      )}
    </div>
  );
}

function identityErrorMessage(
  t: (k: string, ...args: Array<string | number>) => string,
  errorCode: string,
  fallback: string | null,
): string {
  const key = `onboarding.identity.error.${errorCode}`;
  const translated = t(key);
  if (translated !== key) return translated;
  return fallback || t("onboarding.identity.error.UNKNOWN");
}
