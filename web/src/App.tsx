import { useState } from "react";
import { Link, NavLink, Outlet, Route, Routes, useParams } from "react-router";
import { findSurface, previews, uiKits } from "./surfaces";

export function App() {
  return (
    <Routes>
      <Route element={<ShellLayout />}>
        <Route index element={<Home />} />
        <Route path="/preview/:slug" element={<EmbeddedPreview />} />
      </Route>
      <Route path="/ui/:slug" element={<UiKitViewport />} />
    </Routes>
  );
}

function ShellLayout() {
  // Wraps Home + preview cards (small design-system references).
  // UI Kit routes use their own full-viewport layout, not this shell.
  return (
    <div className="m1-shell">
      <aside className="m1-shell__nav">
        <header className="m1-shell__brand">
          <img src="/design-system/assets/logo.svg" alt="" width={20} height={20} />
          <span>Pindoc · M1</span>
        </header>
        <nav>
          <p className="m1-shell__group">UI kits (full screen)</p>
          {uiKits.map((s) => (
            <Link key={s.slug} to={`/ui/${s.slug}`} className="m1-shell__link">
              {s.label}
              {s.sublabel && <span className="m1-shell__sublabel">{s.sublabel}</span>}
            </Link>
          ))}
          <p className="m1-shell__group">Design-system references</p>
          {previews.map((s) => (
            <NavLink key={s.slug} to={`/preview/${s.slug}`} className="m1-shell__link">
              {s.label}
            </NavLink>
          ))}
          <p className="m1-shell__group">Bundle docs</p>
          <a className="m1-shell__link" href="/design-system/README.md" target="_blank" rel="noreferrer">
            Design README
          </a>
          <a className="m1-shell__link" href="/design-system/SKILL.md" target="_blank" rel="noreferrer">
            Design SKILL
          </a>
        </nav>
        <footer className="m1-shell__foot">M1 scaffold · design-system v0</footer>
      </aside>
      <main className="m1-shell__main">
        <Outlet />
      </main>
    </div>
  );
}

function Home() {
  return (
    <article className="m1-shell__home">
      <h1>Pindoc · M1 visual skeleton</h1>
      <p className="m1-shell__callout">
        <strong>Everything shown here is a Claude Design mockup.</strong> Not a
        React component yet. The outer left rail is a dev scaffold for browsing
        the handoff — it is not part of the product. React-ification starts in
        M1.5 (Wiki Reader first).
      </p>
      <h2>Two kinds of surfaces</h2>
      <ul>
        <li>
          <strong>UI kits</strong> open in a clean <em>full viewport</em> route
          (<code>/ui/*</code>) with a device-size switcher (Desktop 1440 / Tablet
          768 / Mobile 375 / Fit). That is the right way to judge layout.
        </li>
        <li>
          <strong>Design-system references</strong> open here in this shell
          (<code>/preview/*</code>) because they are small swatches — Typography,
          Neutral ramp, Components, etc. Storybook-style, not product screens.
        </li>
      </ul>
      <h2>What is mockup vs what is ours</h2>
      <ul>
        <li>
          <code>web/public/design-system/</code> → Claude Design bundle,
          unmodified. Do not edit here; iterate in Claude Design and re-export.
        </li>
        <li>
          <code>web/src/</code> → M1 dev shell (routing + device switcher only).
          Scaffolding, throwaway.
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
        <code>{slug}</code> is not a known surface. Check
        <code> src/surfaces.ts</code>.
      </p>
      <p>
        <Link to="/">Back to M1 home</Link>
      </p>
    </article>
  );
}
