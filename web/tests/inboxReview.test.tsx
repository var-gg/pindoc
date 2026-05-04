import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { api, buildInboxReviewBody } from "../src/api/client";
import { I18nProvider } from "../src/i18n";
import en from "../src/i18n/en.json";
import ko from "../src/i18n/ko.json";
import { Inbox, inboxCardA11yPosition, nextInboxFocusIndex, refreshInboxAfterReview } from "../src/reader/Inbox";
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
    "inbox.review_refresh_warning",
    "inbox.reject_reason_required",
    "inbox.truncated_notice",
    "inbox.list_aria",
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
    JSON.stringify(buildInboxReviewBody("reject", { commitMsg: "  duplicate " })),
    JSON.stringify({ decision: "reject", commit_msg: "duplicate" }),
    "review body should include trimmed reason and leave reviewer to the server",
  );
  assertEqual(
    JSON.stringify(buildInboxReviewBody("approve", { commitMsg: "" })),
    JSON.stringify({ decision: "approve", commit_msg: "" }),
    "approve body should preserve empty commit message for backward compatibility",
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
      commitMsg: "wrong scope",
    });
  } finally {
    globalThis.fetch = originalFetch;
  }

  assertEqual(path, "/api/p/pindoc/inbox/artifact%20one/review", "review endpoint should encode slug");
  assertEqual(init?.method, "POST", "review method should be POST");
  assertEqual(
    String(init?.body),
    JSON.stringify({ decision: "reject", commit_msg: "wrong scope" }),
    "review fetch body should carry commit_msg and omit caller-controlled reviewer_id",
  );
}

function testRovingFocusWraps(): void {
  assertEqual(nextInboxFocusIndex(0, 3, -1), 2, "ArrowUp from first card should wrap to last");
  assertEqual(nextInboxFocusIndex(2, 3, 1), 0, "ArrowDown from last card should wrap to first");
  assertEqual(nextInboxFocusIndex(0, 0, 1), 0, "empty list should keep focus index at zero");
}

function testInboxCardA11yPosition(): void {
  assertEqual(inboxCardA11yPosition(1, 5)["aria-posinset"], 2, "card posinset should be 1-based");
  assertEqual(inboxCardA11yPosition(1, 5)["aria-setsize"], 5, "card setsize should expose total count");
}

async function testReviewSuccessRefreshesOnce(): Promise<void> {
  let calls = 0;
  const ok = await refreshInboxAfterReview(async () => {
    calls += 1;
    return true;
  });
  assert(ok, "refresh helper should return load result");
  assertEqual(calls, 1, "review success refresh should call load once");
}

testDisabledOwnerGetsSettingsCTA();
testDisabledNonOwnerGetsAdminHint();
testInboxI18nKeysExist();
testReviewBodyUsesAuditInputsAndFallbacks();
await testApiInboxReviewSendsAuditBody();
testRovingFocusWraps();
testInboxCardA11yPosition();
await testReviewSuccessRefreshesOnce();
