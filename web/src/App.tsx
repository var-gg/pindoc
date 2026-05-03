import { useEffect, useState } from "react";
import { Link, Navigate, NavLink, Outlet, Route, Routes, useLocation, useParams } from "react-router";
import { api } from "./api/client";
import { ProvidersPanel } from "./admin/ProvidersPanel";
import { useI18n } from "./i18n";
import { IdentitySetup } from "./onboarding/IdentitySetup";
import { Telemetry } from "./ops/Telemetry";
import { CommitDetailPage } from "./git/CommitDetailPage";
import { CreateProjectPage } from "./reader/CreateProjectPage";
import { Diff } from "./reader/Diff";
import { History } from "./reader/History";
import { PindocTooltipProvider } from "./reader/Tooltip";
import { ProjectSettingsPage } from "./reader/ProjectSettingsPage";
import { ReaderShell, type ReaderView } from "./reader/ReaderShell";
import { SignupCompletePage } from "./signup/SignupCompletePage";
import { SignupPage } from "./signup/SignupPage";
import { DEFAULT_READER_ORG_SLUG, isReaderDevSurfaceEnabled, normalizeReaderSurfaceSegment, projectBaseRedirectPath, projectSurfacePath } from "./readerRoutes";
import { findSurface, previews, uiKits } from "./surfaces";

export function App() {
  return (
    <PindocTooltipProvider>
      <Routes>
        {/* Design-system scaffold. Production keeps it off normal user
          paths; append ?dev=1 to inspect the handoff bundle without
          exposing it through Reader chrome. */}
        <Route path="/design" element={<DesignSurfaceGate />}>
          <Route index element={<Home />} />
          <Route path="preview/:slug" element={<EmbeddedPreview />} />
        </Route>
        <Route path="/ui/:slug" element={<UiKitViewport />} />

      {/* Canonical project-scoped surfaces. Locale is project metadata, not
          route identity, after task-canonical-locale-migration. */}
      <Route path="/:org/p/:project" element={<ProjectBaseRedirect />} />
      <Route path="/:org/p/:project/today" element={<ReaderRoute view="today" />} />
      <Route path="/:org/p/:project/wiki" element={<ReaderRoute view="reader" />} />
      <Route path="/:org/p/:project/wiki/:slug" element={<ReaderRoute view="reader" />} />
      <Route path="/:org/p/:project/wiki/:slug/history" element={<History />} />
      <Route path="/:org/p/:project/wiki/:slug/diff" element={<Diff />} />
      <Route path="/:org/p/:project/task" element={<ProjectSurfaceRedirect segment="task" />} />
      <Route path="/:org/p/:project/task/:slug" element={<ProjectSurfaceRedirect segment="task" />} />
      <Route path="/:org/p/:project/tasks" element={<ReaderRoute view="tasks" />} />
      <Route path="/:org/p/:project/tasks/:slug" element={<ReaderRoute view="tasks" />} />
      <Route path="/:org/p/:project/graph" element={<GraphSurfaceGate />} />
      <Route path="/:org/p/:project/inbox" element={<ReaderRoute view="inbox" />} />
      <Route path="/:org/p/:project/settings" element={<ProjectSettingsPage />} />
      <Route path="/:org/p/:project/git/:repoId/commit/:sha" element={<CommitDetailPage />} />
      <Route path="/:org/p/:project/:surface" element={<ProjectSurfaceNotFound />} />
      <Route path="/:org/p/:project/:surface/*" element={<ProjectSurfaceNotFound />} />
      <Route path="/p/:project" element={<ProjectBaseRedirect />} />
      <Route path="/p/:project/today" element={<ReaderRoute view="today" />} />
      <Route path="/p/:project/wiki" element={<ReaderRoute view="reader" />} />
      <Route path="/p/:project/wiki/:slug" element={<ReaderRoute view="reader" />} />
      <Route path="/p/:project/wiki/:slug/history" element={<History />} />
      <Route path="/p/:project/wiki/:slug/diff" element={<Diff />} />
      <Route path="/p/:project/task" element={<ProjectSurfaceRedirect segment="task" />} />
      <Route path="/p/:project/task/:slug" element={<ProjectSurfaceRedirect segment="task" />} />
      <Route path="/p/:project/tasks" element={<ReaderRoute view="tasks" />} />
      <Route path="/p/:project/tasks/:slug" element={<ReaderRoute view="tasks" />} />
      <Route path="/p/:project/graph" element={<GraphSurfaceGate />} />
      <Route path="/p/:project/inbox" element={<ReaderRoute view="inbox" />} />
      <Route path="/p/:project/settings" element={<ProjectSettingsPage />} />
      <Route path="/p/:project/git/:repoId/commit/:sha" element={<CommitDetailPage />} />
      <Route path="/p/:project/:surface" element={<ProjectSurfaceNotFound />} />
      <Route path="/p/:project/:surface/*" element={<ProjectSurfaceNotFound />} />
      <Route path="/help/design-legend" element={<DesignLegendRedirect />} />
      <Route path="/signup" element={<SignupPage />} />
      <Route path="/signup/complete" element={<SignupCompletePage />} />

      {/* Project bootstrap (Decision project-bootstrap-canonical-flow-
          reader-ui-first-class). The 1급 사용자 진입점 for new
          installs — fresh users land here from the onboarding wizard
          (T4) or by direct URL. Hits POST /api/projects, then shows a
          copy-paste-ready .mcp.json snippet so the user finishes by
          opening Claude Code in the workspace they pasted into. */}
      <Route path="/projects/new" element={<CreateProjectPage />} />

      {/* Legacy /p/:project/:locale/... routes remove the locale segment.
          The daemon returns a real 301; this keeps Vite-dev sessions
          compatible too. */}
      <Route path="/p/:project/:locale/wiki/*" element={<LegacyLocaleRedirect base="wiki" />} />
      <Route path="/p/:project/:locale/today" element={<LegacyLocaleRedirect base="today" />} />
      <Route path="/p/:project/:locale/tasks/*" element={<LegacyLocaleRedirect base="tasks" />} />
      <Route path="/p/:project/:locale/graph" element={<LegacyLocaleRedirect base="graph" />} />
      <Route path="/p/:project/:locale/inbox" element={<LegacyLocaleRedirect base="inbox" />} />

      {/* Pre-project-scope legacy paths redirect to the default project's
          equivalent URL. Keeps old shares, old bookmarks, and the seed
          PINDOC.md pointer working. */}
      <Route path="/wiki/*" element={<LegacyRedirect base="wiki" />} />
      <Route path="/today" element={<LegacyRedirect base="today" />} />
      <Route path="/tasks/*" element={<LegacyRedirect base="tasks" />} />
      <Route path="/graph" element={<LegacyRedirect base="graph" />} />
      <Route path="/inbox" element={<LegacyRedirect base="inbox" />} />

      {/* Ops — Phase J MCP tool-call telemetry. Instance-wide, not
          project-scoped (tool calls span every project this instance
          serves). Reachable via /ops/telemetry; linked from the Reader
          TopNav's overflow menu. */}
      <Route path="/ops/telemetry" element={<Telemetry />} />

      {/* Admin — task-providers-admin-ui. Instance-level identity
          provider registry. Loopback principal only at the BE so non-
          owner callers see INSTANCE_OWNER_REQUIRED here too. */}
      <Route path="/admin/providers" element={<ProvidersPanel />} />

      {/* Agent-era first-time identity flow. Fresh installs land
          here from LegacyRedirect when /api/config.identity_required.
          Once the form is submitted, server_settings binds the new
          users.id and subsequent visits skip this route. */}
      <Route path="/onboarding/identity" element={<IdentitySetup />} />

      {/* Bare root. / redirects to /p/:default/today. */}
        <Route path="/" element={<LegacyRedirect base="today" />} />
      </Routes>
    </PindocTooltipProvider>
  );
}

function DesignSurfaceGate() {
  const location = useLocation();
  if (isReaderDevSurfaceEnabled(location.search, import.meta.env.DEV)) {
    return <ShellLayout />;
  }
  return <LegacyRedirect base="today" />;
}

function GraphSurfaceGate() {
  const location = useLocation();
  if (isReaderDevSurfaceEnabled(location.search, import.meta.env.DEV)) {
    return <ReaderRoute view="graph" />;
  }
  return <ReaderRoute view="reader" unavailableSurface="graph" />;
}

function ProjectBaseRedirect() {
  const { org, project = "" } = useParams<{ org?: string; project: string }>();
  const location = useLocation();
  return (
    <Navigate
      to={`${projectBaseRedirectPath(project, org ?? DEFAULT_READER_ORG_SLUG)}${location.search || ""}`}
      replace
    />
  );
}

function ProjectSurfaceRedirect({ segment }: { segment: string }) {
  const { org, project = "", slug } = useParams<{ org?: string; project: string; slug?: string }>();
  const location = useLocation();
  const surface = normalizeReaderSurfaceSegment(segment);
  if (!surface) return <ProjectSurfaceNotFound surfaceOverride={segment} />;
  return (
    <Navigate
      to={`${projectSurfacePath(project, surface, slug, org ?? DEFAULT_READER_ORG_SLUG)}${location.search || ""}`}
      replace
    />
  );
}

function ProjectSurfaceNotFound({ surfaceOverride }: { surfaceOverride?: string }) {
  const { surface = surfaceOverride ?? "" } = useParams<{ surface?: string }>();
  return <ReaderRoute view="reader" unavailableSurface={surfaceOverride ?? surface} />;
}

function ReaderRoute(props: { view: ReaderView; unavailableSurface?: string }) {
  const { org } = useParams<{ org?: string }>();
  return <ReaderShell {...props} orgSlug={org ?? DEFAULT_READER_ORG_SLUG} />;
}

function ShellLayout() {
  // Wraps /design Home + design-system preview cards. Live surfaces use their
  // own Reader shell (ReaderShell) which mounts directly under the top-level
  // Routes, not under this scaffold.
  const { t, lang, setLang } = useI18n();
  return (
    <div className="m1-shell">
      <aside className="m1-shell__nav">
        <header className="m1-shell__brand">
          <img src="/design-system/assets/logo.svg" alt="" width={20} height={20} />
          <span>Pindoc · M1</span>
        </header>
        <nav>
          <p className="m1-shell__group">{t("nav.live_data")}</p>
          <Link to="/" className="m1-shell__link">{t("nav.wiki_reader")}</Link>
          <p className="m1-shell__group">{t("nav.ui_kits")}</p>
          {uiKits.map((s) => (
            <Link key={s.slug} to={`/ui/${s.slug}`} className="m1-shell__link">
              {s.label}
              {s.sublabel && <span className="m1-shell__sublabel">{s.sublabel}</span>}
            </Link>
          ))}
          <p className="m1-shell__group">{t("nav.design_refs")}</p>
          {previews.map((s) => (
            <NavLink key={s.slug} to={`/design/preview/${s.slug}`} className="m1-shell__link">
              {s.label}
            </NavLink>
          ))}
          <p className="m1-shell__group">{t("nav.bundle_docs")}</p>
          <a className="m1-shell__link" href="/design-system/README.md" target="_blank" rel="noreferrer">
            {t("nav.design_readme")}
          </a>
          <a className="m1-shell__link" href="/design-system/SKILL.md" target="_blank" rel="noreferrer">
            {t("nav.design_skill")}
          </a>
        </nav>
        <footer className="m1-shell__foot">
          <div className="m1-shell__langs">
            <span>{t("lang.switch")}:</span>
            <button type="button" className={lang === "en" ? "is-active" : ""} onClick={() => setLang("en")}>
              {t("lang.en")}
            </button>
            <button type="button" className={lang === "ko" ? "is-active" : ""} onClick={() => setLang("ko")}>
              {t("lang.ko")}
            </button>
          </div>
          <div>M1 scaffold · design-system v0</div>
        </footer>
      </aside>
      <main className="m1-shell__main">
        <Outlet />
      </main>
    </div>
  );
}

function Home() {
  const { t } = useI18n();
  return (
    <article className="m1-shell__home">
      <h1>{t("home.title")}</h1>
      <p className="m1-shell__callout">{t("home.callout")}</p>
      <h2>Two kinds of surfaces</h2>
      <ul>
        <li>
          <strong>Live data surfaces</strong> open in a React-ified Reader shell
          ported from the Claude Design bundle. Wiki / Tasks / Graph / Inbox.
          Top nav + sidebar + sidecar + ⌘K all match the handoff prototype.
        </li>
        <li>
          <strong>UI kits</strong> open in a clean full-viewport route
          (<code>/ui/*</code>) as the raw HTML prototypes — useful for diffing
          against future Claude Design exports.
        </li>
        <li>
          <strong>Design-system references</strong> open here in this shell
          (<code>/design/preview/*</code>) because they are small swatches —
          Typography, Neutral ramp, Components, etc.
        </li>
      </ul>
      <p className="m1-shell__note">
        Handoff source: Claude Design bundle v0 · Imported 2026-04-21.
      </p>
    </article>
  );
}

// LegacyRedirect resolves the server's default project slug once and rewrites
// /wiki/foo → /p/:default/wiki/foo. It fires on:
//   - /            (bare root)
//   - /wiki/...    (legacy pointer in PINDOC.md + old shares)
//   - /tasks/...   (same)
//   - /graph, /inbox
// Purpose: keep URL shares from the pre-multiproject era working while every
// canonical link migrates to the /p/:project/… shape.
function LegacyRedirect({ base }: { base: "wiki" | "tasks" | "graph" | "inbox" | "today" }) {
  const location = useLocation();
  const [target, setTarget] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const { t } = useI18n();

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const cfg = await api.config();
        if (cancelled) return;
        // Identity intercept first — the agent-era first-time flow
        // routes a fresh install to /onboarding/identity before any
        // project chrome loads. Self-correcting: once the form
        // commits, settings.default_loopback_user_id is bound and
        // identity_required flips to false on the next /api/config.
        if (cfg.identity_required) {
          setTarget(`/onboarding/identity`);
          return;
        }
        // Onboarding intercept (Decision project-bootstrap-canonical-
        // flow-reader-ui-first-class): when the instance has no
        // projects other than the seed `pindoc` row, redirect a fresh
        // user to the new-project wizard instead of the legacy
        // /p/{default}/{base} landing. The wizard re-uses
        // /projects/new with `?welcome=1` so the page renders a
        // friendlier header. Self-correcting — once they create a
        // project, onboarding_required flips to false on the next
        // /api/config call.
        if (cfg.onboarding_required) {
          setTarget(`/projects/new?welcome=1`);
          return;
        }
        const tail = trimLegacyPrefix(location.pathname, base);
        const suffix = tail ? `/${tail}` : "";
        const search = location.search || "";
        setTarget(`/p/${cfg.default_project_slug}/${base}${suffix}${search}`);
      } catch (e) {
        if (!cancelled) setErr(String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [base, location.pathname, location.search]);

  if (err) {
    return (
      <div className="reader-state reader-state--error">
        <strong>{t("wiki.error_generic_title")}</strong>
        <p>{err}</p>
        {import.meta.env.DEV && (
          <p>
            {t("wiki.error_dev_hint_prefix")} <code>{t("wiki.error_dev_hint_cmd")}</code>{" "}
            {t("wiki.error_dev_hint_suffix")}
          </p>
        )}
      </div>
    );
  }
  if (!target) {
    return <div className="reader-state">{t("wiki.loading")}</div>;
  }
  return <Navigate to={target} replace />;
}

function DesignLegendRedirect() {
  const location = useLocation();
  const [target, setTarget] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const { t } = useI18n();

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const cfg = await api.config();
        if (cancelled) return;
        setTarget(
          `/p/${cfg.default_project_slug}/wiki/visual-language-reference${location.search || ""}`,
        );
      } catch (e) {
        if (!cancelled) setErr(String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [location.search]);

  if (err) {
    return (
      <div className="reader-state reader-state--error">
        <strong>{t("wiki.error_generic_title")}</strong>
        <p>{err}</p>
        {import.meta.env.DEV && (
          <p>
            {t("wiki.error_dev_hint_prefix")} <code>{t("wiki.error_dev_hint_cmd")}</code>{" "}
            {t("wiki.error_dev_hint_suffix")}
          </p>
        )}
      </div>
    );
  }
  if (!target) {
    return <div className="reader-state">{t("wiki.loading")}</div>;
  }
  return <Navigate to={target} replace />;
}

// LegacyLocaleRedirect fires for `/p/:project/:locale/(wiki|tasks|graph|inbox)`
// shares from the Phase 18 URL shape and rewrites them to the canonical
// locale-free URL, keeping every path suffix and query string intact.
function LegacyLocaleRedirect({ base }: { base: "wiki" | "tasks" | "graph" | "inbox" | "today" }) {
  const { project = "", locale = "" } = useParams<{ project: string; locale: string }>();
  const location = useLocation();
  const [target, setTarget] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const { t } = useI18n();

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        if (cancelled) return;
        // pathname looks like "/p/<project>/<locale>/<base>/<rest>"; strip
        // the prefix and rebuild without the locale segment.
        const prefix = `/p/${project}/${locale}/${base}`;
        const rest = location.pathname.startsWith(prefix)
          ? location.pathname.slice(prefix.length)
          : "";
        const search = location.search || "";
        setTarget(`/p/${project}/${base}${rest}${search}`);
      } catch (e) {
        if (!cancelled) setErr(String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [project, locale, base, location.pathname, location.search]);

  if (err) {
    return (
      <div className="reader-state reader-state--error">
        <strong>{t("wiki.error_generic_title")}</strong>
        <p>{err}</p>
      </div>
    );
  }
  if (!target) {
    return <div className="reader-state">{t("wiki.loading")}</div>;
  }
  return <Navigate to={target} replace />;
}

function trimLegacyPrefix(pathname: string, base: string): string {
  // Strip leading "/" + base + optional "/" so the suffix is the artifact
  // slug / sub-path the new route expects.
  const p = pathname.replace(/^\/+/, "");
  if (p === base) return "";
  if (p.startsWith(`${base}/`)) return p.slice(base.length + 1);
  // Bare root ("/") falls through here — "p" is "" after the strip.
  return "";
}

function EmbeddedPreview() {
  const { slug } = useParams<{ slug: string }>();
  const surface = findSurface(slug);
  if (!surface) return <NotFound slug={slug} />;
  return (
    <iframe
      key={surface.slug}
      title={surface.label}
      src={surface.path}
      className="m1-shell__frame"
    />
  );
}

type DeviceMode = "desktop" | "tablet" | "mobile" | "fit";
const DEVICE_WIDTHS: Record<DeviceMode, number | null> = {
  desktop: 1440,
  tablet: 768,
  mobile: 375,
  fit: null,
};

function UiKitViewport() {
  const { slug } = useParams<{ slug: string }>();
  const surface = findSurface(slug);
  const [mode, setMode] = useState<DeviceMode>("fit");
  if (!surface) return <NotFound slug={slug} />;

  const width = DEVICE_WIDTHS[mode];
  const frameStyle: React.CSSProperties =
    width === null
      ? { width: "100%", height: "100%" }
      : { width: `${width}px`, height: "100%", maxHeight: "100%" };

  return (
    <div className="uikit">
      <header className="uikit__bar">
        <Link to="/design" className="uikit__back">◀ M1 home</Link>
        <div className="uikit__title">
          <span>{surface.label}</span>
          {surface.sublabel && <span className="uikit__sub">{surface.sublabel}</span>}
        </div>
        <div className="uikit__devices">
          {(Object.keys(DEVICE_WIDTHS) as DeviceMode[]).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMode(m)}
              className={`uikit__dev ${mode === m ? "is-active" : ""}`}
            >
              {deviceLabel(m)}
            </button>
          ))}
        </div>
      </header>
      <div className="uikit__stage">
        <iframe
          key={`${surface.slug}-${mode}`}
          title={surface.label}
          src={surface.path}
          style={frameStyle}
          className="uikit__frame"
        />
      </div>
    </div>
  );
}

function deviceLabel(m: DeviceMode): string {
  switch (m) {
    case "desktop": return "Desktop 1440";
    case "tablet":  return "Tablet 768";
    case "mobile":  return "Mobile 375";
    case "fit":     return "Fit";
  }
}

function NotFound({ slug }: { slug: string | undefined }) {
  return (
    <article className="m1-shell__home">
      <h1>Not found</h1>
      <p>
        <code>{slug}</code> is not a known surface. Check <code>src/surfaces.ts</code>.
      </p>
      <p>
        <Link to="/design">Back to M1 home</Link>
      </p>
    </article>
  );
}
