import { useState } from "react";
import { Link, NavLink, Outlet, Route, Routes, useParams } from "react-router";
import { useI18n } from "./i18n";
import { ReaderShell } from "./reader/ReaderShell";
import { findSurface, previews, uiKits } from "./surfaces";

export function App() {
  return (
    <Routes>
      <Route element={<ShellLayout />}>
        <Route index element={<Home />} />
        <Route path="/preview/:slug" element={<EmbeddedPreview />} />
      </Route>
      <Route path="/ui/:slug" element={<UiKitViewport />} />

      {/* Phase 4 React-ified surfaces. ReaderShell owns top nav + sidebar
          + sidecar so every live surface shares the design-system chrome. */}
      <Route path="/wiki" element={<ReaderShell view="reader" />} />
      <Route path="/wiki/:slug" element={<ReaderShell view="reader" />} />
      <Route path="/tasks" element={<ReaderShell view="tasks" />} />
      <Route path="/tasks/:slug" element={<ReaderShell view="tasks" />} />
      <Route path="/graph" element={<ReaderShell view="graph" />} />
      <Route path="/inbox" element={<ReaderShell view="inbox" />} />
    </Routes>
  );
}

function ShellLayout() {
  // Wraps Home + design-system preview cards. Live surfaces use their own
  // reader shell (ReaderShell) which mounts directly under the top-level
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
          <NavLink to="/wiki" className="m1-shell__link">{t("nav.wiki_reader")}</NavLink>
          <NavLink to="/tasks" className="m1-shell__link">{t("nav.tasks")}</NavLink>
          <NavLink to="/graph" className="m1-shell__link">{t("nav.graph")}</NavLink>
          <NavLink to="/inbox" className="m1-shell__link">{t("nav.inbox")}</NavLink>
          <p className="m1-shell__group">{t("nav.ui_kits")}</p>
          {uiKits.map((s) => (
            <Link key={s.slug} to={`/ui/${s.slug}`} className="m1-shell__link">
              {s.label}
              {s.sublabel && <span className="m1-shell__sublabel">{s.sublabel}</span>}
            </Link>
          ))}
          <p className="m1-shell__group">{t("nav.design_refs")}</p>
          {previews.map((s) => (
            <NavLink key={s.slug} to={`/preview/${s.slug}`} className="m1-shell__link">
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
          (<code>/preview/*</code>) because they are small swatches —
          Typography, Neutral ramp, Components, etc.
        </li>
      </ul>
      <p className="m1-shell__note">
        Handoff source: Claude Design bundle v0 · Imported 2026-04-21.
      </p>
    </article>
  );
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
        <Link to="/" className="uikit__back">◀ M1 home</Link>
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
        <Link to="/">Back to M1 home</Link>
      </p>
    </article>
  );
}
