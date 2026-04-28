import {
  canExtendInvite,
  canIssuePermanentInvite,
  expiresPolicyHours,
  inviteAuditKindForExtend,
  inviteAuditKindForIssue,
  inviteIssuePayload,
} from "../src/project/invitePolicy";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testExpiryOptionsMapToContract(): void {
  assertEqual(expiresPolicyHours("1d"), 24, "1 day option");
  assertEqual(expiresPolicyHours("7d"), 168, "7 day option");
  assertEqual(expiresPolicyHours("30d"), 720, "30 day default option");
  assertEqual(expiresPolicyHours("permanent"), undefined, "permanent option");
  assertEqual(inviteIssuePayload("viewer", "30d").expires_in_hours, 720, "payload keeps 30d default");
  assertEqual(inviteIssuePayload("editor", "permanent").expires_policy, "permanent", "payload marks permanent");
}

function testOwnerGatesPermanentAndExtend(): void {
  assert(canIssuePermanentInvite({ current_role: "owner" }), "owner can issue permanent");
  assert(!canIssuePermanentInvite({ current_role: "editor" }), "editor cannot issue permanent");
  assert(canExtendInvite({ current_role: "owner" }, "2026-05-01T00:00:00Z"), "owner can extend expiring invite");
  assert(!canExtendInvite({ current_role: "viewer" }, "2026-05-01T00:00:00Z"), "viewer cannot extend");
  assert(!canExtendInvite({ current_role: "owner" }, null), "permanent invite is not shortened");
}

function testAuditKindsAreStable(): void {
  assertEqual(inviteAuditKindForIssue("permanent"), "invite.permanent_issued", "permanent issue audit kind");
  assertEqual(inviteAuditKindForIssue("30d"), null, "normal issue has no permanent audit kind");
  assertEqual(inviteAuditKindForExtend(), "invite.extended", "extend audit kind");
}

testExpiryOptionsMapToContract();
testOwnerGatesPermanentAndExtend();
testAuditKindsAreStable();
