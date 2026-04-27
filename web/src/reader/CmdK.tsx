import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { FileText, Search } from "lucide-react";
import { api, type SearchHit } from "../api/client";
import { useI18n } from "../i18n";
import { localizedAreaName } from "./areaLocale";

type Props = {
  projectSlug: string;
  open: boolean;
  onClose: () => void;
};

export function CmdK({ projectSlug, open, onClose }: Props) {
  const { t } = useI18n();
  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [notice, setNotice] = useState<string | undefined>();
  const [selected, setSelected] = useState(0);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const navigate = useNavigate();

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
    const q = query.trim();
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
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSelected((s) => Math.min(s + 1, Math.max(0, hits.length - 1)));
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setSelected((s) => Math.max(s - 1, 0));
        return;
      }
      if (e.key === "Enter") {
        e.preventDefault();
        const hit = hits[selected];
        if (hit) {
          navigate(`/p/${projectSlug}/wiki/${hit.slug}`);
          onClose();
        }
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, hits, selected, navigate, onClose, projectSlug]);

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
          {hits.length === 0 && (
            <div className="palette__empty">
              {query ? t("cmdk.no_hits") : t("cmdk.hint")}
            </div>
          )}
          {hits.map((hit, i) => (
            <button
              key={hit.artifact_id}
              className={`palette__item${i === selected ? " selected" : ""}`}
              onClick={() => {
                navigate(`/p/${projectSlug}/wiki/${hit.slug}`);
                onClose();
              }}
              onMouseEnter={() => setSelected(i)}
            >
              <FileText className="lucide" />
              <div>
                <div>{hit.title}</div>
                <div className="mono">
                  {hit.type} · {localizedAreaName(t, hit.area_slug, hit.area_slug)} · distance {hit.distance.toFixed(3)}
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
