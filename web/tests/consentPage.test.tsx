import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { I18nProvider } from "../src/i18n";
import type { OAuthConsentInfo } from "../src/api/client";
import en from "../src/i18n/en.json";
import ko from "../src/i18n/ko.json";
import { ConsentGrantPanel, ConsentPage, OAuthLoadingStatus } from "../src/oauth/ConsentPage";

const enCopy = en as Record<string, string>;
const koCopy = ko as Record<string, string>;

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function renderWithLang(node: ReactNode, lang: "ko" | "en" = "en"): string {
  return renderToStaticMarkup(
    <I18nProvider projectLang={lang}>
      {node}
    </I18nProvider>,
  );
}

const baseInfo: OAuthConsentInfo = {
  client_id: "client_test",
  client_display_name: "Cursor",
  scopes: ["pindoc"],
  already_granted: false,
  consent_nonce: "nonce-123",
  created_via: "dcr",
  created_at: new Date().toISOString(),
  redirect_uris: ["http://127.0.0.1:3846/callback"],
};

function countMatches(haystack: string, needle: string): number {
  return haystack.split(needle).length - 1;
}

function testConsentPageLoadingStatusIsAnnounced(): void {
  const pageHtml = renderWithLang(<ConsentPage />);
  const statusHtml = renderWithLang(<OAuthLoadingStatus label={enCopy["oauth.consent.loading"]} />);

  assert(pageHtml.includes(enCopy["oauth.consent.title"]), "consent page should use localized title");
  assert(statusHtml.includes('role="status"'), "loading status should expose role=status");
  assert(statusHtml.includes('aria-live="polite"'), "loading status should use polite live region");
  assert(statusHtml.includes(enCopy["oauth.consent.loading"]), "loading status should render localized copy");
}

function testConsentPanelUsesLocalizedCopy(): void {
  const koHtml = renderWithLang(
    <ConsentGrantPanel info={baseInfo} query="?client_id=client_test" />,
    "ko",
  );
  const enHtml = renderWithLang(
    <ConsentGrantPanel info={baseInfo} query="?client_id=client_test" />,
    "en",
  );

  assert(koHtml.includes(koCopy["oauth.consent.client_id"]), "KO client ID label should render");
  assert(koHtml.includes(koCopy["oauth.consent.approve"]), "KO approve button should render");
  assert(koHtml.includes(koCopy["oauth.scope.pindoc.title"]), "KO scope catalog title should render");
  assert(koHtml.includes(koCopy["oauth.consent.trust.dcr.label"]), "KO DCR trust label should render");
  assert(enHtml.includes(enCopy["oauth.consent.client_id"]), "EN client ID label should render");
  assert(enHtml.includes(enCopy["oauth.consent.trust.dcr.label"]), "EN DCR trust label should render");
}

function testScopeListAndUnknownFallback(): void {
  const html = renderWithLang(
    <ConsentGrantPanel
      info={{ ...baseInfo, scopes: ["pindoc", "offline_access", "mcp.read"] }}
      query="?client_id=client_test"
    />,
  );

  assert(countMatches(html, "<li>") === 3, "three requested scopes should render as list items");
  assert(html.includes(enCopy["oauth.scope.offline_access.title"]), "offline_access title should render");
  assert(html.includes("mcp.read"), "unknown scope name should render");
  assert(
    html.includes("Pindoc does not have a catalog description for mcp.read yet."),
    "unknown scope fallback description should render",
  );
}

function testNoScopesCopy(): void {
  const html = renderWithLang(
    <ConsentGrantPanel info={{ ...baseInfo, scopes: [] }} query="?client_id=client_test" />,
  );

  assert(html.includes(enCopy["oauth.consent.no_scopes"]), "no-scope copy should render");
}

function testSingleFormAndPendingButtons(): void {
  const html = renderWithLang(
    <ConsentGrantPanel
      info={baseInfo}
      query="?client_id=client_test"
      submitting={true}
    />,
  );

  assert(countMatches(html, "<form") === 1, "consent actions should use a single form");
  assert(countMatches(html, 'name="action"') === 2, "approve and deny should share action field name");
  assert(html.includes('value="approve"'), "approve button value should render");
  assert(html.includes('value="deny"'), "deny button value should render");
  assert(html.includes('name="consent_nonce"'), "single-use consent nonce should post with the form");
  assert(countMatches(html, "disabled=\"\"") === 2, "both action buttons should disable while pending");
  assert(html.includes(enCopy["oauth.consent.submitting"]), "pending label should render");
}

function testTrustSignalShowsRedirectHostAndRecentRegistration(): void {
  const html = renderWithLang(
    <ConsentGrantPanel info={baseInfo} query="?client_id=client_test" />,
  );

  assert(html.includes("127.0.0.1:3846"), "redirect host should render");
  assert(html.includes("Registered within the last"), "recent registration trust signal should render");
}

testConsentPageLoadingStatusIsAnnounced();
testConsentPanelUsesLocalizedCopy();
testScopeListAndUnknownFallback();
testNoScopesCopy();
testSingleFormAndPendingButtons();
testTrustSignalShowsRedirectHostAndRecentRegistration();
