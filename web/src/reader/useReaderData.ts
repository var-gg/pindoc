// Shared data-loading hook for the Reader shell. Loads project + areas +
// artifact list once per route change, plus the selected artifact detail.
// Derives type/agent counts from the artifact list so the sidebar doesn't
// need a separate endpoint.

import { useEffect, useState } from "react";
import {
  api,
  type Area,
  type Artifact,
  type ArtifactRef,
  type Project,
} from "../api/client";

export type Aggregate = { key: string; count: number };

export type ReaderData = {
  project: Project;
  areas: Area[];
  artifacts: ArtifactRef[];
  detail: Artifact | null;
  /** counts by artifact type across the unfiltered list. */
  types: Aggregate[];
  /** counts by author_id across the unfiltered list. */
  agents: Aggregate[];
};

export type LoadState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; data: ReaderData };

export function useReaderData(projectSlug: string, slug?: string, includeTemplates = false): LoadState {
  const [state, setState] = useState<LoadState>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [project, areasResp, listResp] = await Promise.all([
          api.project(projectSlug),
          api.areas(projectSlug),
          api.artifacts(projectSlug, { includeTemplates }),
        ]);
        // Only load detail when a slug is explicitly requested. The
        // handoff's intended navigation is ⌘K-first; auto-loading the
        // latest artifact made /tasks and /wiki render identically
        // (first artifact is an Analysis), which hid the point of the
        // type filter entirely.
        let detail: Artifact | null = null;
        if (slug) {
          detail = await api.artifact(projectSlug, slug);
        }
        if (cancelled) return;

        setState({
          kind: "ready",
          data: {
            project,
            areas: areasResp.areas,
            artifacts: listResp.artifacts,
            detail,
            types: aggregate(listResp.artifacts, (a) => a.type),
            agents: aggregate(listResp.artifacts, (a) => a.author_id),
          },
        });
      } catch (err) {
        if (cancelled) return;
        setState({ kind: "error", message: String(err) });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectSlug, slug, includeTemplates]);

  return state;
}

function aggregate<T>(xs: T[], keyOf: (x: T) => string): Aggregate[] {
  const counts = new Map<string, number>();
  for (const x of xs) {
    const k = keyOf(x);
    counts.set(k, (counts.get(k) ?? 0) + 1);
  }
  return Array.from(counts, ([key, count]) => ({ key, count })).sort(
    (a, b) => b.count - a.count,
  );
}
