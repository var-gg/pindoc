import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { I18nProvider } from "../src/i18n";
import { CmdK } from "../src/reader/CmdK";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function renderPalette(projectLang = "en"): string {
  return renderToStaticMarkup(
    <I18nProvider projectLang={projectLang}>
      <MemoryRouter>
        <CmdK projectSlug="pindoc" orgSlug="default" open={true} onClose={() => undefined} />
      </MemoryRouter>
    </I18nProvider>,
  );
}

function testCmdKDialogHasAccessibleName(): void {
  const html = renderPalette();

  assert(html.includes('role="dialog"'), "CmdK should render a dialog role");
  assert(html.includes('aria-modal="true"'), "CmdK dialog should be modal");
  assert(html.includes('aria-labelledby="cmdk-title"'), "CmdK dialog should reference its title");
  assert(html.includes('id="cmdk-title"'), "CmdK dialog title id should be stable");
  assert(html.includes("Command palette"), "CmdK dialog should render localized accessible title");
}

function testCmdKInputUsesComboboxPattern(): void {
  const html = renderPalette();

  assert(html.includes('id="cmdk-input"'), "CmdK input id should be stable");
  assert(html.includes('role="combobox"'), "CmdK input should use combobox role");
  assert(html.includes('aria-expanded="false"'), "empty CmdK results should set aria-expanded=false");
  assert(html.includes('aria-controls="cmdk-listbox"'), "CmdK input should control the listbox");
  assert(html.includes('aria-autocomplete="list"'), "CmdK input should declare list autocomplete");
}

function testCmdKResultsUseListboxPattern(): void {
  const html = renderPalette();

  assert(html.includes('id="cmdk-listbox"'), "CmdK listbox id should be stable");
  assert(html.includes('role="listbox"'), "CmdK results should render a listbox role");
  assert(html.includes('aria-label="Command palette results"'), "CmdK listbox should have a localized label");
  assert(html.includes("Search artifacts or a commit SHA in this project."), "empty CmdK should show the initial hint");
}

testCmdKDialogHasAccessibleName();
testCmdKInputUsesComboboxPattern();
testCmdKResultsUseListboxPattern();
