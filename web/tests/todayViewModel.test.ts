import type { ChangeGroup, TodayResp } from "../src/api/client";
import { buildChangeGroupCardView, buildTodayBrief } from "../src/reader/todayViewModel";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
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
    first_artifact: {
      id: "a1",
      slug: "reader-qa-followup",
      title: "Reader QA followup",
      type: "Task",
      area_slug: "ui",
    },
    artifacts: [
      {
        id: "a1",
        slug: "reader-qa-followup",
        title: "Reader QA followup",
        type: "Task",
        area_slug: "ui",
      },
    ],
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

function testCardViewUsesArtifactTitlesAndEnums(): void {
  const view = buildChangeGroupCardView(group({
    commit_summary: "[fallback_missing_commit_msg] create artifact: Reader QA followup; [fallback_missing_commit_msg] create artifact: Today copy cleanup",
    grouping_key: { kind: "author_time_window", value: "codex", confidence: "low" },
    verification_state: "unverified",
    artifact_count: 2,
    artifacts: [
      {
        id: "a1",
        slug: "reader-qa-followup",
        title: "Reader QA followup",
        type: "Task",
        area_slug: "ui",
      },
      {
        id: "a2",
        slug: "today-copy-cleanup",
        title: "Today copy cleanup",
        type: "Task",
        area_slug: "ui",
      },
    ],
  }), t("ko"));

  assertEqual(view.title, "ko:today.change_group_title_representative_area_more:ui/Reader QA followup/1", "representative title");
  assertEqual(view.bullets[0], "ko:today.change_group_bullet_artifact:Reader QA followup", "first artifact bullet");
  assertEqual(view.bullets[1], "ko:today.change_group_bullet_artifact:Today copy cleanup", "second artifact bullet");
  assertEqual(view.verificationLabel, "ko:today.verification_needs_review", "verification label");
}

function testImplementedCommitNoiseDoesNotLeadTodayCard(): void {
  const data = today([
    group({
      commit_summary: "implemented in commit d4ad2e2; logo href /p/pindoc/today; trusted_local profile menu",
      artifact_count: 2,
      areas: ["ui"],
      first_artifact: {
        id: "a1",
        slug: "reader-logo-route",
        title: "Reader logo route",
        type: "Task",
        area_slug: "ui",
      },
      artifacts: [
        {
          id: "a1",
          slug: "reader-logo-route",
          title: "Reader logo route",
          type: "Task",
          area_slug: "ui",
        },
      ],
    }),
  ]);
  const brief = buildTodayBrief(data, t("ko"));
  const firstCard = buildChangeGroupCardView(data.groups[0], t("ko"));
  const snapshot = [
    brief.bullets[0],
    firstCard.title,
    ...firstCard.bullets,
  ].join("\n");

  assertEqual(firstCard.title, "ko:today.change_group_title_representative_area_more:ui/Reader logo route/1", "title uses representative artifact");
  assert(!/implemented in commit [0-9a-f]{7}/i.test(snapshot), "Today copy hides implementation commit noise");
  assert(!snapshot.includes("trusted_local"), "Today copy hides internal commit terms");
}

function testBriefingSnapshotsStayUserFacing(): void {
  const data = today([
    group({
      commit_summary: "[fallback_missing_commit_msg] create artifact: Reader QA followup; [fallback_missing_commit_msg] create artifact: Today copy cleanup",
      grouping_key: { kind: "source_session_time_window", value: "session-1", confidence: "low" },
      verification_state: "unverified",
      artifact_count: 2,
      artifacts: [
        {
          id: "a1",
          slug: "reader-qa-followup",
          title: "Reader QA followup",
          type: "Task",
          area_slug: "ui",
        },
        {
          id: "a2",
          slug: "today-copy-cleanup",
          title: "Today copy cleanup",
          type: "Task",
          area_slug: "ui",
        },
      ],
    }),
  ]);
  const brief = buildTodayBrief(data, t("ko"));
  const firstCard = buildChangeGroupCardView(data.groups[0], t("ko"));
  const desktopSnapshot = [
    brief.sourceLabel,
    brief.headline,
    ...brief.bullets,
    firstCard.title,
    ...firstCard.bullets,
    firstCard.verificationLabel,
  ].join("\n");
  const mobileSnapshot = [
    brief.headline,
    brief.bullets[0],
    firstCard.title,
    firstCard.verificationLabel,
  ].join("\n");

  for (const [name, snapshot] of [["desktop", desktopSnapshot], ["mobile", mobileSnapshot]] as const) {
    assert(!snapshot.includes("fallback_missing_commit_msg"), `${name} snapshot hides fallback marker`);
    assert(!snapshot.includes("source_session_time_window"), `${name} snapshot hides grouping key`);
    assert(!snapshot.includes("unverified"), `${name} snapshot hides raw verification enum`);
    assert(snapshot.includes("Reader QA followup"), `${name} snapshot keeps a readable first-card title`);
  }
}

testHeadlineUsesSameDataWithLocaleCopyOnly();
testFallbackAvoidsTodayReviewHeadline();
testCardViewUsesArtifactTitlesAndEnums();
testImplementedCommitNoiseDoesNotLeadTodayCard();
testBriefingSnapshotsStayUserFacing();
