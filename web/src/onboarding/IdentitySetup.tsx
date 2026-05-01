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

import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import { Check, Copy, Loader2, ShieldCheck } from "lucide-react";
import { api, type OnboardingIdentityResp } from "../api/client";
import "../styles/reader.css";

type CopyTarget = "url" | "mcp_json" | "agent_prompt";

export function IdentitySetup() {
  const navigate = useNavigate();
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [githubHandle, setGithubHandle] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [created, setCreated] = useState<OnboardingIdentityResp | null>(null);

  // Pre-fill from /api/config when the daemon already wrote a default
  // (e.g. operator set PINDOC_USER_NAME). Skips if it errors — the
  // form still works without the prefill.
  useEffect(() => {
    (async () => {
      try {
        const r = await fetch("/api/users", { headers: { Accept: "application/json" } });
        if (!r.ok) return;
        // intentionally a no-op: keep the form blank on a truly fresh
        // install. We don't want to silently pick "OAuth Test ..."
        // residue as a default name.
      } catch {
        // ignore
      }
    })();
  }, []);

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
      />
    );
  }

  return (
    <div className="cp-page">
      <div className="cp-welcome">
        <p className="cp-welcome__step">step 1 — owner identity</p>
        <h2 className="cp-welcome__title">Welcome to Pindoc</h2>
        <p className="cp-welcome__sub">
          Pindoc is loopback-only by default. Tell us who's the owner
          on this box so every artifact attribution and project
          ownership row anchors to a stable identity. No env edits or
          GitHub OAuth needed for this first step — just a name and
          email you'll recognise later.
        </p>
      </div>
      <header className="cp-header">
        <h1>Set your identity</h1>
        <p className="cp-subtitle">
          The first identity created on a fresh instance becomes the
          owner. Subsequent collaborators join through GitHub OAuth +
          invite once you flip the daemon to external bind.
        </p>
      </header>

      <form className="cp-form" onSubmit={handleSubmit} noValidate>
        <label className="cp-field">
          <span className="cp-field__label">Display name</span>
          <input
            type="text"
            className="cp-field__input"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder="Ada Lovelace"
            autoComplete="off"
            spellCheck={false}
            required
          />
          <span className="cp-field__hint">
            Shown next to revisions and on the Members panel.
          </span>
        </label>
        <label className="cp-field">
          <span className="cp-field__label">Email</span>
          <input
            type="email"
            className="cp-field__input"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
            autoComplete="off"
            spellCheck={false}
            required
          />
          <span className="cp-field__hint">
            Used as the canonical anchor — if you later activate GitHub
            OAuth, the same email links to this row.
          </span>
        </label>
        <label className="cp-field">
          <span className="cp-field__label">GitHub handle (optional)</span>
          <input
            type="text"
            className="cp-field__input"
            value={githubHandle}
            onChange={(e) => setGithubHandle(e.target.value)}
            placeholder="octocat"
            autoComplete="off"
            spellCheck={false}
          />
          <span className="cp-field__hint">
            Pre-fills the @-handle column. You can leave it blank now
            and add it later via the Members panel.
          </span>
        </label>

        {errorCode && (
          <div className="cp-error" role="alert">
            <strong>{errorCode}</strong>
            {errorMsg && <span> · {errorMsg}</span>}
          </div>
        )}

        <button
          type="submit"
          className="cp-link-primary"
          disabled={submitting}
        >
          {submitting && <Loader2 className="lucide" aria-hidden />}
          {submitting ? "Setting up…" : "Create identity"}
        </button>
      </form>

      <p className="cp-snippet__harness">
        Already have a pindoc-admin or env-set identity? The daemon
        skips this page automatically once <code>server_settings.
        default_loopback_user_id</code> is bound.
      </p>
    </div>
  );
}

function IdentitySuccess({
  result,
  onContinue,
}: {
  result: OnboardingIdentityResp;
  onContinue: () => void;
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
          <ShieldCheck className="lucide" aria-hidden /> Identity set
        </h1>
        <p className="cp-subtitle">
          Welcome, <strong>{result.display_name}</strong>. You own the{" "}
          <code>{result.project.slug}</code> project. Next step: connect
          your AI agent to Pindoc.
        </p>
      </header>

      <section className="cp-snippet">
        <header className="cp-snippet__intro">
          <h2>Connect Claude Code / Codex / Cursor</h2>
          <p>
            Three flavours — pick whichever matches how your agent
            consumes config. The MCP URL goes loopback-Trust by default,
            so no token or sign-in is needed yet.
          </p>
        </header>

        <CopyBlock
          title="MCP URL only"
          subtitle="Paste into a single-field MCP host config."
          value={result.mcp_connect.url}
          monospace
          copied={copied === "url"}
          onCopy={() => copy(result.mcp_connect.url, "url")}
        />

        <CopyBlock
          title=".mcp.json snippet"
          subtitle="Paste into ~/.config/claude-code/mcp.json (or the equivalent for your client)."
          value={result.mcp_connect.mcp_json}
          monospace
          multiline
          copied={copied === "mcp_json"}
          onCopy={() => copy(result.mcp_connect.mcp_json, "mcp_json")}
        />

        <CopyBlock
          title="Agent prompt"
          subtitle="Paste into your agent chat. The agent edits the config + verifies pindoc.ping for you."
          value={result.mcp_connect.agent_prompt}
          multiline
          copied={copied === "agent_prompt"}
          onCopy={() => copy(result.mcp_connect.agent_prompt, "agent_prompt")}
        />
      </section>

      <div className="cp-actions">
        <button type="button" className="cp-link-primary" onClick={onContinue}>
          Continue to {result.project.slug}
        </button>
        <Link className="cp-link-secondary" to="/admin/providers">
          Activate GitHub OAuth (later)
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
}: {
  title: string;
  subtitle: string;
  value: string;
  monospace?: boolean;
  multiline?: boolean;
  copied: boolean;
  onCopy: () => void;
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
          aria-label={`Copy ${title}`}
        >
          {copied ? <Check className="lucide" /> : <Copy className="lucide" />}
          {copied ? "Copied" : "Copy"}
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
