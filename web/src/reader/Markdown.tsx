import { useEffect, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import mermaid from "mermaid";
import { headingsFromBody, slugifyHeading } from "./slug";
import { useI18n } from "../i18n";
import { pindocUrlTransform } from "./urlTransform";
import { isStructureOverlapHeading, type StructureOverlapSection } from "./structureSections";

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
export function PindocMarkdown({
  source,
  projectSlug,
  collapseStructureSections = false,
}: {
  source: string;
  projectSlug?: string;
  collapseStructureSections?: boolean;
}) {
  const { blocks, hiddenSections } = useMemo(
    () => markdownBlocks(source, collapseStructureSections),
    [source, collapseStructureSections],
  );

  return (
    <>
      {blocks.map((block, i) => (
        <MarkdownBlock
          key={`md-${i}`}
          source={block.source}
          headingSlugs={block.headingSlugs}
          projectSlug={projectSlug}
        />
      ))}
      {hiddenSections.length > 0 && (
        <OriginalStructureSections
          sections={hiddenSections}
          projectSlug={projectSlug}
        />
      )}
    </>
  );
}

function MarkdownBlock({
  source,
  headingSlugs,
  projectSlug,
}: {
  source: string;
  headingSlugs: string[];
  projectSlug?: string;
}) {
  let headingIndex = 0;

  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      urlTransform={(url) => pindocUrlTransform(url, projectSlug)}
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
          const id = headingSlugs[headingIndex++] ?? slugifyHeading(text);
          return <h2 id={id}>{children}</h2>;
        },
      }}
    >
      {source}
    </ReactMarkdown>
  );
}

type MarkdownRenderBlock =
  | { kind: "markdown"; source: string; headingSlugs: string[] };

function markdownBlocks(
  source: string,
  collapseStructureSections: boolean,
): { blocks: MarkdownRenderBlock[]; hiddenSections: StructureOverlapSection[] } {
  const headings = headingsFromBody(source);
  if (!collapseStructureSections || headings.length === 0) {
    return {
      blocks: [{ kind: "markdown", source, headingSlugs: headings.map((h) => h.slug) }],
      hiddenSections: [],
    };
  }

  const lines = source.split(/\r?\n/);
  const blocks: MarkdownRenderBlock[] = [];
  const hiddenSections: StructureOverlapSection[] = [];
  const pending: string[] = [];
  const pendingSlugs: string[] = [];
  let headingIndex = 0;
  let inFence = false;

  const flushPending = () => {
    const text = pending.join("\n").trim();
    if (!text) {
      pending.length = 0;
      pendingSlugs.length = 0;
      return;
    }
    blocks.push({
      kind: "markdown",
      source: pending.join("\n"),
      headingSlugs: [...pendingSlugs],
    });
    pending.length = 0;
    pendingSlugs.length = 0;
  };

  for (let i = 0; i < lines.length; ) {
    const line = lines[i].trimEnd();
    if (/^```/.test(line.trim())) {
      inFence = !inFence;
      pending.push(lines[i]);
      i += 1;
      continue;
    }
    const m = !inFence ? /^##\s+(.+?)\s*#*\s*$/.exec(line) : null;
    if (!m) {
      pending.push(lines[i]);
      i += 1;
      continue;
    }

    const title = m[1].trim();
    const heading = headings[headingIndex++];
    const slug = heading?.slug ?? slugifyHeading(title);
    let end = i + 1;
    let sectionFence = false;
    while (end < lines.length) {
      const next = lines[end].trimEnd();
      if (/^```/.test(next.trim())) {
        sectionFence = !sectionFence;
      } else if (!sectionFence && /^##\s+(.+?)\s*#*\s*$/.test(next)) {
        break;
      }
      end += 1;
    }

    if (isStructureOverlapHeading(title)) {
      flushPending();
      hiddenSections.push({
        title,
        slug,
        body: lines.slice(i + 1, end).join("\n").trim(),
      });
    } else {
      pending.push(...lines.slice(i, end));
      pendingSlugs.push(slug);
    }
    i = end;
  }
  flushPending();
  return { blocks, hiddenSections };
}

function OriginalStructureSections({
  sections,
  projectSlug,
}: {
  sections: StructureOverlapSection[];
  projectSlug?: string;
}) {
  const { t } = useI18n();
  return (
    <details id="reader-original-structure" className="reader-original-structure">
      <summary>{t("reader.structure_original_toggle", sections.length)}</summary>
      <div className="reader-original-structure__body">
        {sections.map((section) => (
          <section key={section.slug} id={section.slug} className="reader-original-structure__section">
            <h2>{section.title}</h2>
            {section.body ? (
              <MarkdownBlock source={section.body} headingSlugs={[]} projectSlug={projectSlug} />
            ) : (
              <p>{t("reader.structure_empty")}</p>
            )}
          </section>
        ))}
      </div>
    </details>
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
