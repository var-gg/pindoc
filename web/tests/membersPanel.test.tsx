import { renderToStaticMarkup } from "react-dom/server";
import type { MemberRow } from "../src/api/client";
import {
  formatMembersPanelError,
  getMemberActionState,
  MembersSection,
} from "../src/project/MembersPanel";
import en from "../src/i18n/en.json";
import ko from "../src/i18n/ko.json";

const enCopy = en as Record<string, string>;
const koCopy = ko as Record<string, string>;

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function tFrom(copy: Record<string, string>) {
  return (key: string, ...args: Array<string | number>): string => {
    let value = copy[key] ?? key;
    for (const arg of args) {
      value = value.replace(/%[sd]/, String(arg));
    }
    return value;
  };
}

function member(userID: string, role: MemberRow["role"], isSelf = false): MemberRow {
  return {
    user_id: userID,
    display_name: userID === "u1" ? "Curious Store" : "Teammate",
    role,
    joined_at: "2026-05-03T00:00:00Z",
    is_self: isSelf,
  };
}

function renderMembers(members: MemberRow[], lang: "ko" | "en"): string {
  return renderToStaticMarkup(
    <MembersSection
      members={members}
      viewerRole="owner"
      viewerId="u1"
      usersByID={new Map()}
      pendingAction={null}
      onRemove={() => undefined}
      t={tFrom(lang === "ko" ? koCopy : enCopy)}
      lang={lang}
    />,
  );
}

function testSoleOwnerSelfLeaveRendersDisabledGuard(): void {
  const members = [member("u1", "owner", true), member("u2", "viewer")];
  const state = getMemberActionState({
    member: members[0],
    members,
    viewerRole: "owner",
    viewerId: "u1",
  });
  const html = renderMembers(members, "ko");

  assert(state.isLastOwnerSelfLeave, "sole owner self-leave should be marked as last-owner guarded");
  assert(!state.requiresConfirm, "disabled last-owner leave should not open the confirm branch");
  assert(html.includes('disabled=""'), "sole owner leave button should render disabled");
  assert(
    html.includes(koCopy["members_panel.last_owner_blocked"]),
    "sole owner leave button should include the localized guard tooltip",
  );
}

function testTwoOwnerSelfLeaveRequiresConfirmationAndStaysEnabled(): void {
  const members = [member("u1", "owner", true), member("u2", "owner")];
  const state = getMemberActionState({
    member: members[0],
    members,
    viewerRole: "owner",
    viewerId: "u1",
  });
  const html = renderMembers(members, "en");

  assert(!state.isLastOwnerSelfLeave, "two-owner self-leave should not be last-owner guarded");
  assert(state.requiresConfirm, "two-owner self-leave should require confirmation");
  assert(!html.includes('disabled=""'), "two-owner self-leave button should stay enabled");
  assert(
    !html.includes(enCopy["members_panel.last_owner_blocked"]),
    "two-owner self-leave button should not show the last-owner tooltip",
  );
}

function testLastOwnerErrorCodeMapsToLocalizedCopy(): void {
  const err = Object.assign(new Error("cannot remove the last owner"), {
    error_code: "LAST_OWNER",
  });

  assertEqual(
    formatMembersPanelError(tFrom(koCopy), err),
    koCopy["members_panel.error_last_owner"],
    "KO LAST_OWNER error should map to localized copy",
  );
  assertEqual(
    formatMembersPanelError(tFrom(enCopy), err),
    enCopy["members_panel.error_last_owner"],
    "EN LAST_OWNER error should map to localized copy",
  );
}

testSoleOwnerSelfLeaveRendersDisabledGuard();
testTwoOwnerSelfLeaveRequiresConfirmationAndStaysEnabled();
testLastOwnerErrorCodeMapsToLocalizedCopy();
