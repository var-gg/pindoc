import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { FileText, GitCommit, Search } from "lucide-react";
import { api, type GitRepoSummary, type SearchHit } from "../api/client";
import { useI18n } from "../i18n";
import { gitCommitPath, isCommitQuery, shortSha } from "../git/routes";
import { cmdkResultMeta } from "./cmdkViewModel";

type Props = {
  projectSlug: string;
  open: boolean;
  onClose: () => void;
};

export function CmdK({ projectSlug, open, onClose }: Props) {
  const { t } = useI18n();
  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [repos, setRepos] = useState<GitRepoSummary[]>([]);
  const [notice, setNotice] = useState<string | undefined>();
  const [selected, setSelected] = useState(0);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const navigate = useNavigate();
  const q = query.trim();
  const commitCandidate = open && isCommitQuery(q) && repos[0]
    ? { kind: "commit" as const, repo: repos[0], sha: q }
    : null;
  const items = [
    ...(commitCandidate ? [commitCandidate] : []),
    ...hits.map((hit) => ({ kind: "artifact" as const, hit })),
  ];

  // Focus the input when the palette opens.
  useEffect(() => {
    if (open) {
      const id = window.setTimeout(() => inputRef.current?.focus(), 40);
      return () => window.clearTimeout(id);
    }
    return;
  }, [open]);

  // Debounced search against the project-scoped search endpoint.
  useEffect(() => {
    if (!open) return;
    if (!q) {
      setHits([]);
      setNotice(undefined);
      return;
    }
    const id = window.setTimeout(async () => {
      try {
        const res = await api.search(projectSlug, q);
        setHits(res.hits);
        setNotice(res.notice);
        setSelected(0);
      } catch (err) {
        setHits([]);
        setNotice(String(err));
      }
    }, 150);
    return () => window.clearTimeout(id);
  }, [open, query, projectSlug]);

  useEffect(() => {
    if (!open || !isCommitQuery(q)) return;
    let cancelled = false;
    api.gitRepos(projectSlug)
      .then((resp) => { if (!cancelled) setRepos(resp.repos); })
      .catch(() => { if (!cancelled) setRepos([]); });
    return () => {
      cancelled = true;
    };
  }, [open, q, projectSlug]);

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSelected((s) => Math.min(s + 1, Math.max(0, items.length - 1)));
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setSelected((s) => Math.max(s - 1, 0));
        return;
      }
      if (e.key === "Enter") {
        e.preventDefault();
        const item = items[selected];
        if (item?.kind === "artifact") {
          navigate(`/p/${projectSlug}/wiki/${item.hit.slug}`);
          onClose();
        }
        if (item?.kind === "commit") {
          navigate(gitCommitPath(projectSlug, item.repo.id, item.sha));
          onClose();
        }
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, items, selected, navigate, onClose, projectSlug]);

  if (!open) return null;

  return (
    <div className="palette-overlay open" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="palette" role="dialog" aria-modal="true">
        <div className="palette__input">
          <Search className="lucide" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("cmdk.placeholder")}
          />
          <span className="kbd">esc</span>
        </div>
        <div className="palette__section">
          <div className="palette__section-head">{t("cmdk.artifacts")}</div>
          {items.length === 0 && (
            <div className="palette__empty">
              {query ? t("cmdk.no_hits") : t("cmdk.hint")}
            </div>
          )}
          {items.map((item, i) => (
            <button
              key={item.kind === "artifact" ? item.hit.artifact_id : `commit-${item.repo.id}-${item.sha}`}
              className={`palette__item${i === selected ? " selected" : ""}`}
              onClick={() => {
                if (item.kind === "artifact") {
                  navigate(`/p/${projectSlug}/wiki/${item.hit.slug}`);
                } else {
                  navigate(gitCommitPath(projectSlug, item.repo.id, item.sha));
                }
                onClose();
              }}
              onMouseEnter={() => setSelected(i)}
            >
              {item.kind === "artifact" ? <FileText className="lucide" /> : <GitCommit className="lucide" />}
              <div>
                <div>{item.kind === "artifact" ? item.hit.title : t("cmdk.commit_result", shortSha(item.sha))}</div>
                <div className="mono">
                  {item.kind === "artifact"
                    ? cmdkResultMeta(item.hit, t)
                    : `${item.repo.name || item.repo.id} · ${item.repo.default_branch}`}
                </div>
              </div>
              <span className="mono">↵</span>
            </button>
          ))}
        </div>
        {notice && (
          <div className="palette__section">
            <div className="palette__empty" style={{ color: "var(--stale)" }}>
              {notice}
            </div>
          </div>
        )}
        <div className="palette__foot">
          <span><span className="kbd">↑↓</span> {t("cmdk.navigate")}</span>
          <span><span className="kbd">↵</span> {t("cmdk.open")}</span>
          <span><span className="kbd">esc</span> {t("cmdk.close")}</span>
        </div>
      </div>
    </div>
  );
}
