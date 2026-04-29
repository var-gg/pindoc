import type { ArtifactReadState, ArtifactRef, ReadState } from "../api/client";

export type AreaTreeLike = {
  slug: string;
  children?: AreaTreeLike[];
};

export function isUnreadReadState(state: ReadState): boolean {
  return state === "unseen" || state === "glanced";
}

export function buildAreaUnreadOwnCounts(
  artifacts: readonly ArtifactRef[],
  states: readonly ArtifactReadState[],
): Map<string, number> {
  const artifactById = new Map(artifacts.map((a) => [a.id, a]));
  const counts = new Map<string, number>();

  for (const row of states) {
    if (!isUnreadReadState(row.read_state)) continue;
    const artifact = artifactById.get(row.artifact_id);
    if (!artifact) continue;
    counts.set(artifact.area_slug, (counts.get(artifact.area_slug) ?? 0) + 1);
  }

  return counts;
}

export function subtreeUnreadCount(
  node: AreaTreeLike,
  ownCounts: ReadonlyMap<string, number>,
): number {
  const children = node.children ?? [];
  return (ownCounts.get(node.slug) ?? 0)
    + children.reduce((sum, child) => sum + subtreeUnreadCount(child, ownCounts), 0);
}
