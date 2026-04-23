// readerWidth — persistent toggle for the article column width
// (Task task-reader-width-modes). Three modes: narrow (blog-like, 640px),
// default (820px, balanced density for spec prose + code blocks), wide
// (1120px, docs-like for diagrams + tables). The chosen mode is written
// to `:root[data-reader-width]` so the CSS cascade in reader.css can
// re-resolve --reader-max without re-render. Persisted in localStorage so
// reloads and new tabs keep the reader's last choice.

export type ReaderWidth = "narrow" | "default" | "wide";

const STORAGE_KEY = "pindoc.reader.width";
export const READER_WIDTHS: ReaderWidth[] = ["narrow", "default", "wide"];

export function initReaderWidth(): ReaderWidth {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === "narrow" || saved === "default" || saved === "wide") {
      document.documentElement.setAttribute("data-reader-width", saved);
      return saved;
    }
  } catch {
    // Ignore (private mode, iframe, etc.).
  }
  document.documentElement.setAttribute("data-reader-width", "default");
  return "default";
}

export function setReaderWidth(next: ReaderWidth) {
  document.documentElement.setAttribute("data-reader-width", next);
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // Ignore.
  }
}
