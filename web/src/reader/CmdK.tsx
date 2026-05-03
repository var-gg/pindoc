import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { FileText, GitCommit, Search } from "lucide-react";
import { api, type GitRepoSummary, type SearchHit } from "../api/client";
import { useI18n } from "../i18n";
import { gitCommitPath, isCommitQuery, shortSha } from "../git/routes";
import {
  cmdkCommitRows,
  cmdkEmptyCopyKey,
  cmdkNextIndex,
  cmdkOptionId,
  cmdkRelevantHits,
  cmdkResultMeta,
  cmdkSections,
  cmdkTrapTabTarget,
  type CmdKFocusTarget,
  type CmdKNavigationKey,
} from "./cmdkViewModel";
import { projectRoutePrefix, projectSurfacePath } from "../readerRoutes";
import { dismissTooltipsForModal } from "./Tooltip";

type Props = {
  projectSlug: string;
  orgSlug: string;
  open: boolean;
  onClose: () => void;
};

type CmdKCommitItem = {
  kind: "commit";
  repo: GitRepoSummary;
  sha: string;
  summary?: string;
};

type CmdKArtifactItem = {
  kind: "artifact";
  hit: SearchHit;
};

type CmdKItem = CmdKCommitItem | CmdKArtifactItem;

const inputId = "cmdk-input";
const listboxId = "cmdk-listbox";
const titleId = "cmdk-title";

export function CmdK({ projectSlug, orgSlug, open, onClose }: Props) {
  const { t } = useI18n();
  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [repos, setRepos] = useState<GitRepoSummary[]>([]);
  const [commitItems, setCommitItems] = useState<CmdKCommitItem[]>([]);
  const [notice, setNotice] = useState<string | undefined>();
  const [selected, setSelected] = useState(0);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const paletteRef = useRef<HTMLDivElement | null>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);
  const navigate = useNavigate();
  const q = query.trim();
  const items: CmdKItem[] = [
    ...commitItems,
    ...hits.map((hit) => ({ kind: "artifact" as const, hit })),
  ];
  const sections = cmdkSections(items);

  // Focus the input when the palette opens and restore focus after close.
  useEffect(() => {
    if (open) {
      dismissTooltipsForModal();
      restoreFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      const id = window.setTimeout(() => inputRef.current?.focus(), 40);
      return () => window.clearTimeout(id);
    }
    restoreFocusRef.current?.focus();
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
        setHits(cmdkRelevantHits(res.hits));
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
    if (!open || !isCommitQuery(q)) {
      setRepos([]);
      setCommitItems([]);
      return;
    }
    let cancelled = false;
    api.gitRepos(projectSlug)
      .then((resp) => { if (!cancelled) setRepos(resp.repos); })
      .catch(() => { if (!cancelled) setRepos([]); });
    return () => {
      cancelled = true;
    };
  }, [open, q, projectSlug]);

  useEffect(() => {
    if (!open || !isCommitQuery(q) || repos.length === 0) {
      setCommitItems([]);
      return;
    }
    let cancelled = false;
    setCommitItems([]);
    Promise.all(repos.map(async (repo) => {
      try {
        const resp = await api.gitCommit(projectSlug, repo.id, q);
        return {
          repo,
          available: resp.git_preview.available,
          commit: resp.commit,
          summary: resp.commit_info?.summary,
        };
      } catch {
        return { repo, available: false };
      }
    })).then((rows) => {
      if (cancelled) return;
      setCommitItems(cmdkCommitRows(rows, q));
      setSelected(0);
    });
    return () => {
      cancelled = true;
    };
  }, [open, q, repos, projectSlug]);

  useEffect(() => {
    if (items.length === 0) {
      setSelected(0);
      return;
    }
    setSelected((value) => Math.min(value, items.length - 1));
  }, [items.length]);

  useEffect(() => {
    if (!open) return;
    function optionNodes(): HTMLElement[] {
      return Array.from(paletteRef.current?.querySelectorAll<HTMLElement>(".palette__item") ?? []);
    }
    function focusTrapTarget(e: KeyboardEvent): CmdKFocusTarget | null {
      const input = inputRef.current;
      const options = optionNodes();
      const active = document.activeElement;
      if (active !== input && !options.includes(active as HTMLElement)) {
        return "input";
      }
      const optionIndex = options.findIndex((node) => node === active);
      const current: CmdKFocusTarget = optionIndex >= 0 ? optionIndex : "input";
      return cmdkTrapTabTarget(current, options.length, e.shiftKey);
    }
    function activate(item: CmdKItem | undefined) {
      if (item?.kind === "artifact") {
        navigate(projectSurfacePath(projectSlug, "wiki", item.hit.slug, orgSlug));
        onClose();
      }
      if (item?.kind === "commit") {
        navigate(gitCommitPath(projectSlug, item.repo.id, item.sha));
        onClose();
      }
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
        return;
      }
      if (e.key === "Tab") {
        const target = focusTrapTarget(e);
        if (target === null) return;
        e.preventDefault();
        const options = optionNodes();
        if (target === "input") inputRef.current?.focus();
        if (typeof target === "number") options[target]?.focus();
        return;
      }
      if (
        e.key === "ArrowDown" ||
        e.key === "ArrowUp" ||
        e.key === "Home" ||
        e.key === "End" ||
        e.key === "PageDown" ||
        e.key === "PageUp"
      ) {
        e.preventDefault();
        setSelected((value) => cmdkNextIndex(value, items.length, e.key as CmdKNavigationKey));
        return;
      }
      if (e.key === "Enter") {
        e.preventDefault();
        activate(items[selected]);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, items, selected, navigate, onClose, projectSlug, orgSlug]);

  if (!open) return null;

  return (
    <div className="palette-overlay open" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div ref={paletteRef} className="palette" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <h2 id={titleId} className="sr-only">{t("cmdk.dialog_label")}</h2>
        <div className="palette__input">
          <Search className="lucide" />
          <input
            id={inputId}
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("cmdk.placeholder")}
            role="combobox"
            aria-expanded={items.length > 0}
            aria-controls={listboxId}
            aria-autocomplete="list"
            aria-activedescendant={items[selected] ? cmdkOptionId(selected) : undefined}
          />
          <span className="kbd">esc</span>
        </div>
        <div className="palette__section" id={listboxId} role="listbox" aria-label={t("cmdk.results_label")}>
          {items.length === 0 && (
            <div className="palette__empty">
              {t(cmdkEmptyCopyKey(query))}
            </div>
          )}
          {sections.map((section) => (
            <div key={section.kind} className="palette__group" role="group" aria-label={t(section.labelKey)}>
              <div className="palette__section-head" role="presentation">{t(section.labelKey)}</div>
              {section.items.map((item, offset) => {
                const i = section.startIndex + offset;
                return (
                  <button
                    key={item.kind === "artifact" ? item.hit.artifact_id : `commit-${item.repo.id}-${item.sha}`}
                    id={cmdkOptionId(i)}
                    className={`palette__item${i === selected ? " selected" : ""}`}
                    role="option"
                    aria-selected={i === selected}
                    onClick={() => {
                      if (item.kind === "artifact") {
                        navigate(projectSurfacePath(projectSlug, "wiki", item.hit.slug, orgSlug));
                      } else {
                        navigate(gitCommitPath(projectSlug, item.repo.id, item.sha));
                      }
                      onClose();
                    }}
                    onMouseEnter={() => setSelected(i)}
                  >
                    {item.kind === "artifact" ? <FileText className="lucide" /> : <GitCommit className="lucide" />}
                    <div className="palette__item-body">
                      <div className="palette__item-title">
                        {item.kind === "artifact" ? item.hit.title : t("cmdk.commit_result", shortSha(item.sha))}
                      </div>
                      <div className="mono">
                        {item.kind === "artifact"
                          ? `${projectRoutePrefix(projectSlug, orgSlug)} · ${cmdkResultMeta(item.hit, t)}`
                          : `${item.repo.name || item.repo.id} · ${item.summary || item.repo.default_branch}`}
                      </div>
                      {item.kind === "artifact" && item.hit.snippet && (
                        <div className="palette__snippet">{item.hit.snippet}</div>
                      )}
                    </div>
                    <span className="mono">↵</span>
                  </button>
                );
              })}
            </div>
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
