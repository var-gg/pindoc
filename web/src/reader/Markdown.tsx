import { useEffect, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import mermaid from "mermaid";
import { headingsFromBody, slugifyHeading } from "./slug";

// Initialize mermaid once per page. Theme follows the Pindoc dark/light
// class on <html>. We invert the theme selection at render time so fresh
// toggles take effect on next render.
let mermaidInited = false;
function ensureMermaid(): void {
  if (mermaidInited) return;
  const isDark = document.documentElement.classList.contains("theme-dark");
  mermaid.initialize({
    startOnLoad: false,
    theme: isDark ? "dark" : "default",
    securityLevel: "loose",
    fontFamily: "JetBrains Mono, ui-monospace, monospace",
  });
  mermaidInited = true;
}

/**
 * PindocMarkdown wraps react-markdown with the set of renderers Pindoc's
 * PINDOC.md promises agents can rely on. Today: GFM (tables, task lists,
 * strikethrough, autolinks) and Mermaid diagrams via fenced `mermaid`
 * blocks. Agents inspecting pindoc.project.current see this same list
 * under `rendering.markdown` so they never render something we don't
 * display.
 */
export function PindocMarkdown({ source }: { source: string }) {
  // Derive every H2's final slug up-front from the raw source so the
  // h2 renderer is a pure lookup instead of a stateful slug ledger.
  // The previous design kept a `used` Set inside a memoized closure —
  // harmless in production but under React StrictMode's dev double-render
  // the Set accumulated across the two passes, so "Purpose" on the second
  // pass collided and came back as "purpose-2". TOC hrefs (fresh ledger
  // from headingsFromBody) then pointed at ids that didn't exist, which
  // is what made the TOC look dead. Index-by-text-occurrence + a local
  // counter keeps both sides in lock-step and is idempotent per render.
  const slugsByText = useMemo(() => {
    const map = new Map<string, string[]>();
    for (const h of headingsFromBody(source)) {
      const bucket = map.get(h.text);
      if (bucket) bucket.push(h.slug);
      else map.set(h.text, [h.slug]);
    }
    return map;
  }, [source]);

  // Per-render counter: a fresh Map every time PindocMarkdown runs, so
  // StrictMode's double-invoke gets independent state and h2 ids stay
  // deterministic across both passes.
  const counters = new Map<string, number>();

  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        code(props) {
          const { className, children } = props;
          const match = /language-(\w+)/.exec(className || "");
          if (match && match[1] === "mermaid") {
            return <MermaidBlock source={String(children).trimEnd()} />;
          }
          return <code className={className}>{children}</code>;
        },
        h2({ children }) {
          // extractHeadingText flattens children (often an array of
          // strings + inline code + emphasis) back into the plain text
          // used for slug generation. Keeping the rendered children
          // unchanged preserves inline formatting on the heading itself.
          const text = extractHeadingText(children);
          const slugs = slugsByText.get(text);
          const n = counters.get(text) ?? 0;
          counters.set(text, n + 1);
          const id = slugs?.[n] ?? slugifyHeading(text);
          return <h2 id={id}>{children}</h2>;
        },
      }}
    >
      {source}
    </ReactMarkdown>
  );
}

function extractHeadingText(node: unknown): string {
  if (node == null || node === false) return "";
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(extractHeadingText).join("");
  if (typeof node === "object" && "props" in (node as Record<string, unknown>)) {
    const props = (node as { props?: { children?: unknown } }).props;
    return extractHeadingText(props?.children);
  }
  return "";
}

function MermaidBlock({ source }: { source: string }) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [state, setState] = useState<"idle" | "rendered" | "error">("idle");
  const [errMsg, setErrMsg] = useState<string>("");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      ensureMermaid();
      try {
        const id = `mermaid-${Math.random().toString(36).slice(2, 10)}`;
        const { svg } = await mermaid.render(id, source);
        if (cancelled || !ref.current) return;
        ref.current.innerHTML = svg;
        setState("rendered");
      } catch (err) {
        if (cancelled) return;
        setErrMsg(String(err));
        setState("error");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [source]);

  if (state === "error") {
    return (
      <pre
        style={{
          background: "var(--bg-2)",
          borderLeft: "2px solid var(--stale)",
          padding: "12px 14px",
          fontSize: "12.5px",
          whiteSpace: "pre-wrap",
        }}
      >
        <strong style={{ color: "var(--stale)" }}>mermaid render failed:</strong>
        {"\n"}
        {errMsg}
        {"\n\n"}
        {source}
      </pre>
    );
  }

  return (
    <div
      ref={ref}
      className="mermaid-diagram"
      style={{
        background: "var(--bg-1)",
        border: "1px solid var(--border)",
        borderRadius: "var(--r-3)",
        padding: "16px",
        margin: "16px 0",
        overflowX: "auto",
      }}
    />
  );
}
