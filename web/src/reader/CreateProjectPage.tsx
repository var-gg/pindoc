// CreateProjectPage is the Reader's GUI bootstrap entrypoint (Decision
// project-bootstrap-canonical-flow-reader-ui-first-class, Task
// t3-reader-new-project-page-and-mcp-snippet). Hits POST /api/projects
// (same backend function the MCP tool and pindoc-admin CLI use), then
// shows MCP connection copy targets so the user can finish bootstrap
// by pasting either the URL, the `.mcp.json` block, or the agent-ready
// prompt into their workspace.
//
// The page is intentionally standalone — no Reader sidebar/topnav — so a
// fresh install can land directly here from the onboarding wizard (T4)
// without depending on the Reader's project-scoped chrome being usable
// yet.
import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Check, Copy, Loader2 } from "lucide-react";
import { api, type CreateProjectResp } from "../api/client";
import { useI18n } from "../i18n";
import {
  fieldForProjectCreateError,
  isProjectCreateSubmitDisabled,
  PROJECT_SLUG_HTML_PATTERN,
  projectCreateErrorMessage,
  validateProjectSlugInput,
} from "./projectSlugPolicy";
import "../styles/reader.css";

type Lang = "en" | "ko" | "ja";
type CopyTarget = "url" | "mcp_json" | "agent_prompt";

export function CreateProjectPage() {
  const { t, lang } = useI18n();
  const initialLang: Lang = lang === "ko" ? "ko" : "en";
  const [searchParams] = useSearchParams();
  // welcome=1 marks the onboarding wizard entry — LegacyRedirect sends
  // fresh installs here, and the page renders a friendlier header so
  // the user knows they're at "step 1 of 3 — pick a project name". A
  // direct visitor without ?welcome=1 sees the bare form.
  const isWelcome = searchParams.get("welcome") === "1";

  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [primaryLanguage, setPrimaryLanguage] = useState<Lang>(initialLang);
  const [description, setDescription] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const [created, setCreated] = useState<CreateProjectResp | null>(null);
  const [copied, setCopied] = useState<CopyTarget | null>(null);
  const slugRef = useRef<HTMLInputElement | null>(null);
  const nameRef = useRef<HTMLInputElement | null>(null);
  const languageRef = useRef<HTMLInputElement | null>(null);

  // Reset the "copied" flash when the user navigates back to the form.
  useEffect(() => {
    if (!created) setCopied(null);
  }, [created]);

  useEffect(() => {
    const field = fieldForProjectCreateError(errorCode);
    if (!field) return;
    const target =
      field === "slug"
        ? slugRef.current
        : field === "name"
          ? nameRef.current
          : languageRef.current;
    target?.focus();
  }, [errorCode]);

  const serverErrorField = fieldForProjectCreateError(errorCode);
  const serverErrorMessage = errorCode ? projectCreateErrorMessage(t, errorCode) : null;
  const slugClientErrorCode = validateProjectSlugInput(slug);
  const slugError =
    slugClientErrorCode
      ? projectCreateErrorMessage(t, slugClientErrorCode)
      : serverErrorField === "slug"
        ? serverErrorMessage
        : null;
  const nameError = serverErrorField === "name" ? serverErrorMessage : null;
  const languageError = serverErrorField === "language" ? serverErrorMessage : null;
  const formError = errorCode && !serverErrorField ? serverErrorMessage : null;
  const submitDisabled = isProjectCreateSubmitDisabled({
    slug,
    name,
    primaryLanguage,
    submitting,
  });

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (submitDisabled) return;
    setErrorCode(null);
    setSubmitting(true);
    try {
      const out = await api.createProject({
        slug: slug.trim(),
        name: name.trim(),
        primary_language: primaryLanguage,
        description: description.trim() || undefined,
      });
      setCreated(out);
    } catch (e) {
      const err = e as Error & { error_code?: string };
      setErrorCode(err.error_code ?? "UNKNOWN");
    } finally {
      setSubmitting(false);
    }
  }

  function reset() {
    setSlug("");
    setName("");
    setDescription("");
    setErrorCode(null);
    setCreated(null);
  }

  if (created) {
    return (
      <CreateProjectSuccess
        result={created}
        copied={copied}
        onCopy={async (value, target) => {
          await navigator.clipboard.writeText(value);
          setCopied(target);
          setTimeout(() => setCopied((c) => (c === target ? null : c)), 2000);
        }}
        onCreateAnother={reset}
        t={t}
      />
    );
  }

  return (
    <div className="cp-page">
      {isWelcome && (
        <div className="cp-welcome">
          <p className="cp-welcome__step">{t("new_project.welcome.step")}</p>
          <h2 className="cp-welcome__title">{t("new_project.welcome.title")}</h2>
          <p className="cp-welcome__sub">{t("new_project.welcome.subtitle")}</p>
        </div>
      )}
      <header className="cp-header">
        <h1>{t("new_project.title")}</h1>
        <p className="cp-subtitle">{t("new_project.subtitle")}</p>
      </header>

      <form className="cp-form" onSubmit={handleSubmit}>
        <label className="cp-field">
          <span className="cp-field__label">{t("new_project.field.slug")}</span>
          <input
            ref={slugRef}
            type="text"
            className="cp-field__input"
            value={slug}
            onChange={(e) => {
              setSlug(e.target.value);
              setErrorCode(null);
            }}
            placeholder={t("new_project.field.slug.placeholder")}
            pattern={PROJECT_SLUG_HTML_PATTERN}
            minLength={2}
            maxLength={40}
            aria-invalid={slugError ? true : undefined}
            aria-describedby={slugError ? "new-project-slug-error" : "new-project-slug-hint"}
            autoFocus
            required
          />
          <span id="new-project-slug-hint" className="cp-field__hint">
            {t("new_project.field.slug.hint")}
          </span>
          {slugError && (
            <span id="new-project-slug-error" className="cp-field__error">
              {slugError}
            </span>
          )}
        </label>

        <label className="cp-field">
          <span className="cp-field__label">{t("new_project.field.name")}</span>
          <input
            ref={nameRef}
            type="text"
            className="cp-field__input"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              setErrorCode(null);
            }}
            placeholder={t("new_project.field.name.placeholder")}
            aria-invalid={nameError ? true : undefined}
            aria-describedby={nameError ? "new-project-name-error" : undefined}
            required
          />
          {nameError && (
            <span id="new-project-name-error" className="cp-field__error">
              {nameError}
            </span>
          )}
        </label>

        <fieldset
          className="cp-field"
          aria-invalid={languageError ? true : undefined}
          aria-describedby={languageError ? "new-project-language-error new-project-language-hint" : "new-project-language-hint"}
        >
          <legend className="cp-field__label">
            {t("new_project.field.language")}
          </legend>
          <div className="cp-radio-group" aria-invalid={languageError ? true : undefined}>
            {(["en", "ko", "ja"] as const).map((l) => (
              <label key={l} className="cp-radio">
                <input
                  ref={l === "en" ? languageRef : undefined}
                  type="radio"
                  name="primary_language"
                  value={l}
                  checked={primaryLanguage === l}
                  onChange={() => {
                    setPrimaryLanguage(l);
                    setErrorCode(null);
                  }}
                />
                <span>{l.toUpperCase()}</span>
              </label>
            ))}
          </div>
          <span id="new-project-language-hint" className="cp-field__hint">
            {t("new_project.field.language.hint")}
          </span>
          {languageError && (
            <span id="new-project-language-error" className="cp-field__error">
              {languageError}
            </span>
          )}
        </fieldset>

        <label className="cp-field">
          <span className="cp-field__label">
            {t("new_project.field.description")}
            <span className="cp-field__optional">
              {" "}
              · {t("new_project.field.optional")}
            </span>
          </span>
          <input
            type="text"
            className="cp-field__input"
            value={description}
            onChange={(e) => {
              setDescription(e.target.value);
              setErrorCode(null);
            }}
            placeholder={t("new_project.field.description.placeholder")}
            maxLength={200}
          />
        </label>

        {formError && (
          <div role="alert" className="cp-error">
            {formError}
          </div>
        )}

        <div className="cp-actions">
          <button type="submit" className="cp-submit" disabled={submitDisabled}>
            {submitting && (
              <Loader2 className="lucide cp-spinner" aria-hidden />
            )}
            {submitting
              ? t("new_project.submitting")
              : t("new_project.submit")}
          </button>
        </div>
      </form>
    </div>
  );
}

export function CreateProjectSuccess({
  result,
  copied,
  onCopy,
  onCreateAnother,
  t,
}: {
  result: CreateProjectResp;
  copied: CopyTarget | null;
  onCopy: (value: string, target: CopyTarget) => void | Promise<void>;
  onCreateAnother: () => void;
  t: (k: string, ...args: Array<string | number>) => string;
}) {
  return (
    <div className="cp-page cp-page--success">
      <header className="cp-header">
        <p className="cp-welcome__step">{t("new_project.success.step")}</p>
        <h1 className="cp-success-title">
          <Check className="lucide" aria-hidden /> {t("new_project.success.title", result.slug)}
        </h1>
        <p className="cp-subtitle">
          {t(
            "new_project.success.subtitle",
            result.areas_created,
            result.templates_created,
          )}
        </p>
      </header>

      <section className="cp-snippet">
        <header className="cp-snippet__intro">
          <h2>{t("new_project.snippet.title")}</h2>
          <p>{t("new_project.snippet.intro")}</p>
        </header>
        <CopyBlock
          title={t("new_project.snippet.copy.url.title")}
          subtitle={t("new_project.snippet.copy.url.subtitle")}
          value={result.mcp_connect.url}
          monospace
          copied={copied === "url"}
          onCopy={() => onCopy(result.mcp_connect.url, "url")}
          copyLabel={t("new_project.snippet.copy")}
          copiedLabel={t("new_project.snippet.copied")}
        />
        <CopyBlock
          title={t("new_project.snippet.copy.mcp_json.title")}
          subtitle={t("new_project.snippet.copy.mcp_json.subtitle")}
          value={result.mcp_connect.mcp_json}
          monospace
          multiline
          copied={copied === "mcp_json"}
          onCopy={() => onCopy(result.mcp_connect.mcp_json, "mcp_json")}
          copyLabel={t("new_project.snippet.copy")}
          copiedLabel={t("new_project.snippet.copied")}
        />
        <CopyBlock
          title={t("new_project.snippet.copy.agent_prompt.title")}
          subtitle={t("new_project.snippet.copy.agent_prompt.subtitle")}
          value={result.mcp_connect.agent_prompt}
          multiline
          copied={copied === "agent_prompt"}
          onCopy={() => onCopy(result.mcp_connect.agent_prompt, "agent_prompt")}
          copyLabel={t("new_project.snippet.copy")}
          copiedLabel={t("new_project.snippet.copied")}
        />
        <p className="cp-snippet__harness">{t("new_project.harness_hint")}</p>
      </section>

      <div className="cp-actions">
        <Link className="cp-link-primary" to={result.url}>
          {t("new_project.open_project")}
        </Link>
        <button
          type="button"
          className="cp-link-secondary"
          onClick={onCreateAnother}
        >
          {t("new_project.create_another")}
        </button>
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
