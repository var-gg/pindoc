// Theme bootstrap mirror of the script tag in reader.html. Runs on module
// load so the initial paint matches the user's last choice.

export type Theme = "light" | "dark";

const STORAGE_KEY = "pindoc.theme";

export function initTheme(): Theme {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    const root = document.documentElement;
    if (saved === "dark") {
      root.classList.add("theme-dark");
      root.setAttribute("data-theme-source", "user");
      return "dark";
    }
    if (saved === "light") {
      root.classList.remove("theme-dark");
      root.setAttribute("data-theme-source", "user");
      return "light";
    }
  } catch {
    // Ignore storage errors (private mode, iframe, etc.).
  }
  return "light";
}

export function setTheme(next: Theme) {
  const root = document.documentElement;
  if (next === "dark") {
    root.classList.add("theme-dark");
  } else {
    root.classList.remove("theme-dark");
  }
  root.setAttribute("data-theme-source", "user");
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // Ignore.
  }
}
