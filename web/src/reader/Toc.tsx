import { useEffect, useRef, useState } from "react";
import { useI18n } from "../i18n";

// Toc — section inside Sidecar that lists every H2 in the current artifact
// body and scroll-spies the active section via IntersectionObserver.
// Clicks trigger smooth scroll + URL fragment update so the section is
// bookmarkable. Rendered only when headings.length >= 2; the parent
// Sidecar gates visibility so a one-section artifact doesn't get a
// TOC-shaped empty card.
//
// Scroll container = window (after reader-toc-into-sidecar refactor), so
// root=null and scrollIntoView both target the viewport naturally. Hiding
// on narrow width follows Sidecar's own responsive rules — the old
// :root[data-reader-width="narrow"] .reader-toc override was dropped
// because Sidecar hides itself at that breakpoint anyway.

type Heading = { text: string; slug: string };

type Props = {
  headings: Heading[];
};

export function Toc({ headings }: Props) {
  const { t } = useI18n();
  const [active, setActive] = useState<string | null>(headings[0]?.slug ?? null);
  const clickLockUntil = useRef(0);

  useEffect(() => {
    if (headings.length === 0) return;
    const els = headings
      .map((h) => document.getElementById(h.slug))
      .filter((el): el is HTMLElement => el !== null);
    if (els.length === 0) return;

    // rootMargin: top=0, bottom=-70% viewport → a heading counts as "in
    // view" once it reaches the top 30% of the viewport. This matches
    // how readers actually track "the section I'm reading" rather than
    // "the section at the absolute top."
    const observer = new IntersectionObserver(
      (entries) => {
        if (Date.now() < clickLockUntil.current) return;
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible.length > 0) {
          setActive(visible[0].target.id);
        }
      },
      { root: null, rootMargin: "0px 0px -70% 0px", threshold: [0, 1] },
    );
    for (const el of els) observer.observe(el);
    return () => observer.disconnect();
  }, [headings]);

  function handleClick(e: React.MouseEvent<HTMLAnchorElement>, slug: string) {
    e.preventDefault();
    const el = document.getElementById(slug);
    if (!el) return;
    clickLockUntil.current = Date.now() + 900;
    el.scrollIntoView({ behavior: "smooth", block: "start" });
    history.replaceState(null, "", `#${slug}`);
    setActive(slug);
  }

  return (
    <nav className="reader-toc" aria-label={t("reader.toc_title")}>
      <div className="reader-toc__title">{t("reader.toc_title")}</div>
      <ul className="reader-toc__list">
        {headings.map((h) => (
          <li key={h.slug}>
            <a
              href={`#${h.slug}`}
              className={`reader-toc__link${active === h.slug ? " is-active" : ""}`}
              onClick={(e) => handleClick(e, h.slug)}
            >
              {h.text}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}
