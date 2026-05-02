// Shared data-loading hook for the Reader shell. Loads project + areas +
// artifact list once per route change, plus the selected artifact detail.
// Derives type/agent counts from the artifact list so the sidebar doesn't
// need a separate endpoint.
//
// reload() lets local write surfaces such as visibility changes refetch
// artifact detail so the sidecar and revision rail refresh without a
// full page navigation.

import { useCallback, useEffect, useState } from "react";
import {
  api,
  type Area,
  type Artifact,
  type ArtifactRef,
  type Project,
  type UserRef,
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
  /** instance-wide users list (migration 0014). Empty array while the
   * endpoint is reachable but returned no rows; null when the fetch
   * failed so the UI can fall back without blocking the rest of the shell. */
  users: UserRef[] | null;
};

export type LoadState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; data: ReaderData; reload: () => void };

export function useReaderData(projectSlug: string, slug?: string, includeTemplates = false): LoadState {
  const [state, setState] = useState<LoadState>({ kind: "loading" });
  // nonce bump forces the fetch effect to re-run without restarting the
  // whole shell.
  const [reloadNonce, setReloadNonce] = useState(0);

  const reload = useCallback(() => setReloadNonce((n) => n + 1), []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [project, areasResp, listResp, usersResp] = await Promise.all([
          api.project(projectSlug),
          api.areas(projectSlug, { includeTemplates }),
          api.artifacts(projectSlug, { includeTemplates }),
          api.users().catch(() => null),
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
            users: usersResp?.users ?? null,
          },
          reload,
        });
      } catch (err) {
        if (cancelled) return;
        setState({ kind: "error", message: String(err) });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectSlug, slug, includeTemplates, reloadNonce, reload]);

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
