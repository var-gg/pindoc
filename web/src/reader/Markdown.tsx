import { useEffect, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import mermaid from "mermaid";
import { uniqueSlug } from "./slug";

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
  // Each render keeps its own slug ledger so duplicate H2 titles inside
  // one artifact get stable `-2`, `-3` suffixes. Ledger lives inside
  // useMemo so it rebuilds when the body changes but is stable during a
  // single render pass (headingId is called once per h2 the renderer
  // walks across, in document order).
  const headingIds = useMemo(() => {
    const used = new Set<string>();
    return (text: string) => uniqueSlug(text, used);
  }, [source]);

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
          const id = headingIds(extractHeadingText(children));
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
