import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { api, type Area, type Artifact, type ArtifactRef, type Project } from "../api/client";

type LoadState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; project: Project; areas: Area[]; list: ArtifactRef[]; detail: Artifact | null };

export function WikiRoute() {
  const params = useParams<{ slug?: string }>();
  const [state, setState] = useState<LoadState>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [project, areasResp, listResp] = await Promise.all([
          api.currentProject(),
          api.areas(),
          api.artifacts(),
        ]);
        let detail: Artifact | null = null;
        if (params.slug) {
          detail = await api.artifact(params.slug);
        } else if (listResp.artifacts.length > 0) {
          detail = await api.artifact(listResp.artifacts[0]!.slug);
        }
        if (cancelled) return;
        setState({
          kind: "ready",
          project,
          areas: areasResp.areas,
          list: listResp.artifacts,
          detail,
        });
      } catch (err) {
        if (cancelled) return;
        setState({ kind: "error", message: String(err) });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [params.slug]);

  if (state.kind === "loading") {
    return <div className="wiki__state">Loading…</div>;
  }
  if (state.kind === "error") {
    return (
      <div className="wiki__state wiki__state--error">
        <strong>Can't reach pindoc-api.</strong>
        <p>{state.message}</p>
        <p>
          Start it with: <code>go run ./cmd/pindoc-api</code> (listens on{" "}
          <code>127.0.0.1:5831</code>).
        </p>
      </div>
    );
  }

  const { project, areas, list, detail } = state;
  return (
    <div className="wiki">
      <aside className="wiki__nav">
        <header className="wiki__project">
          <span className="wiki__dot" style={{ background: project.color || "var(--accent)" }} />
          <div>
            <div className="wiki__name">{project.name}</div>
            <div className="wiki__meta">
              {project.artifacts_count} artifacts · {project.primary_language}
            </div>
          </div>
        </header>
        <section>
          <h3>Areas</h3>
          <ul className="wiki__areas">
            {areas.map((a) => (
              <li key={a.id} className={a.is_cross_cutting ? "is-cross" : undefined}>
                <span className="wiki__area-name">{a.name}</span>
                <span className="wiki__area-count">{a.artifact_count}</span>
              </li>
            ))}
          </ul>
        </section>
        <section>
          <h3>Artifacts</h3>
          <ul className="wiki__list">
            {list.length === 0 && <li className="wiki__empty">No artifacts yet. Write one via pindoc.artifact.propose.</li>}
            {list.map((a) => (
              <li key={a.id} className={detail?.id === a.id ? "is-active" : undefined}>
                <Link to={`/wiki/${a.slug}`}>
                  <span className="wiki__chip">{a.type}</span>
                  <span className="wiki__title">{a.title}</span>
                  <span className="wiki__row-meta">{a.area_slug}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      </aside>
      <main className="wiki__main">
        {detail ? <ArtifactView detail={detail} /> : <div className="wiki__empty">Pick an artifact.</div>}
      </main>
    </div>
  );
}

function ArtifactView({ detail }: { detail: Artifact }) {
  const publishedAt = useMemo(
    () => (detail.published_at ? new Date(detail.published_at).toLocaleString() : "—"),
    [detail.published_at],
  );
  return (
    <article className="reader">
      <header>
        <div className="reader__crumb">
          {detail.area_slug} · <span className="reader__type">{detail.type}</span> ·{" "}
          <span className="reader__slug">{detail.slug}</span>
        </div>
        <h1>{detail.title}</h1>
        <div className="reader__meta">
          <span className={`reader__badge reader__badge--${detail.status}`}>{detail.status}</span>
          <span>written by {detail.author_id}</span>
          <span>published {publishedAt}</span>
        </div>
      </header>
      <pre className="reader__body">{detail.body_markdown}</pre>
    </article>
  );
}
