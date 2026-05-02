import type { ArtifactRef, TaskMeta } from "../src/api/client";
import {
  TASK_REVIEW_INITIAL_LIMIT,
  groupTasksByStatus,
  taskBoardSummary,
  taskColumnInitialLimit,
  visibleTaskGroups,
} from "../src/reader/taskBoardViewModel";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual<T>(actual: T, expected: T, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function task(slug: string, status: string | undefined, priority: string, updatedOffset: number): ArtifactRef {
  const taskMeta: TaskMeta = status
    ? { status: status as TaskMeta["status"], priority: priority as TaskMeta["priority"] }
    : { priority: priority as TaskMeta["priority"] };
  return {
    id: slug,
    slug,
    type: "Task",
    title: slug,
    area_slug: "ui",
    visibility: "org",
    completeness: "partial",
    status: "published",
    review_state: "auto_published",
    author_id: "codex",
    updated_at: new Date(Date.UTC(2026, 3, 28, 0, updatedOffset)).toISOString(),
    task_meta: taskMeta,
  };
}

function testReviewQueueSummaryAndLimit(): void {
  const items = Array.from({ length: 55 }, (_, i) => task(`review-${i}`, "claimed_done", i === 0 ? "p0" : "p3", i));
  const groups = groupTasksByStatus([
    ...items,
    task("open", "open", "p2", 60),
    task("blocked", "blocked", "p1", 61),
  ]);
  const summary = taskBoardSummary(groups);
  assertEqual(summary.reviewQueue, 55, "claimed_done count becomes verification queue");
  assertEqual(summary.open, 1, "open count");
  assertEqual(summary.blocked, 1, "blocked count");
  assertEqual(taskColumnInitialLimit("claimed_done", 55), TASK_REVIEW_INITIAL_LIMIT, "large review queue initial limit");

  const visible = visibleTaskGroups(groups, {});
  const reviewItems = visible.get("claimed_done") ?? [];
  assertEqual(reviewItems.length, TASK_REVIEW_INITIAL_LIMIT, "large review queue shows a recent completion slice");
  assertEqual(reviewItems[0].slug, "review-54", "newest claimed_done item sorts first");
  assert(!reviewItems.some((item) => item.slug === "review-0"), "old p0 claimed_done item does not displace recent completions");
}

function testCancelledStaysInPrimaryColumns(): void {
  const groups = groupTasksByStatus([task("cancelled", "cancelled", "p3", 1)]);
  assert(groups.has("cancelled"), "cancelled column is primary");
  assertEqual(groups.get("cancelled")?.length, 1, "cancelled task stays in the primary status map");
}

function testClaimedDoneTieBreaksByPriority(): void {
  const groups = groupTasksByStatus([
    task("done-p3", "claimed_done", "p3", 10),
    task("done-p1", "claimed_done", "p1", 10),
  ]);
  const done = groups.get("claimed_done") ?? [];
  assertEqual(done[0].slug, "done-p1", "claimed_done ties fall back to priority");
}

function testOpenBlockedAndCancelledKeepPriorityFirst(): void {
  const groups = groupTasksByStatus([
    task("open-new-p3", "open", "p3", 30),
    task("open-old-p1", "open", "p1", 1),
    task("blocked-new-p3", "blocked", "p3", 31),
    task("blocked-old-p1", "blocked", "p1", 2),
    task("cancelled-new-p3", "cancelled", "p3", 32),
    task("cancelled-old-p1", "cancelled", "p1", 3),
  ]);

  assertEqual(groups.get("open")?.[0]?.slug, "open-old-p1", "open column keeps priority before recency");
  assertEqual(groups.get("blocked")?.[0]?.slug, "blocked-old-p1", "blocked column keeps priority before recency");
  assertEqual(groups.get("cancelled")?.[0]?.slug, "cancelled-old-p1", "cancelled column keeps priority before recency");
}

testReviewQueueSummaryAndLimit();
testCancelledStaysInPrimaryColumns();
testClaimedDoneTieBreaksByPriority();
testOpenBlockedAndCancelledKeepPriorityFirst();
