import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { I18nProvider } from "../src/i18n";
import en from "../src/i18n/en.json";
import ko from "../src/i18n/ko.json";
import { ConfirmDialog } from "../src/components/ConfirmDialog";
import { copyTextToClipboard, NewClientSecretReveal, ProvidersPanel } from "../src/admin/ProvidersPanel";

const enCopy = en as Record<string, string>;
const koCopy = ko as Record<string, string>;

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function renderWithLang(node: ReactNode, lang: "ko" | "en"): string {
  return renderToStaticMarkup(
    <I18nProvider projectLang={lang}>
      <MemoryRouter>
        {node}
      </MemoryRouter>
    </I18nProvider>,
  );
}

function testProvidersPanelUsesLocalizedCopy(): void {
  const koHtml = renderWithLang(<ProvidersPanel />, "ko");
  const enHtml = renderWithLang(<ProvidersPanel />, "en");

  assert(koHtml.includes(koCopy["admin.providers.title"]), "KO providers title should render");
  assert(koHtml.includes(koCopy["admin.oauth_clients.title"]), "KO OAuth clients title should render");
  assert(koHtml.includes(koCopy["admin.oauth_clients.register"]), "KO register button should render");
  assert(enHtml.includes(enCopy["admin.providers.title"]), "EN providers title should render");
  assert(enHtml.includes(enCopy["admin.oauth_clients.title"]), "EN OAuth clients title should render");
}

function testSecretRevealRequiresExplicitDismissAndCopiesSecret(): void {
  const html = renderWithLang(
    <NewClientSecretReveal secret="secret-value" onDismiss={() => undefined} />,
    "en",
  );

  assert(html.includes('role="alert"'), "secret reveal should be an alert/sticky warning");
  assert(html.includes("secret-value"), "secret value should render in the reveal area");
  assert(html.includes(enCopy["admin.oauth_clients.copy_secret"]), "copy button should render");
  assert(html.includes(enCopy["admin.oauth_clients.secret_saved_dismiss"]), "explicit dismiss button should render");
}

async function testClipboardHelperCallsClipboardWriter(): Promise<void> {
  let written = "";
  const ok = await copyTextToClipboard("secret-value", {
    writeText: async (value: string) => {
      written = value;
    },
  });

  assert(ok, "copy helper should report success");
  assert(written === "secret-value", "copy helper should call clipboard.writeText with the secret");
}

function testConfirmDialogRendersStyledDialogAndCheckbox(): void {
  const html = renderWithLang(
    <ConfirmDialog
      open={true}
      title="Delete OAuth client client_123?"
      body={<p>Delete body</p>}
      confirmLabel="Delete client"
      cancelLabel="Cancel"
      onConfirm={() => undefined}
      onCancel={() => undefined}
      checkbox={{
        checked: true,
        label: "Suppress reseed",
        onChange: () => undefined,
      }}
    />,
    "en",
  );

  assert(html.includes('role="dialog"'), "confirm dialog should render as a dialog");
  assert(html.includes('aria-modal="true"'), "confirm dialog should be modal");
  assert(html.includes('type="checkbox"'), "env_seed confirm should include checkbox");
}

testProvidersPanelUsesLocalizedCopy();
testSecretRevealRequiresExplicitDismissAndCopiesSecret();
await testClipboardHelperCallsClipboardWriter();
testConfirmDialogRendersStyledDialogAndCheckbox();
