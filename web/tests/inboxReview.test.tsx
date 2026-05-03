import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { api, buildInboxReviewBody } from "../src/api/client";
import { I18nProvider } from "../src/i18n";
import en from "../src/i18n/en.json";
import ko from "../src/i18n/ko.json";
import { Inbox, nextInboxFocusIndex } from "../src/reader/Inbox";
import { PindocTooltipProvider } from "../src/reader/Tooltip";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function renderDisabledInbox(role: "owner" | "editor" | "viewer" | undefined, projectLang = "en"): string {
  return renderToStaticMarkup(
    <I18nProvider projectLang={projectLang}>
      <PindocTooltipProvider>
        <MemoryRouter>
          <Inbox
            projectSlug="pindoc"
            orgSlug="default"
            enabled={false}
            projectRole={role}
            onCountChange={() => undefined}
          />
        </MemoryRouter>
      </PindocTooltipProvider>
    </I18nProvider>,
  );
}

function testDisabledOwnerGetsSettingsCTA(): void {
  const html = renderDisabledInbox("owner", "en");
  assert(html.includes("Open project settings"), "owner disabled state should include settings CTA");
  assert(html.includes('href="/default/p/pindoc/settings"'), "settings CTA should target project settings");
}

function testDisabledNonOwnerGetsAdminHint(): void {
  const html = renderDisabledInbox("viewer", "en");
  assert(html.includes("Contact your admin"), "non-owner disabled state should include admin guidance");
  assert(!html.includes('href="/default/p/pindoc/settings"'), "non-owner disabled state should not link settings");
}

function testInboxI18nKeysExist(): void {
  const keys = [
    "inbox.disabled_empty_cta",
    "inbox.disabled_empty_admin",
    "inbox.error_load",
    "inbox.error_review",
    "inbox.approved_toast",
    "inbox.rejected_toast",
    "inbox.dialog_reject_title",
    "shortcuts.inbox_select.label",
    "shortcuts.inbox_reject.hint",
  ];
  const enCopy = en as Record<string, string>;
  const koCopy = ko as Record<string, string>;
  for (const key of keys) {
    assert(Boolean(enCopy[key]) && enCopy[key] !== key, `missing EN key: ${key}`);
    assert(Boolean(koCopy[key]) && koCopy[key] !== key, `missing KO key: ${key}`);
  }
}

function testReviewBodyUsesAuditInputsAndFallbacks(): void {
  assertEqual(
    JSON.stringify(buildInboxReviewBody("reject", { reviewerId: " user-1 ", commitMsg: "  duplicate " })),
    JSON.stringify({ decision: "reject", commit_msg: "duplicate", reviewer_id: "user-1" }),
    "review body should include trimmed reviewer and reason",
  );
  assertEqual(
    JSON.stringify(buildInboxReviewBody("approve", { reviewerId: "", commitMsg: "" })),
    JSON.stringify({ decision: "approve", commit_msg: "Reader Inbox approve" }),
    "review body should preserve fallback commit message and omit empty reviewer",
  );
}

async function testApiInboxReviewSendsAuditBody(): Promise<void> {
  const originalFetch = globalThis.fetch;
  let path = "";
  let init: RequestInit | undefined;
  globalThis.fetch = (async (input: RequestInfo | URL, requestInit?: RequestInit) => {
    path = String(input);
    init = requestInit;
    return new Response(JSON.stringify({
      status: "accepted",
      artifact_id: "artifact-1",
      slug: "artifact-one",
      review_state: "rejected",
      row_status: "archived",
    }), { status: 200, headers: { "Content-Type": "application/json" } });
  }) as typeof fetch;

  try {
    await api.inboxReview("pindoc", "artifact one", "reject", {
      reviewerId: "user-123",
      commitMsg: "wrong scope",
    });
  } finally {
    globalThis.fetch = originalFetch;
  }

  assertEqual(path, "/api/p/pindoc/inbox/artifact%20one/review", "review endpoint should encode slug");
  assertEqual(init?.method, "POST", "review method should be POST");
  assertEqual(
    String(init?.body),
    JSON.stringify({ decision: "reject", commit_msg: "wrong scope", reviewer_id: "user-123" }),
    "review fetch body should carry reviewer_id and commit_msg",
  );
}

function testRovingFocusWraps(): void {
  assertEqual(nextInboxFocusIndex(0, 3, -1), 2, "ArrowUp from first card should wrap to last");
  assertEqual(nextInboxFocusIndex(2, 3, 1), 0, "ArrowDown from last card should wrap to first");
  assertEqual(nextInboxFocusIndex(0, 0, 1), 0, "empty list should keep focus index at zero");
}

testDisabledOwnerGetsSettingsCTA();
testDisabledNonOwnerGetsAdminHint();
testInboxI18nKeysExist();
testReviewBodyUsesAuditInputsAndFallbacks();
await testApiInboxReviewSendsAuditBody();
testRovingFocusWraps();
