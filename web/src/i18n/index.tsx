// Minimal i18n for the web shell. No react-i18next — that's overkill for
// a few dozen UI strings. A tiny Provider + hook keeps the surface area
// about 30 lines total and ships zero runtime dependencies.
//
// Language is resolved in this order:
//   1. localStorage key 'pindoc.ui.lang' (user override)
//   2. Pindoc project's primary_language from /api/projects/current
//   3. Browser navigator.language substring ('ko-KR' → 'ko')
//   4. Default 'en'

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import en from "./en.json";
import ko from "./ko.json";

type Bundle = Record<string, string>;

const bundles: Record<string, Bundle> = { en, ko };

export type Lang = "en" | "ko";

type I18nCtx = {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: (key: string, ...args: Array<string | number>) => string;
};

const ctx = createContext<I18nCtx | null>(null);

function interpolate(template: string, args: Array<string | number>): string {
  if (args.length === 0) return template;
  // printf-like %s | %d — kept minimal to match the server's fmt.Sprintf
  let i = 0;
  return template.replace(/%[sd]/g, () => String(args[i++] ?? ""));
}

function detectLang(projectLang?: string): Lang {
  if (typeof window !== "undefined") {
    const stored = window.localStorage.getItem("pindoc.ui.lang");
    if (stored === "ko" || stored === "en") return stored;
  }
  if (projectLang === "ko" || projectLang === "en") return projectLang;
  if (typeof navigator !== "undefined" && navigator.language?.startsWith("ko")) {
    return "ko";
  }
  return "en";
}

export function I18nProvider({
  children,
  projectLang,
}: {
  children: ReactNode;
  projectLang?: string;
}) {
  const [lang, setLangState] = useState<Lang>(() => detectLang(projectLang));

  useEffect(() => {
    // Re-run detection when the project-reported language arrives.
    setLangState((prev) => {
      const next = detectLang(projectLang);
      return prev === next ? prev : next;
    });
  }, [projectLang]);

  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    if (typeof window !== "undefined") {
      window.localStorage.setItem("pindoc.ui.lang", l);
    }
  }, []);

  const t = useCallback(
    (key: string, ...args: Array<string | number>) => {
      const bundle = bundles[lang] ?? bundles.en!;
      const template = bundle[key] ?? bundles.en![key] ?? key;
      return interpolate(template, args);
    },
    [lang],
  );

  const value = useMemo(() => ({ lang, setLang, t }), [lang, setLang, t]);
  return <ctx.Provider value={value}>{children}</ctx.Provider>;
}

export function useI18n(): I18nCtx {
  const v = useContext(ctx);
  if (!v) throw new Error("useI18n must be used inside I18nProvider");
  return v;
}
