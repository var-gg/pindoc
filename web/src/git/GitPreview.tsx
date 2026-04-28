import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from "react";
import { ExternalLink, FileText, GitCommit, Image, Link as LinkIcon } from "lucide-react";
import {
  api,
  type GitBlobResp,
  type GitChangedFile,
  type GitDiffResp,
  type GitPreviewEnvelope,
  type PinRef,
} from "../api/client";
import { useI18n } from "../i18n";
import { PindocMarkdown } from "../reader/Markdown";
import { BadgeWithExplain } from "../reader/BadgeWithExplain";
import {
  visualDescription,
  visualLabel,
  visualPin,
} from "../reader/visualLanguage";
import { visualIconComponent } from "../reader/visualLanguageIcons";
import { shortSha } from "./routes";

export function formatShortSha(sha: string | undefined | null): string {
  return shortSha(sha);
}

export function pinLabel(pin: PinRef, locale: string): string {
  const entry = visualPin(pin.kind);
  return entry ? visualLabel(entry, locale) : pin.kind;
}

export function PinKindBadge({ pin }: { pin: Pick<PinRef, "kind"> }) {
  const { lang } = useI18n();
  const entry = visualPin(pin.kind);
  const Icon = visualIconComponent(entry?.icon);
  const label = entry ? visualLabel(entry, lang) : pin.kind;
  const description = entry ? visualDescription(entry, lang) : `${pin.kind} pin`;
  const style = entry
    ? { "--pin-color": `var(${entry.color_token})` } as CSSProperties & Record<"--pin-color", string>
    : undefined;
  return (
    <BadgeWithExplain label={label} description={description} className="pin-kind-chip" style={style}>
      <Icon className="lucide" />
      {label}
    </BadgeWithExplain>
  );
}

export function GitFileList({
  files,
  selectedPath,
  onSelect,
}: {
  files: GitChangedFile[];
  selectedPath?: string;
  onSelect: (path: string) => void;
}) {
  const { t } = useI18n();
  if (files.length === 0) {
    return <div className="git-empty-line">{t("git.no_changed_files")}</div>;
  }
  return (
    <div className="git-file-list" role="list" aria-label={t("git.changed_files")}>
      {files.map((file) => (
        <button
          key={`${file.status}-${file.path}`}
          type="button"
          className={`git-file-list__item${file.path === selectedPath ? " is-active" : ""}`}
          onClick={() => onSelect(file.path)}
        >
          <span className={`git-file-list__status git-file-list__status--${statusClass(file.status)}`}>
            {file.status}
          </span>
          <span className="git-file-list__path">{file.path}</span>
          <span className="git-file-list__stats">
            {file.binary
              ? t("git.binary")
              : `+${file.additions ?? 0} -${file.deletions ?? 0}`}
          </span>
        </button>
      ))}
    </div>
  );
}

export function GitBlobPreview({
  project,
  pin,
}: {
  project: string;
  pin: PinRef;
}) {
  const { t } = useI18n();
  const [resp, setResp] = useState<GitBlobResp | null>(null);
  const [error, setError] = useState<string | null>(null);
  const repoID = pin.repo_id ?? "";
  const commit = pin.commit_sha ?? "";
  const canFetch = Boolean(repoID && commit && pin.path && pin.kind !== "url" && pin.kind !== "resource");

  useEffect(() => {
    if (!canFetch) {
      setResp(null);
      setError(null);
      return;
    }
    let cancelled = false;
    setError(null);
    api.gitBlob(project, repoID, commit, pin.path)
      .then((data) => { if (!cancelled) setResp(data); })
      .catch((err) => { if (!cancelled) setError(String(err)); });
    return () => {
      cancelled = true;
    };
  }, [canFetch, project, repoID, commit, pin.path]);

  if (pin.kind === "url") {
    return (
      <GitPreviewShell>
        <a className="git-url-preview" href={pin.path} target="_blank" rel="noreferrer">
          <ExternalLink className="lucide" />
          <span>{pin.path}</span>
        </a>
      </GitPreviewShell>
    );
  }
  if (pin.kind === "resource") {
    return (
      <GitPreviewShell>
        <div className="git-asset-preview">
          <LinkIcon className="lucide" />
          <span>{pin.path}</span>
        </div>
      </GitPreviewShell>
    );
  }
  if (!canFetch) {
    return <GitPreviewUnavailable preview={{ available: false, reason: "missing_repo_or_commit" }} />;
  }
  if (error) {
    return <GitPreviewUnavailable preview={{ available: false, reason: error }} />;
  }
  if (!resp) {
    return <GitPreviewShell><div className="git-empty-line">{t("git.loading_preview")}</div></GitPreviewShell>;
  }
  if (!resp.git_preview.available || !resp.blob) {
    return <GitPreviewUnavailable preview={resp.git_preview} />;
  }
  const blob = resp.blob;
  if (pin.kind === "asset" || blob.binary) {
    return (
      <GitPreviewShell>
        <div className="git-asset-preview">
          {isImagePath(blob.path) && !blob.binary ? <Image className="lucide" /> : <FileText className="lucide" />}
          <span>{blob.path}</span>
          <span className="git-asset-preview__meta">{formatBytes(blob.size)}</span>
        </div>
      </GitPreviewShell>
    );
  }
  if (pin.kind === "doc" && isMarkdownPath(blob.path)) {
    return (
      <GitPreviewShell>
        <div className="git-markdown-preview">
          <PindocMarkdown source={blob.text ?? ""} projectSlug={project} collapseStructureSections />
        </div>
      </GitPreviewShell>
    );
  }
  return (
    <GitPreviewShell>
      <CodePreview text={blob.text ?? ""} linesStart={pin.lines_start} linesEnd={pin.lines_end} />
    </GitPreviewShell>
  );
}

export function GitDiff({
  project,
  repoID,
  commit,
  path,
}: {
  project: string;
  repoID: string;
  commit: string;
  path: string;
}) {
  const { t } = useI18n();
  const [resp, setResp] = useState<GitDiffResp | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!repoID || !commit || !path) return;
    let cancelled = false;
    setResp(null);
    setError(null);
    api.gitDiff(project, repoID, commit, path)
      .then((data) => { if (!cancelled) setResp(data); })
      .catch((err) => { if (!cancelled) setError(String(err)); });
    return () => {
      cancelled = true;
    };
  }, [project, repoID, commit, path]);

  if (error) {
    return <GitPreviewUnavailable preview={{ available: false, reason: error }} />;
  }
  if (!repoID || !commit || !path) {
    return <GitPreviewUnavailable preview={{ available: false, reason: "missing_repo_or_commit" }} />;
  }
  if (!resp) {
    return <GitPreviewShell><div className="git-empty-line">{t("git.loading_diff")}</div></GitPreviewShell>;
  }
  if (!resp.git_preview.available) {
    return <GitPreviewUnavailable preview={resp.git_preview} />;
  }
  return (
    <GitPreviewShell>
      <pre className="git-diff" aria-label={t("git.diff")}>
        {(resp.diff || t("git.no_diff")).split("\n").map((line, index) => (
          <span key={`${index}-${line}`} className={diffLineClass(line)}>
            {line || " "}
          </span>
        ))}
      </pre>
    </GitPreviewShell>
  );
}

export function GitPreviewUnavailable({ preview }: { preview: GitPreviewEnvelope }) {
  const { t } = useI18n();
  return (
    <GitPreviewShell>
      <div className="git-preview-unavailable">
        <GitCommit className="lucide" />
        <span>{gitPreviewReasonLabel(preview.reason, t)}</span>
        {preview.fallback_url && (
          <a href={preview.fallback_url} target="_blank" rel="noreferrer">
            {t("git.open_fallback")}
          </a>
        )}
      </div>
    </GitPreviewShell>
  );
}

function GitPreviewShell({ children }: { children: ReactNode }) {
  return <div className="git-preview">{children}</div>;
}

function CodePreview({
  text,
  linesStart,
  linesEnd,
}: {
  text: string;
  linesStart?: number;
  linesEnd?: number;
}) {
  const lines = useMemo(() => text.split("\n"), [text]);
  const start = linesStart && linesStart > 0 ? linesStart : 1;
  const end = linesEnd && linesEnd >= start ? linesEnd : start;
  return (
    <pre className="git-code-preview">
      {lines.map((line, i) => {
        const lineNo = i + 1;
        const highlighted = Boolean(linesStart && lineNo >= start && lineNo <= end);
        return (
          <span key={`${lineNo}-${line}`} className={highlighted ? "is-highlighted" : ""}>
            <span className="git-code-preview__line-no">{lineNo}</span>
            <span className="git-code-preview__text">{line || " "}</span>
          </span>
        );
      })}
    </pre>
  );
}

function statusClass(status: string): string {
  const c = status.slice(0, 1).toUpperCase();
  if (c === "A") return "added";
  if (c === "D") return "deleted";
  if (c === "R") return "renamed";
  return "modified";
}

function diffLineClass(line: string): string {
  if (line.startsWith("+++") || line.startsWith("---")) return "git-diff__line git-diff__line--meta";
  if (line.startsWith("@@")) return "git-diff__line git-diff__line--hunk";
  if (line.startsWith("+")) return "git-diff__line git-diff__line--add";
  if (line.startsWith("-")) return "git-diff__line git-diff__line--del";
  if (line.startsWith("diff ") || line.startsWith("index ")) return "git-diff__line git-diff__line--meta";
  return "git-diff__line";
}

function gitPreviewReasonLabel(
  reason: string | undefined,
  t: (key: string, ...args: Array<string | number>) => string,
): string {
  const key = reason ? `git.unavailable.${reason}` : "git.unavailable.default";
  const translated = t(key);
  return translated === key ? t("git.unavailable.default") : translated;
}

function isMarkdownPath(path: string): boolean {
  return /\.(md|mdx|markdown|txt)$/i.test(path);
}

function isImagePath(path: string): boolean {
  return /\.(png|jpe?g|gif|webp|avif|svg)$/i.test(path);
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
