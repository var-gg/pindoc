import type { ArtifactRef, RecentWarning } from "../api/client";

export type BadgeFilterKey =
  | "source_type"
  | "verification_state"
  | "next_context_policy"
  | "confidence"
  | "audience"
  | "task_status"
  | "warning";

export type BadgeFilter = {
  key: BadgeFilterKey;
  value: string;
  label: string;
};

export type TFn = (key: string, ...args: Array<string | number>) => string;

export const BADGE_FILTER_KEYS: BadgeFilterKey[] = [
  "source_type",
  "verification_state",
  "next_context_policy",
  "confidence",
  "audience",
  "task_status",
  "warning",
];

export const BADGE_FILTER_PARAM_BY_KEY: Record<BadgeFilterKey, string> = {
  source_type: "source",
  verification_state: "verify",
  next_context_policy: "context",
  confidence: "confidence",
  audience: "audience",
  task_status: "task_status",
  warning: "warning",
};

export function readBadgeFilters(params: URLSearchParams, t: TFn): BadgeFilter[] {
  const filters: BadgeFilter[] = [];
  for (const key of BADGE_FILTER_KEYS) {
    const value = params.get(BADGE_FILTER_PARAM_BY_KEY[key]);
    if (!value) continue;
    filters.push({ key, value, label: badgeFilterValueLabel(key, value, t) });
  }
  return filters;
}

export function setBadgeFilterParam(params: URLSearchParams, filter: BadgeFilter): void {
  params.set(BADGE_FILTER_PARAM_BY_KEY[filter.key], filter.value);
}

export function clearBadgeFilterParam(params: URLSearchParams, key: BadgeFilterKey): void {
  params.delete(BADGE_FILTER_PARAM_BY_KEY[key]);
}

export function clearAllBadgeFilterParams(params: URLSearchParams): void {
  for (const key of BADGE_FILTER_KEYS) {
    clearBadgeFilterParam(params, key);
  }
}

export function appendBadgeFilters(params: URLSearchParams, filters: BadgeFilter[]): void {
  for (const filter of filters) {
    setBadgeFilterParam(params, filter);
  }
}

export function artifactMatchesBadgeFilters(a: ArtifactRef, filters: BadgeFilter[]): boolean {
  for (const filter of filters) {
    switch (filter.key) {
      case "source_type":
        if (a.artifact_meta?.source_type !== filter.value) return false;
        break;
      case "verification_state":
        if ((a.artifact_meta?.verification_state ?? "partially_verified") !== filter.value) return false;
        break;
      case "next_context_policy":
        if ((a.artifact_meta?.next_context_policy ?? "default") !== filter.value) return false;
        break;
      case "confidence":
        if (a.artifact_meta?.confidence !== filter.value) return false;
        break;
      case "audience":
        if (a.artifact_meta?.audience !== filter.value) return false;
        break;
      case "task_status":
        if ((a.task_meta?.status ?? "open") !== filter.value) return false;
        break;
      case "warning":
        if (!latestWarningCodes(a.recent_warnings).has(filter.value)) return false;
        break;
    }
  }
  return true;
}

export function badgeFilterKeyLabel(key: BadgeFilterKey, t: TFn): string {
  switch (key) {
    case "source_type":
      return t("reader.filter_key_source_type");
    case "verification_state":
      return t("reader.filter_key_verification");
    case "next_context_policy":
      return t("reader.filter_key_context");
    case "confidence":
      return t("reader.filter_key_confidence");
    case "audience":
      return t("reader.filter_key_audience");
    case "task_status":
      return t("reader.filter_key_task_status");
    case "warning":
      return t("reader.filter_key_warning");
  }
}

export function badgeFilterValueLabel(key: BadgeFilterKey, value: string, t: TFn): string {
  switch (key) {
    case "source_type":
      return sourceLabel(value, t);
    case "verification_state":
      return verificationLabel(value, t);
    case "next_context_policy":
      return contextPolicyLabel(value, t);
    case "confidence":
      return confidenceLabel(value, t);
    case "audience":
      return audienceLabel(value, t);
    case "task_status":
      return taskStatusLabel(value, t);
    case "warning":
      return warningLabel(value, t);
  }
}

function sourceLabel(value: string, t: TFn): string {
  switch (value) {
    case "code":
      return t("trust.source.code.label");
    case "artifact":
      return t("trust.source.artifact.label");
    case "user_chat":
      return t("trust.source.user_chat.label");
    case "external":
      return t("trust.source.external.label");
    case "mixed":
      return t("trust.source.mixed.label");
    default:
      return value;
  }
}

function verificationLabel(value: string, t: TFn): string {
  switch (value) {
    case "verified":
      return t("trust.verification.verified.label");
    case "partially_verified":
      return t("trust.verification.partially_verified.label");
    case "unverified":
      return t("trust.verification.unverified.label");
    default:
      return value;
  }
}

function contextPolicyLabel(value: string, t: TFn): string {
  switch (value) {
    case "default":
      return t("trust.next_context.default.label");
    case "opt_in":
      return t("trust.next_context.opt_in.label");
    case "excluded":
      return t("trust.next_context.excluded.label");
    default:
      return value;
  }
}

function confidenceLabel(value: string, t: TFn): string {
  switch (value) {
    case "low":
      return t("trust.confidence.low.label");
    case "medium":
      return t("reader.filter_value_confidence_medium");
    case "high":
      return t("reader.filter_value_confidence_high");
    default:
      return value;
  }
}

function audienceLabel(value: string, t: TFn): string {
  switch (value) {
    case "owner_only":
      return t("trust.audience.owner_only.label");
    case "approvers":
      return t("trust.audience.approvers.label");
    case "project_readers":
      return t("reader.filter_value_project_readers");
    default:
      return value;
  }
}

function taskStatusLabel(value: string, t: TFn): string {
  switch (value) {
    case "open":
      return t("reader.filter_value_task_open");
    case "claimed_done":
      return t("trust.task.claimed_done.label");
    case "blocked":
      return t("trust.task.blocked.label");
    case "cancelled":
      return t("trust.task.cancelled.label");
    default:
      return value;
  }
}

function warningLabel(value: string, t: TFn): string {
  switch (value) {
    case "CANONICAL_REWRITE_WITHOUT_EVIDENCE":
      return t("trust.warning.canonical_rewrite.label");
    case "CONSENT_REQUIRED_FOR_USER_CHAT":
      return t("trust.warning.consent_required.label");
    case "SOURCE_TYPE_UNCLASSIFIED":
      return t("trust.warning.source_unclassified.label");
    case "RECOMMEND_READ_BEFORE_CREATE":
      return t("trust.warning.near_duplicate.label");
    default:
      return value;
  }
}

function latestWarningCodes(recent: RecentWarning[] | undefined): Set<string> {
  if (!recent || recent.length === 0) return new Set();
  let latest = 0;
  for (const warning of recent) {
    if (warning.revision_number > latest) latest = warning.revision_number;
  }
  const codes = new Set<string>();
  for (const warning of recent) {
    if (warning.revision_number !== latest) continue;
    for (const code of warning.codes) codes.add(code);
  }
  return codes;
}
