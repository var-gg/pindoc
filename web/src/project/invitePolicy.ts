import type { InviteExpiryPolicy, InviteExtendTo, InviteIssueInput, InviteRole, Project } from "../api/client";

export const INVITE_EXPIRY_OPTIONS: InviteExpiryPolicy[] = ["1d", "7d", "30d", "permanent"];
export const INVITE_EXTEND_OPTIONS: InviteExtendTo[] = ["+7d", "+30d", "permanent"];

export function canIssuePermanentInvite(project: Pick<Project, "current_role">): boolean {
  return project.current_role === "owner";
}

export function canExtendInvite(project: Pick<Project, "current_role">, expiresAt: string | null): boolean {
  return project.current_role === "owner" && expiresAt !== null;
}

export function inviteIssuePayload(
  role: InviteRole,
  expiresPolicy: InviteExpiryPolicy,
): InviteIssueInput {
  return {
    role,
    expires_policy: expiresPolicy,
    expires_in_hours: expiresPolicyHours(expiresPolicy),
  };
}

export function expiresPolicyHours(policy: InviteExpiryPolicy): number | undefined {
  switch (policy) {
    case "1d":
      return 24;
    case "7d":
      return 168;
    case "30d":
      return 720;
    case "permanent":
      return undefined;
  }
}

export function inviteAuditKindForIssue(policy: InviteExpiryPolicy): string | null {
  return policy === "permanent" ? "invite.permanent_issued" : null;
}

export function inviteAuditKindForExtend(): string {
  return "invite.extended";
}
