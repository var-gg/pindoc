import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { Check, Copy, ExternalLink, GitCommit, Loader2 } from "lucide-react";
import {
  api,
  type GitChangedFile,
  type GitCommitInfo,
  type GitCommitReference,
} from "../api/client";
import { useI18n } from "../i18n";
import { EmptyState, SurfaceHeader } from "../reader/SurfacePrimitives";
import { gitCommitPath, shortSha } from "./routes";
import { GitDiff, GitFileList, GitPreviewUnavailable, PinKindBadge } from "./GitPreview";
import { formatDateTime } from "../utils/formatDateTime";

type LoadState =
  | { status: "loading" }
  | { status: "ready"; info: GitCommitInfo; files: GitChangedFile[]; references: GitCommitReference[] }
  | { status: "unavailable"; reason?: string; fallbackURL?: string }
  | { status: "error"; message: string };

export function CommitDetailPage() {
  const { project = "", repoId = "", sha = "" } = useParams<{
    project: string;
    repoId: string;
    sha: string;
  }>();
  const { t, lang } = useI18n();
  const navigate = useNavigate();
  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [selectedPath, setSelectedPath] = useState("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!project || !repoId || !sha) return;
    let cancelled = false;
    setState({ status: "loading" });
    setSelectedPath("");
    (async () => {
      try {
        const commit = await api.gitCommit(project, repoId, sha);
        if (!commit.git_preview.available || !commit.commit_info) {
          if (!cancelled) {
            setState({
              status: "unavailable",
              reason: commit.git_preview.reason,
              fallbackURL: commit.git_preview.fallback_url,
            });
          }
          return;
        }
        const full = commit.commit_info.sha;
        if (full && full !== sha) {
          navigate(gitCommitPath(project, repoId, full), { replace: true });
          return;
        }
        const [changed, refs] = await Promise.all([
          api.gitChangedFiles(project, repoId, full),
          api.gitCommitReferences(project, full),
        ]);
        if (cancelled) return;
        if (!changed.git_preview.available) {
          setState({
            status: "unavailable",
            reason: changed.git_preview.reason,
            fallbackURL: changed.git_preview.fallback_url,
          });
          return;
        }
        const files = changed.files ?? [];
        setState({
          status: "ready",
          info: commit.commit_info,
          files,
          references: refs.references ?? [],
        });
        setSelectedPath(files[0]?.path ?? "");
      } catch (err) {
        if (!cancelled) setState({ status: "error", message: String(err) });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [project, repoId, sha, navigate]);

  const stats = useMemo(() => {
    if (state.status !== "ready") return { additions: 0, deletions: 0 };
    return state.files.reduce(
      (acc, file) => ({
        additions: acc.additions + (file.additions ?? 0),
        deletions: acc.deletions + (file.deletions ?? 0),
      }),
      { additions: 0, deletions: 0 },
    );
  }, [state]);

  const copySha = async () => {
    if (state.status !== "ready" || typeof navigator === "undefined" || !navigator.clipboard) return;
    await navigator.clipboard.writeText(state.info.sha);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  };

  if (state.status === "loading") {
    return (
      <main className="commit-surface">
        <SurfaceHeader name="commit" count={0} />
        <div className="reader-state">
          <Loader2 className="lucide today-spin" />
          {t("git.loading_commit")}
        </div>
      </main>
    );
  }

  if (state.status === "unavailable") {
    return (
      <main className="commit-surface">
        <SurfaceHeader name="commit" count={0} />
        <EmptyState
          message={t("git.commit_unavailable")}
          action={{ label: t("surface.return_today"), href: `/p/${project}/today` }}
        />
        <GitPreviewUnavailable preview={{ available: false, reason: state.reason, fallback_url: state.fallbackURL }} />
      </main>
    );
  }

  if (state.status === "error") {
    return (
      <main className="commit-surface">
        <SurfaceHeader name="commit" count={0} />
        <EmptyState
          message={state.message}
          action={{ label: t("surface.return_today"), href: `/p/${project}/today` }}
        />
      </main>
    );
  }

  return (
    <main className="commit-surface">
      <SurfaceHeader name="commit" count={state.files.length} />
      <header className="commit-head">
        <div className="commit-head__main">
          <div className="commit-head__eyebrow">
            <GitCommit className="lucide" />
            <span>{repoId}</span>
            <span>{shortSha(state.info.sha)}</span>
          </div>
          <h1>{state.info.summary || t("git.commit_no_summary")}</h1>
          <div className="commit-head__meta">
            <span>{state.info.author}</span>
            <time dateTime={state.info.author_time}>{formatDateTime(state.info.author_time, lang)}</time>
            <span className="commit-head__stats">+{stats.additions} -{stats.deletions}</span>
          </div>
        </div>
        <button type="button" className="commit-copy" onClick={copySha}>
          {copied ? <Check className="lucide" /> : <Copy className="lucide" />}
          <span>{copied ? t("git.copied") : state.info.sha}</span>
        </button>
      </header>

      <section className="commit-grid">
        <aside className="commit-files">
          <div className="commit-section-title">{t("git.changed_files")}</div>
          <GitFileList files={state.files} selectedPath={selectedPath} onSelect={setSelectedPath} />
        </aside>
        <section className="commit-diff-pane">
          <div className="commit-section-title">{selectedPath || t("git.diff")}</div>
          {selectedPath ? (
            <GitDiff project={project} repoID={repoId} commit={state.info.sha} path={selectedPath} />
          ) : (
            <EmptyState message={t("git.no_changed_files")} />
          )}
        </section>
      </section>

      <section className="commit-references">
        <div className="commit-section-title">{t("git.referencing_artifacts")}</div>
        {state.references.length === 0 ? (
          <div className="git-empty-line">{t("git.no_references")}</div>
        ) : (
          <div className="commit-reference-list">
            {state.references.map((ref) => (
              <Link
                key={`${ref.artifact_id}-${ref.path}-${ref.lines_start ?? ""}`}
                className="commit-reference"
                to={ref.human_url}
              >
                <PinKindBadge pin={{ kind: ref.kind }} />
                <span className="commit-reference__body">
                  <span className="commit-reference__title">{ref.title}</span>
                  <span className="commit-reference__meta">
                    {ref.type} · {ref.area_slug} · {ref.path}
                    {ref.lines_start
                      ? `:${ref.lines_start}${ref.lines_end && ref.lines_end !== ref.lines_start ? `-${ref.lines_end}` : ""}`
                      : ""}
                  </span>
                </span>
                <ExternalLink className="lucide" />
              </Link>
            ))}
          </div>
        )}
      </section>
    </main>
  );
}
