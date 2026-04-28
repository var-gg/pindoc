import type { ChangeGroup, TodayResp } from "../src/api/client";
import { buildChangeGroupCardView, buildTodayBrief } from "../src/reader/todayViewModel";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function t(prefix: string) {
  return (key: string, ...args: Array<string | number>) =>
    `${prefix}:${key}${args.length ? `:${args.join("/")}` : ""}`;
}

function group(overrides: Partial<ChangeGroup> = {}): ChangeGroup {
  return {
    group_id: "g1",
    group_kind: "human_trigger",
    grouping_key: { kind: "bulk_op_id", value: "bulk-1", confidence: "high" },
    commit_summary: "ship route patch",
    revision_count: 2,
    artifact_count: 1,
    areas: ["ui"],
    authors: ["codex"],
    time_start: "2026-04-28T00:00:00Z",
    time_end: "2026-04-28T00:01:00Z",
    importance: { score: 3, level: "high", reasons: ["test"] },
    verification_state: "verified",
    ...overrides,
  };
}

function today(groups: ChangeGroup[], fallbackUsed?: TodayResp["baseline"]["fallback_used"]): TodayResp {
  return {
    project_slug: "pindoc",
    groups,
    summary: {
      headline: "server headline",
      bullets: ["server bullet"],
      source: "rule_based",
      ai_hint: "rule-based",
      created_at: "2026-04-28T00:00:00Z",
    },
    baseline: {
      revision_watermark: 1,
      fallback_used: fallbackUsed,
    },
    max_revision_id: 10,
  };
}

function testHeadlineUsesSameDataWithLocaleCopyOnly(): void {
  const data = today([group(), group({ group_id: "g2", revision_count: 4, artifact_count: 3 })]);
  const ko = buildTodayBrief(data, t("ko"));
  const en = buildTodayBrief(data, t("en"));

  assertEqual(ko.headline, "ko:today.brief_headline_review:2", "KO headline key and count");
  assertEqual(en.headline, "en:today.brief_headline_review:2", "EN headline key and count");
  assertEqual(ko.bullets[1], "ko:today.brief_bullet_counts:2/6/4", "KO totals");
  assertEqual(en.bullets[1], "en:today.brief_bullet_counts:2/6/4", "EN totals");
}

function testFallbackAvoidsTodayReviewHeadline(): void {
  const brief = buildTodayBrief(today([group()], "recent_7d"), t("en"));

  assertEqual(brief.headline, "en:today.brief_headline_fallback:1", "fallback headline key");
  assertEqual(brief.fallbackHint, "en:today.fallback_recent_7d", "fallback hint");
}

function testCardViewHidesRawFallbackAndEnums(): void {
  const view = buildChangeGroupCardView(group({
    commit_summary: "[fallback_missing_commit_msg] create artifact: Reader QA followup; [fallback_missing_commit_msg] create artifact: Today copy cleanup",
    grouping_key: { kind: "author_time_window", value: "codex", confidence: "low" },
    verification_state: "unverified",
  }), t("ko"));

  assertEqual(view.title, "Reader QA followup", "sanitized title");
  assertEqual(view.bullets[0], "Today copy cleanup", "sanitized bullet");
  assertEqual(view.verificationLabel, "ko:today.verification_needs_review", "verification label");
}

testHeadlineUsesSameDataWithLocaleCopyOnly();
testFallbackAvoidsTodayReviewHeadline();
testCardViewHidesRawFallbackAndEnums();
