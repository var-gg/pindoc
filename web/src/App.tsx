import { useEffect, useState } from "react";
import { Link, Navigate, NavLink, Outlet, Route, Routes, useLocation, useParams } from "react-router";
import { api } from "./api/client";
import { useI18n } from "./i18n";
import { Diff } from "./reader/Diff";
import { History } from "./reader/History";
import { ReaderShell } from "./reader/ReaderShell";
import { findSurface, previews, uiKits } from "./surfaces";

export function App() {
  return (
    <Routes>
      {/* Design-system scaffold. Lives at /design so the bare root can
          canonical-redirect to a project-scoped URL. */}
      <Route path="/design" element={<ShellLayout />}>
        <Route index element={<Home />} />
        <Route path="preview/:slug" element={<EmbeddedPreview />} />
      </Route>
      <Route path="/ui/:slug" element={<UiKitViewport />} />

      {/* Canonical project-scoped surfaces. Every live path carries /p/:project
          so URLs are shareable without ambient project context. */}
      <Route path="/p/:project/wiki" element={<ReaderShell view="reader" />} />
      <Route path="/p/:project/wiki/:slug" element={<ReaderShell view="reader" />} />
      <Route path="/p/:project/wiki/:slug/history" element={<History />} />
      <Route path="/p/:project/wiki/:slug/diff" element={<Diff />} />
      <Route path="/p/:project/tasks" element={<ReaderShell view="tasks" />} />
      <Route path="/p/:project/tasks/:slug" element={<ReaderShell view="tasks" />} />
      <Route path="/p/:project/graph" element={<ReaderShell view="graph" />} />
      <Route path="/p/:project/inbox" element={<ReaderShell view="inbox" />} />

      {/* Legacy paths redirect to the default project's equivalent URL. Keeps
          old shares, old bookmarks, and the seed PINDOC.md pointer working. */}
      <Route path="/wiki/*" element={<LegacyRedirect base="wiki" />} />
      <Route path="/tasks/*" element={<LegacyRedirect base="tasks" />} />
      <Route path="/graph" element={<LegacyRedirect base="graph" />} />
      <Route path="/inbox" element={<LegacyRedirect base="inbox" />} />

      {/* Bare root. / redirects to /p/:default/wiki. */}
      <Route path="/" element={<LegacyRedirect base="wiki" />} />
    </Routes>
  );
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
function LegacyRedirect({ base }: { base: "wiki" | "tasks" | "graph" | "inbox" }) {
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
        <strong>{t("wiki.error_title")}</strong>
        <p>{err}</p>
        <p>
          {t("wiki.error_hint_prefix")} <code>{t("wiki.error_hint_cmd")}</code>{" "}
          {t("wiki.error_hint_suffix")}
        </p>
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
