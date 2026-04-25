// CreateProjectPage is the Reader's GUI bootstrap entrypoint (Decision
// project-bootstrap-canonical-flow-reader-ui-first-class, Task
// t3-reader-new-project-page-and-mcp-snippet). Hits POST /api/projects
// (same backend function the MCP tool and pindoc-admin CLI use), then
// shows a copy-paste-ready .mcp.json snippet so the user finishes
// bootstrap by pasting into their workspace's `.mcp.json` and opening
// Claude Code there.
//
// The page is intentionally standalone — no Reader sidebar/topnav — so a
// fresh install can land directly here from the onboarding wizard (T4)
// without depending on the Reader's project-scoped chrome being usable
// yet.
import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Check, Copy, Loader2 } from "lucide-react";
import { api, type CreateProjectResp } from "../api/client";
import { useI18n } from "../i18n";
import "../styles/reader.css";

type Lang = "en" | "ko" | "ja";

// daemonBaseFallback is the default streamable-HTTP MCP daemon URL the
// snippet suggests when /api/config doesn't surface one. Matches the
// recommendation in README's "데몬 모드" section. Users on a custom port
// edit the host:port portion of the snippet manually for now — V1.5
// adds a server_settings.mcp_daemon_base_url override so the UI fills
// it in automatically.
const DAEMON_BASE_FALLBACK = "http://127.0.0.1:5830";

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
  const [copied, setCopied] = useState(false);

  // Reset the "copied" flash when the user navigates back to the form.
  useEffect(() => {
    if (!created) setCopied(false);
  }, [created]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (submitting) return;
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
        onCopy={async () => {
          await navigator.clipboard.writeText(buildSnippet(created.slug));
          setCopied(true);
          setTimeout(() => setCopied(false), 2000);
        }}
        onCreateAnother={reset}
        t={t}
      />
    );
  }

  const errorKey = errorCode ? `new_project.error.${errorCode}` : null;

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

      <form className="cp-form" onSubmit={handleSubmit} noValidate>
        <label className="cp-field">
          <span className="cp-field__label">{t("new_project.field.slug")}</span>
          <input
            type="text"
            className="cp-field__input"
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
            placeholder={t("new_project.field.slug.placeholder")}
            autoFocus
            required
          />
          <span className="cp-field__hint">{t("new_project.field.slug.hint")}</span>
        </label>

        <label className="cp-field">
          <span className="cp-field__label">{t("new_project.field.name")}</span>
          <input
            type="text"
            className="cp-field__input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("new_project.field.name.placeholder")}
            required
          />
        </label>

        <fieldset className="cp-field">
          <legend className="cp-field__label">
            {t("new_project.field.language")}
          </legend>
          <div className="cp-radio-group">
            {(["en", "ko", "ja"] as const).map((l) => (
              <label key={l} className="cp-radio">
                <input
                  type="radio"
                  name="primary_language"
                  value={l}
                  checked={primaryLanguage === l}
                  onChange={() => setPrimaryLanguage(l)}
                />
                <span>{l.toUpperCase()}</span>
              </label>
            ))}
          </div>
          <span className="cp-field__hint">
            {t("new_project.field.language.hint")}
          </span>
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
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t("new_project.field.description.placeholder")}
            maxLength={200}
          />
        </label>

        {errorKey && (
          <div role="alert" className="cp-error">
            <strong>[{errorCode}]</strong> {t(errorKey)}
          </div>
        )}

        <div className="cp-actions">
          <button type="submit" className="cp-submit" disabled={submitting}>
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

function CreateProjectSuccess({
  result,
  copied,
  onCopy,
  onCreateAnother,
  t,
}: {
  result: CreateProjectResp;
  copied: boolean;
  onCopy: () => void;
  onCreateAnother: () => void;
  t: (k: string, ...args: Array<string | number>) => string;
}) {
  const snippet = buildSnippet(result.slug);
  return (
    <div className="cp-page cp-page--success">
      <header className="cp-header">
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
        <p className="cp-snippet__intro">{t("new_project.snippet.intro")}</p>
        <div className="cp-snippet__box">
          <pre className="cp-snippet__pre">{snippet}</pre>
          <button
            type="button"
            className="cp-snippet__copy"
            onClick={onCopy}
            aria-label={t("new_project.snippet.copy")}
          >
            {copied ? <Check className="lucide" /> : <Copy className="lucide" />}
            {copied
              ? t("new_project.snippet.copied")
              : t("new_project.snippet.copy")}
          </button>
        </div>
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

// buildSnippet keeps the JSON formatting consistent with the README
// example so a user comparing docs and the form sees the same shape.
function buildSnippet(slug: string): string {
  return [
    "{",
    `  "mcpServers": {`,
    `    "pindoc": {`,
    `      "url": "${DAEMON_BASE_FALLBACK}/mcp/p/${slug}"`,
    `    }`,
    `  }`,
    `}`,
  ].join("\n");
}
