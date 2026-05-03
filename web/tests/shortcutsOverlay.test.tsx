import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { I18nProvider } from "../src/i18n";
import en from "../src/i18n/en.json";
import ko from "../src/i18n/ko.json";
import { ShortcutsOverlay, shortcutsTrapTabIndex } from "../src/reader/ShortcutsOverlay";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function renderOverlay(projectLang = "en"): string {
  return renderToStaticMarkup(
    <I18nProvider projectLang={projectLang}>
      <MemoryRouter>
        <ShortcutsOverlay
          open={true}
          view="reader"
          projectSlug="pindoc"
          orgSlug="default"
          detail={null}
          selectedArea={null}
          selectedType={null}
          badgeFilters={[]}
          areaNameBySlug={new Map()}
          onClose={() => undefined}
        />
      </MemoryRouter>
    </I18nProvider>,
  );
}

function testKoreanCopyDoesNotExposeRawShortcutLabels(): void {
  const html = renderOverlay("ko");

  assert(html.includes("단축키"), "KO overlay should localize title");
  assert(html.includes("공통"), "KO overlay should localize global group");
  assert(html.includes("기호"), "KO overlay should localize symbols heading");
  assert(html.includes("종류 배지"), "KO overlay should localize symbol labels");
  assert(html.includes("상단 도움말 버튼"), "KO overlay should render the HelpPopover hint");
  assert(!html.includes(">Shortcuts<"), "KO overlay should not show EN title");
  assert(!html.includes(">Global<"), "KO overlay should not show EN group");
  assert(!html.includes(">Symbols<"), "KO overlay should not show EN symbols heading");
  assert(!html.includes(">Type badge<"), "KO overlay should not show EN symbol label");
}

function testShortcutHelpKeysExistInBothLocales(): void {
  for (const key of ["help.shortcuts_link", "shortcuts.help_hint"]) {
    assert(Boolean((en as Record<string, string>)[key]), `missing EN key: ${key}`);
    assert(Boolean((ko as Record<string, string>)[key]), `missing KO key: ${key}`);
  }
}

function testDialogAccessibilityMarkup(): void {
  const html = renderOverlay();

  assert(html.includes('role="dialog"'), "ShortcutsOverlay should render a dialog role");
  assert(html.includes('aria-modal="true"'), "ShortcutsOverlay should be modal");
  assert(html.includes('aria-labelledby="shortcuts-title"'), "dialog should reference title");
  assert(html.includes('aria-label="Close shortcuts overlay"'), "close button should be named");
}

function testFocusTrapWrapsAtEdges(): void {
  assertEqual(shortcutsTrapTabIndex(0, 3, true), 2, "Shift+Tab on first control should wrap to last");
  assertEqual(shortcutsTrapTabIndex(2, 3, false), 0, "Tab on last control should wrap to first");
  assertEqual(shortcutsTrapTabIndex(1, 3, false), null, "middle control should keep native Tab");
  assertEqual(shortcutsTrapTabIndex(-1, 3, false), 0, "outside focus should re-enter first control");
}

testKoreanCopyDoesNotExposeRawShortcutLabels();
testShortcutHelpKeysExistInBothLocales();
testDialogAccessibilityMarkup();
testFocusTrapWrapsAtEdges();
