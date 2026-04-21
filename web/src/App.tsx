import { NavLink, Route, Routes, useParams } from "react-router-dom";
import { surfaces } from "./surfaces";

export function App() {
  return (
    <div className="m1-shell">
      <aside className="m1-shell__nav">
        <header className="m1-shell__brand">
          <img src="/design-system/assets/logo.svg" alt="" width={20} height={20} />
          <span>Pindoc · M1</span>
        </header>
        <nav>
          <p className="m1-shell__group">UI kits</p>
          {surfaces.ui_kits.map((s) => (
            <NavLink key={s.slug} to={`/preview/${s.slug}`} className="m1-shell__link">
              {s.label}
            </NavLink>
          ))}
          <p className="m1-shell__group">Previews</p>
          {surfaces.preview.map((s) => (
            <NavLink key={s.slug} to={`/preview/${s.slug}`} className="m1-shell__link">
              {s.label}
            </NavLink>
          ))}
          <p className="m1-shell__group">Docs</p>
          <a className="m1-shell__link" href="/design-system/README.md" target="_blank" rel="noreferrer">
            Design README
          </a>
          <a className="m1-shell__link" href="/design-system/SKILL.md" target="_blank" rel="noreferrer">
            Design SKILL
          </a>
        </nav>
        <footer className="m1-shell__foot">
          <span>V1 · M1 scaffold · design-system v0</span>
        </footer>
      </aside>
      <main className="m1-shell__main">
        <Routes>
          <Route index element={<Home />} />
          <Route path="/preview/:slug" element={<PreviewFrame />} />
        </Routes>
      </main>
    </div>
  );
}

function Home() {
  return (
    <article className="m1-shell__home">
      <h1>Pindoc · M1 visual skeleton</h1>
      <p>
        Pick a surface on the left to open its HTML prototype inside a frame. These
        prototypes came out of Claude Design and live under <code>/design-system/</code>.
        Nothing here is React-ified yet — that's the next pass.
      </p>
      <p>
        The tokens in <code>colors_and_type.css</code> are globally loaded at the shell
        level, so this page already uses them. Light mode is default. Toggle dark via
        the reader UI kit (the toggle persists to <code>localStorage</code>).
      </p>
      <p className="m1-shell__note">
        Handoff source: Claude Design bundle v0 · Imported 2026-04-21.
      </p>
    </article>
  );
}

function PreviewFrame() {
  const { slug } = useParams<{ slug: string }>();
  const surface = [...surfaces.ui_kits, ...surfaces.preview].find((s) => s.slug === slug);
  if (!surface) {
    return (
      <article className="m1-shell__home">
        <h1>Not found</h1>
        <p>
          <code>{slug}</code> is not a known surface. Check <code>src/surfaces.ts</code>.
        </p>
      </article>
    );
  }
  return (
    <iframe
      key={surface.slug}
      title={surface.label}
      src={surface.path}
      className="m1-shell__frame"
    />
  );
}
