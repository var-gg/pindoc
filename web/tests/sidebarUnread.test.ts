import type { ArtifactReadState, ArtifactRef } from "../src/api/client";
import { buildAreaUnreadOwnCounts, subtreeUnreadCount } from "../src/reader/sidebarUnread";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function artifact(id: string, areaSlug: string): ArtifactRef {
  return {
    id,
    slug: id,
    type: "Decision",
    title: id,
    area_slug: areaSlug,
    visibility: "org",
    completeness: "settled",
    status: "published",
    review_state: "auto_published",
    author_id: "codex",
    updated_at: "2026-04-29T00:00:00Z",
  };
}

function state(artifactId: string, readState: ArtifactReadState["read_state"]): ArtifactReadState {
  return {
    artifact_id: artifactId,
    read_state: readState,
    completion_pct: 0,
    event_count: 1,
  };
}

function testUnreadCountsUseEndpointRowsAndAggregateSubtree(): void {
  const own = buildAreaUnreadOwnCounts(
    [
      artifact("a1", "experience"),
      artifact("a2", "experience.reader"),
      artifact("a3", "experience.reader"),
      artifact("a4", "system"),
    ],
    [
      state("a1", "unseen"),
      state("a2", "glanced"),
      state("a3", "read"),
      state("a4", "deeply_read"),
      state("missing", "unseen"),
    ],
  );

  assertEqual(own.get("experience"), 1, "own unseen count");
  assertEqual(own.get("experience.reader"), 1, "own glanced count");
  assertEqual(own.get("system") ?? 0, 0, "read rows are not unread");
  assertEqual(
    subtreeUnreadCount(
      { slug: "experience", children: [{ slug: "experience.reader" }] },
      own,
    ),
    2,
    "parent area aggregates own plus children",
  );
}

testUnreadCountsUseEndpointRowsAndAggregateSubtree();
