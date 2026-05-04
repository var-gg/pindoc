// Shared data-loading hook for the Reader shell. Loads project + areas +
// artifact list once per route change, plus the selected artifact detail.
// Derives type/agent counts from the artifact list so the sidebar doesn't
// need a separate endpoint.
//
// reload() lets local write surfaces such as visibility changes refetch
// artifact detail so the sidecar and revision rail refresh without a
// full page navigation.

import { useCallback, useEffect, useRef, useState } from "react";
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
  artifactsHasMore: boolean;
  artifactsNextCursor?: string;
  artifactsLoadingMore: boolean;
  artifactsLoadMoreError: string | null;
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
  | { kind: "ready"; data: ReaderData; reload: () => void; loadMoreArtifacts: () => Promise<void> };

export function useReaderData(projectSlug: string, slug?: string, includeTemplates = false): LoadState {
  const [state, setState] = useState<LoadState>({ kind: "loading" });
  const stateRef = useRef<LoadState>({ kind: "loading" });
  // nonce bump forces the fetch effect to re-run without restarting the
  // whole shell.
  const [reloadNonce, setReloadNonce] = useState(0);

  const reload = useCallback(() => setReloadNonce((n) => n + 1), []);
  const loadMoreArtifacts = useCallback(async () => {
    const current = stateRef.current;
    if (
      current.kind !== "ready" ||
      current.data.artifactsLoadingMore ||
      !current.data.artifactsHasMore ||
      !current.data.artifactsNextCursor
    ) {
      return;
    }
    const cursor = current.data.artifactsNextCursor;
    setState((current) => {
      if (current.kind !== "ready") return current;
      return {
        ...current,
        data: {
          ...current.data,
          artifactsLoadingMore: true,
          artifactsLoadMoreError: null,
        },
      };
    });

    try {
      const page = await api.artifacts(projectSlug, {
        includeTemplates,
        cursor,
        limit: 200,
      });
      setState((current) => {
        if (current.kind !== "ready") return current;
        const seen = new Set(current.data.artifacts.map((artifact) => artifact.id));
        const artifacts = current.data.artifacts.concat(
          page.artifacts.filter((artifact) => !seen.has(artifact.id)),
        );
        return {
          ...current,
          data: {
            ...current.data,
            artifacts,
            artifactsHasMore: Boolean(page.has_more),
            artifactsNextCursor: page.next_cursor,
            artifactsLoadingMore: false,
            artifactsLoadMoreError: null,
            types: aggregate(artifacts, (a) => a.type),
            agents: aggregate(artifacts, (a) => a.author_id),
          },
        };
      });
    } catch (err) {
      setState((current) => {
        if (current.kind !== "ready") return current;
        return {
          ...current,
          data: {
            ...current.data,
            artifactsLoadingMore: false,
            artifactsLoadMoreError: String(err),
          },
        };
      });
    }
  }, [projectSlug, includeTemplates]);

  useEffect(() => {
    stateRef.current = state;
  }, [state]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setState({ kind: "loading" });
        const [project, areasResp, listResp, usersResp] = await Promise.all([
          api.project(projectSlug),
          api.areas(projectSlug, { includeTemplates }),
          api.artifacts(projectSlug, { includeTemplates, limit: 200 }),
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
            artifactsHasMore: Boolean(listResp.has_more),
            artifactsNextCursor: listResp.next_cursor,
            artifactsLoadingMore: false,
            artifactsLoadMoreError: null,
            detail,
            types: aggregate(listResp.artifacts, (a) => a.type),
            agents: aggregate(listResp.artifacts, (a) => a.author_id),
            users: usersResp?.users ?? null,
          },
          reload,
          loadMoreArtifacts,
        });
      } catch (err) {
        if (cancelled) return;
        setState({ kind: "error", message: String(err) });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectSlug, slug, includeTemplates, reloadNonce, reload, loadMoreArtifacts]);

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
