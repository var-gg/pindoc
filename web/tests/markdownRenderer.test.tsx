import { renderToStaticMarkup } from "react-dom/server";
import { I18nProvider } from "../src/i18n";
import { decorateDiffHtml, parseCodeMeta, PindocMarkdown } from "../src/reader/Markdown";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function renderMarkdown(source: string): string {
  return renderToStaticMarkup(
    <I18nProvider projectLang="en">
      <div className="art-body">
        <PindocMarkdown source={source} projectSlug="pindoc" />
      </div>
    </I18nProvider>,
  );
}

function testSafeKbdOnlyUpgradesPlainKbd(): void {
  const html = renderMarkdown('Shortcut <kbd>Ctrl</kbd> and <kbd onclick="bad">Alt</kbd>.');

  assert(html.includes("<kbd>Ctrl</kbd>"), "plain kbd HTML should become a safe keyboard element");
  assert(!/<kbd[^>]*onclick/i.test(html), "kbd with attributes must not become an HTML element");
}

function testCodeBlockToolbarRendersTitleLanguageAndCopy(): void {
  const html = renderMarkdown('~~~ts title="web/src/reader/Markdown.tsx"\nconst value = 1;\n~~~');

  assert(html.includes("code-block__copy"), "fenced code blocks should render a copy button");
  assert(html.includes("web/src/reader/Markdown.tsx"), "code block title metadata should render");
  assert(html.includes("typescript"), "language alias should render as the normalized language");
}

function testLongCodeBlockStartsCollapsed(): void {
  const code = Array.from({ length: 52 }, (_, i) => `line ${i}`).join("\n");
  const html = renderMarkdown(`~~~text\n${code}\n~~~`);

  assert(html.includes("is-collapsed"), "long code blocks should start collapsed");
  assert(html.includes("Show full code"), "long code blocks should expose an expand affordance");
}

function testTableUsesScrollWrapper(): void {
  const html = renderMarkdown("| Column A | Column B |\n| --- | --- |\n| long-long-long-long-long | value |");

  assert(html.includes("pindoc-table-scroll"), "GFM tables should render inside the scroll wrapper");
}

function testDiffDecoratorAddsLineClasses(): void {
  const html = decorateDiffHtml(
    '<span class="line"></span><span class="line"></span><span class="line"></span>',
    "+added\n-removed\n@@ hunk",
    "diff",
  );

  assert(html.includes("pindoc-diff-line--add"), "diff add line should get add class");
  assert(html.includes("pindoc-diff-line--remove"), "diff remove line should get remove class");
  assert(html.includes("pindoc-diff-line--hunk"), "diff hunk line should get hunk class");
}

function testCodeMetaParser(): void {
  assertEqual(
    parseCodeMeta('title="src/app.ts" showLineNumbers').title,
    "src/app.ts",
    "quoted title should parse",
  );
  assert(parseCodeMeta("filename=README.md linenums").showLineNumbers, "line number aliases should parse");
}

testSafeKbdOnlyUpgradesPlainKbd();
testCodeBlockToolbarRendersTitleLanguageAndCopy();
testLongCodeBlockStartsCollapsed();
testTableUsesScrollWrapper();
testDiffDecoratorAddsLineClasses();
testCodeMetaParser();
