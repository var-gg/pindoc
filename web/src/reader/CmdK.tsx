import { useEffect, useRef, useState, type CSSProperties } from "react";
import { useNavigate } from "react-router";
import { CheckSquare, FileText, GitCommit, Search } from "lucide-react";
import { api, type GitRepoSummary, type SearchHit } from "../api/client";
import { useI18n } from "../i18n";
import { gitCommitPath, isCommitQuery, shortSha } from "../git/routes";
import {
  cmdkArtifactPath,
  cmdkCommitRows,
  cmdkEmptyCopyKey,
  cmdkNextIndex,
  cmdkOptionId,
  cmdkOtherProjectHits,
  cmdkProjectChip,
  cmdkRelevantHits,
  cmdkResultDetailMeta,
  cmdkSections,
  type CmdKNavigationKey,
} from "./cmdkViewModel";
import { localizedAreaName } from "./areaLocale";
import { typeChipClass } from "./typeChip";
import { dismissTooltipsForModal } from "./Tooltip";
import { visualArea, visualLabel, visualType } from "./visualLanguage";
import { visualIconComponent } from "./visualLanguageIcons";

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
  artifactScope: "current" | "global";
  hit: SearchHit;
};

type CmdKItem = CmdKCommitItem | CmdKArtifactItem;
type CmdKSearchScope = "all" | "tasks";

const inputId = "cmdk-input";
const listboxId = "cmdk-listbox";
const titleId = "cmdk-title";
const taskSearchType = "Task";

export function CmdK({ projectSlug, orgSlug, open, onClose }: Props) {
  const { t } = useI18n();
  const [query, setQuery] = useState("");
  const [searchScope, setSearchScope] = useState<CmdKSearchScope>("all");
  const [currentHits, setCurrentHits] = useState<SearchHit[]>([]);
  const [globalHits, setGlobalHits] = useState<SearchHit[]>([]);
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
    ...currentHits.map((hit) => ({ kind: "artifact" as const, artifactScope: "current" as const, hit })),
    ...globalHits.map((hit) => ({ kind: "artifact" as const, artifactScope: "global" as const, hit })),
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
      setCurrentHits([]);
      setGlobalHits([]);
      setNotice(undefined);
      return;
    }
    let cancelled = false;
    const id = window.setTimeout(async () => {
      try {
        const searchOptions = searchScope === "tasks" ? { type: taskSearchType } : undefined;
        const resp = q.length < 3
          ? await api.search(projectSlug, q, searchOptions)
          : await api.searchGlobal(projectSlug, q, searchOptions);
        if (cancelled) return;
        const hits = cmdkRelevantHits(resp.hits).filter((hit) => (
          searchScope === "tasks" ? hit.type.toLowerCase() === taskSearchType.toLowerCase() : true
        ));
        setCurrentHits(hits.filter((hit) => hit.project_slug === projectSlug));
        setGlobalHits(q.length < 3 ? [] : cmdkOtherProjectHits(hits, projectSlug, orgSlug));
        setNotice(resp.notice);
        setSelected(0);
      } catch (err) {
        if (cancelled) return;
        setCurrentHits([]);
        setGlobalHits([]);
        setNotice(String(err));
      }
    }, 150);
    return () => {
      cancelled = true;
      window.clearTimeout(id);
    };
  }, [open, query, projectSlug, orgSlug, searchScope]);

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
    function focusTrapTarget(e: KeyboardEvent): HTMLElement | null {
      const input = inputRef.current;
      if (!input) return null;
      const controls = Array.from(paletteRef.current?.querySelectorAll<HTMLElement>(".palette__scope-button, .palette__item") ?? []);
      const focusable = [input, ...controls];
      const active = document.activeElement;
      const currentIndex = focusable.findIndex((node) => node === active);
      if (currentIndex < 0) {
        return input;
      }
      if (e.shiftKey && currentIndex === 0) return focusable[focusable.length - 1] ?? input;
      if (!e.shiftKey && currentIndex === focusable.length - 1) return input;
      return null;
    }
    function activate(item: CmdKItem | undefined) {
      if (item?.kind === "artifact") {
        navigate(cmdkArtifactPath(item.hit, projectSlug, orgSlug));
        onClose();
      }
      if (item?.kind === "commit") {
        navigate(gitCommitPath(projectSlug, item.repo.id, item.sha));
        onClose();
      }
    }
    function scopeButtonHasFocus(): boolean {
      const active = document.activeElement;
      return active instanceof HTMLElement && active.classList.contains("palette__scope-button");
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
        target.focus();
        return;
      }
      if ((e.key === "Enter" || e.key === " ") && scopeButtonHasFocus()) {
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
        <div className="palette__scope" role="group" aria-label={t("cmdk.scope_label")}>
          <button
            type="button"
            className={`palette__scope-button${searchScope === "all" ? " is-active" : ""}`}
            aria-pressed={searchScope === "all"}
            onClick={() => {
              setSearchScope("all");
              setSelected(0);
            }}
          >
            <Search className="lucide" aria-hidden="true" />
            <span>{t("cmdk.scope_all")}</span>
          </button>
          <button
            type="button"
            className={`palette__scope-button${searchScope === "tasks" ? " is-active" : ""}`}
            aria-pressed={searchScope === "tasks"}
            onClick={() => {
              setSearchScope("tasks");
              setSelected(0);
            }}
          >
            <CheckSquare className="lucide" aria-hidden="true" />
            <span>{t("cmdk.scope_tasks")}</span>
          </button>
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
                    key={item.kind === "artifact" ? `${item.artifactScope}-${item.hit.artifact_id}` : `commit-${item.repo.id}-${item.sha}`}
                    id={cmdkOptionId(i)}
                    className={`palette__item${i === selected ? " selected" : ""}`}
                    role="option"
                    aria-selected={i === selected}
                    onClick={() => {
                      if (item.kind === "artifact") {
                        navigate(cmdkArtifactPath(item.hit, projectSlug, orgSlug));
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
                      <div className="mono palette__item-meta">
                        {item.kind === "artifact" ? (
                          <ArtifactResultMeta hit={item.hit} projectSlug={projectSlug} orgSlug={orgSlug} />
                        ) : (
                          `${item.repo.name || item.repo.id} · ${item.summary || item.repo.default_branch}`
                        )}
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

function ArtifactResultMeta({
  hit,
  projectSlug,
  orgSlug,
}: {
  hit: SearchHit;
  projectSlug: string;
  orgSlug: string;
}) {
  const { t, lang } = useI18n();
  const typeEntry = visualType(hit.type);
  const TypeIcon = visualIconComponent(typeEntry?.icon);
  const typeLabel = typeEntry ? visualLabel(typeEntry, lang) : hit.type;
  const areaEntry = visualArea(hit.area_slug);
  const AreaIcon = visualIconComponent(areaEntry?.icon ?? "Folder");
  const areaLabel = areaEntry ? visualLabel(areaEntry, lang) : localizedAreaName(t, hit.area_slug, hit.area_slug);
  const areaStyle = areaEntry
    ? ({ "--area-color": `var(${areaEntry.color_token})` } as CSSProperties)
    : undefined;
  const areaClassName = `chip-area chip-area--visual${areaEntry ? "" : " chip-area--custom"}`;
  const detailMeta = cmdkResultDetailMeta(hit, t);

  return (
    <>
      <span className={`${typeChipClass(hit.type)} type-chip--visual type-chip--compact`} aria-label={typeLabel} title={typeLabel}>
        <TypeIcon className="lucide" aria-hidden="true" />
        <span>{typeEntry?.canonical ?? hit.type}</span>
      </span>
      <span className={areaClassName} style={areaStyle} aria-label={areaLabel} title={areaLabel}>
        <AreaIcon className="lucide" aria-hidden="true" />
        <span>{areaLabel}</span>
      </span>
      <span className="palette__project-chip">
        {cmdkProjectChip(hit, projectSlug, orgSlug)}
      </span>
      {detailMeta && <span>{detailMeta}</span>}
    </>
  );
}
