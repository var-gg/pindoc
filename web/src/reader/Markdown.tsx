import {
  isValidElement,
  useEffect,
  useMemo,
  useRef,
  useState,
  type PointerEvent,
  type ReactNode,
  type WheelEvent,
} from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkBreaks from "remark-breaks";
import mermaid from "mermaid";
import {
  AlertCircle,
  AlertTriangle,
  Info,
  Lightbulb,
  Maximize2,
  Minimize2,
  RotateCcw,
  ShieldAlert,
  ZoomIn,
  ZoomOut,
} from "lucide-react";
import { createHighlighter, type Highlighter } from "shiki";
import { headingsFromBody, slugifyHeading } from "./slug";
import { useI18n } from "../i18n";
import { pindocUrlTransform } from "./urlTransform";
import { isStructureOverlapHeading, type StructureOverlapSection } from "./structureSections";
import { remarkGithubAlerts } from "./remarkGithubAlerts";

// Initialize mermaid once per page. Theme follows the Pindoc dark/light
// class on <html>. We invert the theme selection at render time so fresh
// toggles take effect on next render.
let mermaidInited = false;
let mermaidTheme: "default" | "dark" | null = null;
let mermaidRenderQueue: Promise<void> = Promise.resolve();
let mermaidRenderCounter = 0;
const mermaidSvgCache = new Map<string, string>();

function currentMermaidTheme(): "default" | "dark" {
  return document.documentElement.classList.contains("theme-dark") ? "dark" : "default";
}

function mermaidCacheKey(source: string, theme: "default" | "dark"): string {
  return `${theme}\n${source}`;
}

function ensureMermaid(): "default" | "dark" {
  const theme = currentMermaidTheme();
  if (mermaidInited && mermaidTheme === theme) return theme;
  mermaid.initialize({
    startOnLoad: false,
    theme,
    securityLevel: "loose",
    fontFamily: "JetBrains Mono, ui-monospace, monospace",
    flowchart: { useMaxWidth: false },
    sequence: { useMaxWidth: false },
    er: { useMaxWidth: false },
    gantt: { useMaxWidth: false },
  });
  mermaidInited = true;
  mermaidTheme = theme;
  return theme;
}

function queueMermaidRender(render: () => Promise<void>): Promise<void> {
  const next = mermaidRenderQueue.then(render, render);
  mermaidRenderQueue = next.catch(() => undefined);
  return next;
}

// Shiki bootstrap. We mirror the mermaid pattern: lazy-create one
// highlighter for the page, cache rendered HTML keyed by (theme, lang,
// source), and re-derive the theme from the DOM on every render so
// toggles surface fresh syntax colors.
const SHIKI_LANGS = [
  "javascript",
  "typescript",
  "jsx",
  "tsx",
  "html",
  "css",
  "scss",
  "json",
  "go",
  "python",
  "rust",
  "java",
  "kotlin",
  "csharp",
  "php",
  "ruby",
  "swift",
  "c",
  "cpp",
  "bash",
  "shell",
  "powershell",
  "yaml",
  "toml",
  "ini",
  "sql",
  "xml",
  "markdown",
  "diff",
  "dockerfile",
  "graphql",
  "lua",
] as const;
const SHIKI_LANG_SET = new Set<string>(SHIKI_LANGS);
const SHIKI_LANG_ALIASES: Record<string, string> = {
  js: "javascript",
  ts: "typescript",
  sh: "bash",
  zsh: "bash",
  ps1: "powershell",
  yml: "yaml",
  golang: "go",
  py: "python",
  rs: "rust",
  rb: "ruby",
  cs: "csharp",
  "c++": "cpp",
  "c#": "csharp",
  docker: "dockerfile",
  md: "markdown",
};
type ShikiTheme = "github-light" | "github-dark";
let shikiHighlighter: Highlighter | null = null;
let shikiLoading: Promise<Highlighter> | null = null;
const shikiCache = new Map<string, string>();

function currentShikiTheme(): ShikiTheme {
  return document.documentElement.classList.contains("theme-dark")
    ? "github-dark"
    : "github-light";
}

function shikiCacheKey(source: string, lang: string, theme: ShikiTheme): string {
  return `${theme}\n${lang}\n${source}`;
}

function normalizeShikiLang(lang: string): string {
  const lower = lang.toLowerCase();
  if (SHIKI_LANG_SET.has(lower)) return lower;
  const alias = SHIKI_LANG_ALIASES[lower];
  if (alias) return alias;
  return "plaintext";
}

async function loadShiki(): Promise<Highlighter> {
  if (shikiHighlighter) return shikiHighlighter;
  if (shikiLoading) return shikiLoading;
  shikiLoading = createHighlighter({
    themes: ["github-light", "github-dark"],
    langs: [...SHIKI_LANGS],
  }).then((h) => {
    shikiHighlighter = h;
    shikiLoading = null;
    return h;
  });
  return shikiLoading;
}

const ALERT_TYPES = ["note", "tip", "important", "warning", "caution"] as const;
type AlertType = (typeof ALERT_TYPES)[number];
const ALERT_ICONS: Record<AlertType, typeof Info> = {
  note: Info,
  tip: Lightbulb,
  important: AlertCircle,
  warning: AlertTriangle,
  caution: ShieldAlert,
};

function isAlertType(value: string): value is AlertType {
  return (ALERT_TYPES as readonly string[]).includes(value);
}

/**
 * PindocMarkdown wraps react-markdown with the set of renderers Pindoc's
 * PINDOC.md promises agents can rely on. Today: GFM (tables, task lists,
 * strikethrough, autolinks), GitHub-style alerts (`> [!NOTE]` etc.),
 * soft line breaks, syntax highlighting via Shiki, and Mermaid diagrams
 * via fenced ```mermaid blocks. Agents inspecting pindoc.project.current
 * see this same list under `rendering.markdown` so they never render
 * something we don't display.
 */
export function PindocMarkdown({
  source,
  projectSlug,
  orgSlug,
  collapseStructureSections = false,
}: {
  source: string;
  projectSlug?: string;
  orgSlug?: string;
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
          orgSlug={orgSlug}
        />
      ))}
      {hiddenSections.length > 0 && (
        <OriginalStructureSections
          sections={hiddenSections}
          projectSlug={projectSlug}
          orgSlug={orgSlug}
        />
      )}
    </>
  );
}

function MarkdownBlock({
  source,
  headingSlugs,
  projectSlug,
  orgSlug,
}: {
  source: string;
  headingSlugs: string[];
  projectSlug?: string;
  orgSlug?: string;
}) {
  let headingIndex = 0;

  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm, remarkBreaks, remarkGithubAlerts]}
      urlTransform={(url) => pindocUrlTransform(url, projectSlug, orgSlug)}
      components={{
        pre: MarkdownPre,
        code: MarkdownCode,
        blockquote: MarkdownBlockquote,
        h2({ children }) {
          // extractHeadingText flattens children (often an array of
          // strings + inline code + emphasis) back into the plain text
          // used for slug generation. Keeping the rendered children
          // unchanged preserves inline formatting on the heading itself.
          const text = extractHeadingText(children);
          const id = headingSlugs[headingIndex++] ?? slugifyHeading(text);
          return (
            <h2 id={id}>
              <HeadingAnchor id={id} />
              {children}
            </h2>
          );
        },
        h3({ children }) {
          const id = slugifyHeading(extractHeadingText(children));
          return (
            <h3 id={id}>
              <HeadingAnchor id={id} />
              {children}
            </h3>
          );
        },
        h4({ children }) {
          const id = slugifyHeading(extractHeadingText(children));
          return (
            <h4 id={id}>
              <HeadingAnchor id={id} />
              {children}
            </h4>
          );
        },
      }}
    >
      {source}
    </ReactMarkdown>
  );
}

function HeadingAnchor({ id }: { id: string }) {
  if (!id) return null;
  return (
    <a className="heading-anchor" href={`#${id}`} aria-hidden="true" tabIndex={-1}>
      #
    </a>
  );
}

function MarkdownPre({ children }: { children?: ReactNode }) {
  const fence = fenceContent(children);
  if (fence?.lang === "mermaid") {
    return <MermaidBlock source={fence.code} />;
  }
  if (fence) {
    return <ShikiCodeBlock code={fence.code} lang={fence.lang} />;
  }
  return <pre>{children}</pre>;
}

function MarkdownCode({
  className,
  children,
}: {
  className?: string;
  children?: ReactNode;
}) {
  return <code className={className}>{children}</code>;
}

function MarkdownBlockquote({
  className,
  children,
}: {
  className?: string;
  children?: ReactNode;
}) {
  const { t } = useI18n();
  const cls = typeof className === "string" ? className : "";
  const m = /markdown-alert-(\w+)/.exec(cls);
  if (!m || !isAlertType(m[1])) {
    return <blockquote className={cls || undefined}>{children}</blockquote>;
  }
  const type = m[1];
  const Icon = ALERT_ICONS[type];
  return (
    <blockquote className={cls} data-alert={type}>
      <p className="markdown-alert-title">
        <Icon className="lucide" aria-hidden="true" />
        <span>{t(`reader.alert_${type}`)}</span>
      </p>
      {children}
    </blockquote>
  );
}

function fenceContent(children: unknown): { code: string; lang: string } | null {
  const child = Array.isArray(children) && children.length === 1 ? children[0] : children;
  if (!isValidElement(child)) return null;
  const props = child.props as { className?: unknown; children?: unknown };
  const className = typeof props.className === "string" ? props.className : "";
  const langMatch = /(?:^|\s)language-([\w-]+)(?:\s|$)/.exec(className);
  return {
    code: String(props.children ?? "").replace(/\n$/, ""),
    lang: langMatch?.[1] ?? "",
  };
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
  orgSlug,
}: {
  sections: StructureOverlapSection[];
  projectSlug?: string;
  orgSlug?: string;
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
              <MarkdownBlock source={section.body} headingSlugs={[]} projectSlug={projectSlug} orgSlug={orgSlug} />
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

function ShikiCodeBlock({ code, lang }: { code: string; lang: string }) {
  const theme = currentShikiTheme();
  const normalizedLang = normalizeShikiLang(lang);
  const cacheKey = shikiCacheKey(code, normalizedLang, theme);
  const [html, setHtml] = useState<string>(() => shikiCache.get(cacheKey) ?? "");

  useEffect(() => {
    let cancelled = false;
    const cached = shikiCache.get(cacheKey);
    if (cached) {
      setHtml(cached);
      return;
    }
    setHtml("");
    void loadShiki().then((h) => {
      if (cancelled) return;
      try {
        const rendered = h.codeToHtml(code, { lang: normalizedLang, theme });
        shikiCache.set(cacheKey, rendered);
        setHtml(rendered);
      } catch {
        try {
          const fallback = h.codeToHtml(code, { lang: "plaintext", theme });
          shikiCache.set(cacheKey, fallback);
          setHtml(fallback);
        } catch {
          // give up — leave the idle <pre> visible below
        }
      }
    });
    return () => {
      cancelled = true;
    };
  }, [cacheKey, code, normalizedLang, theme]);

  if (!html) {
    return (
      <pre className="shiki shiki--idle" data-lang={normalizedLang}>
        <code>{code}</code>
      </pre>
    );
  }
  return (
    <div
      className="shiki-block"
      data-lang={normalizedLang}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}

function MermaidBlock({ source }: { source: string }) {
  const { t } = useI18n();
  const theme = currentMermaidTheme();
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const dragRef = useRef<{
    pointerId: number;
    startX: number;
    startY: number;
    panX: number;
    panY: number;
  } | null>(null);
  const [state, setState] = useState<"idle" | "rendered" | "error">(
    () => (mermaidSvgCache.has(mermaidCacheKey(source, theme)) ? "rendered" : "idle"),
  );
  const [errMsg, setErrMsg] = useState<string>("");
  const [svg, setSvg] = useState<string>(() => mermaidSvgCache.get(mermaidCacheKey(source, theme)) ?? "");
  const [active, setActive] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [scale, setScale] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const renderTheme = ensureMermaid();
      const cacheKey = mermaidCacheKey(source, renderTheme);
      const cached = mermaidSvgCache.get(cacheKey);
      setState(cached ? "rendered" : "idle");
      setErrMsg("");
      setSvg(cached ?? "");
      try {
        await queueMermaidRender(async () => {
          if (cancelled) return;
          const id = `pindoc-mermaid-${Date.now()}-${++mermaidRenderCounter}`;
          const rendered = await mermaid.render(id, source);
          mermaidSvgCache.set(cacheKey, rendered.svg);
          if (!cancelled) {
            setSvg(rendered.svg);
          }
        });
        if (cancelled) return;
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
  }, [source, theme]);

  useEffect(() => {
    setActive(false);
    setScale(1);
    setPan({ x: 0, y: 0 });
    dragRef.current = null;
  }, [source]);

  useEffect(() => {
    if (!fullscreen) return;
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") setFullscreen(false);
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [fullscreen]);

  function resetView() {
    setScale(1);
    setPan({ x: 0, y: 0 });
  }

  function zoomAt(clientX: number, clientY: number, nextScale: number) {
    const viewport = viewportRef.current;
    const rect = viewport?.getBoundingClientRect();
    const clamped = Math.min(4, Math.max(0.4, nextScale));
    if (!rect) {
      setScale(clamped);
      return;
    }
    const originX = clientX - rect.left;
    const originY = clientY - rect.top;
    const ratio = clamped / scale;
    setPan({
      x: originX - (originX - pan.x) * ratio,
      y: originY - (originY - pan.y) * ratio,
    });
    setScale(clamped);
  }

  function handleWheel(e: WheelEvent<HTMLDivElement>) {
    if (!active && !e.ctrlKey && !e.metaKey) return;
    e.preventDefault();
    setActive(true);
    const nextScale = scale * (e.deltaY < 0 ? 1.12 : 0.88);
    zoomAt(e.clientX, e.clientY, nextScale);
  }

  function handlePointerDown(e: PointerEvent<HTMLDivElement>) {
    if (e.button !== 0) return;
    setActive(true);
    dragRef.current = {
      pointerId: e.pointerId,
      startX: e.clientX,
      startY: e.clientY,
      panX: pan.x,
      panY: pan.y,
    };
    e.currentTarget.setPointerCapture(e.pointerId);
  }

  function handlePointerMove(e: PointerEvent<HTMLDivElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== e.pointerId) return;
    setPan({
      x: drag.panX + e.clientX - drag.startX,
      y: drag.panY + e.clientY - drag.startY,
    });
  }

  function stopDrag(e: PointerEvent<HTMLDivElement>) {
    const drag = dragRef.current;
    if (drag?.pointerId === e.pointerId) {
      dragRef.current = null;
      e.currentTarget.releasePointerCapture(e.pointerId);
    }
  }

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

  const rootClass = [
    "mermaid-diagram",
    active ? "is-active" : "",
    fullscreen ? "is-fullscreen" : "",
  ].filter(Boolean).join(" ");

  return (
    <figure className={rootClass}>
      <div className="mermaid-diagram__toolbar" aria-label={t("reader.mermaid_toolbar")}>
        <button
          type="button"
          aria-label={t("reader.mermaid_zoom_in")}
          title={t("reader.mermaid_zoom_in")}
          onClick={() => {
            setActive(true);
            zoomAt(
              (viewportRef.current?.getBoundingClientRect().left ?? 0) +
                (viewportRef.current?.clientWidth ?? 0) / 2,
              (viewportRef.current?.getBoundingClientRect().top ?? 0) +
                (viewportRef.current?.clientHeight ?? 0) / 2,
              scale * 1.2,
            );
          }}
        >
          <ZoomIn className="lucide" aria-hidden="true" />
        </button>
        <button
          type="button"
          aria-label={t("reader.mermaid_zoom_out")}
          title={t("reader.mermaid_zoom_out")}
          onClick={() => {
            setActive(true);
            zoomAt(
              (viewportRef.current?.getBoundingClientRect().left ?? 0) +
                (viewportRef.current?.clientWidth ?? 0) / 2,
              (viewportRef.current?.getBoundingClientRect().top ?? 0) +
                (viewportRef.current?.clientHeight ?? 0) / 2,
              scale * 0.8,
            );
          }}
        >
          <ZoomOut className="lucide" aria-hidden="true" />
        </button>
        <button type="button" aria-label={t("reader.mermaid_reset")} title={t("reader.mermaid_reset")} onClick={resetView}>
          <RotateCcw className="lucide" aria-hidden="true" />
        </button>
        <button
          type="button"
          aria-label={fullscreen ? t("reader.mermaid_exit_fullscreen") : t("reader.mermaid_fullscreen")}
          title={fullscreen ? t("reader.mermaid_exit_fullscreen") : t("reader.mermaid_fullscreen")}
          onClick={() => setFullscreen((v) => !v)}
        >
          {fullscreen ? (
            <Minimize2 className="lucide" aria-hidden="true" />
          ) : (
            <Maximize2 className="lucide" aria-hidden="true" />
          )}
        </button>
      </div>
      <div
        ref={viewportRef}
        className="mermaid-diagram__viewport"
        tabIndex={0}
        aria-label={t("reader.mermaid_viewport")}
        onClick={() => setActive(true)}
        onWheel={handleWheel}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={stopDrag}
        onPointerCancel={stopDrag}
      >
        {state === "idle" && <div className="mermaid-diagram__status">{t("reader.mermaid_rendering")}</div>}
        <div
          className="mermaid-diagram__canvas"
          style={{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${scale})` }}
          dangerouslySetInnerHTML={{ __html: svg }}
        />
      </div>
    </figure>
  );
}
