import { useMemo, useState } from "react";
import { Check, Copy } from "lucide-react";
import { Link, useLocation } from "react-router";
import { useI18n } from "../i18n";
import "../styles/reader.css";

export function SignupCompletePage() {
  const { t } = useI18n();
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const project = (params.get("project") ?? "").trim();
  const [copied, setCopied] = useState(false);
  const snippet = useMemo(() => buildSnippet(), []);

  async function copySnippet() {
    await navigator.clipboard.writeText(snippet);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  return (
    <main className="signup-page">
      <section className="signup-panel signup-panel--wide" aria-labelledby="signup-complete-title">
        <div className="signup-panel__brand">
          <img src="/design-system/assets/logo.svg" alt="" width={24} height={24} />
          <span>Pindoc</span>
        </div>
        <h1 id="signup-complete-title">{t("signup_complete.title")}</h1>
        {project && (
          <dl className="signup-panel__meta">
            <div>
              <dt>{t("signup_complete.project")}</dt>
              <dd>{project}</dd>
            </div>
          </dl>
        )}

        <section className="cp-snippet">
          <p className="cp-snippet__intro">{t("signup_complete.snippet.intro")}</p>
          <div className="cp-snippet__box">
            <pre className="cp-snippet__pre">{snippet}</pre>
            <button
              type="button"
              className="cp-snippet__copy"
              onClick={copySnippet}
              aria-label={t("signup_complete.snippet.copy")}
            >
              {copied ? <Check className="lucide" /> : <Copy className="lucide" />}
              {copied
                ? t("signup_complete.snippet.copied")
                : t("signup_complete.snippet.copy")}
            </button>
          </div>
          <p className="cp-snippet__harness">{t("signup_complete.harness_hint")}</p>
        </section>

        <Link className="signup-panel__button" to={project ? `/p/${project}/today` : "/"}>
          {t("signup_complete.open_project")}
        </Link>
      </section>
    </main>
  );
}

function buildSnippet(): string {
  const origin =
    typeof window !== "undefined" && window.location.origin
      ? window.location.origin
      : "http://127.0.0.1:5830";
  return [
    "{",
    `  "mcpServers": {`,
    `    "pindoc": {`,
    `      "type": "http",`,
    `      "url": "${origin}/mcp"`,
    `    }`,
    `  }`,
    `}`,
  ].join("\n");
}
