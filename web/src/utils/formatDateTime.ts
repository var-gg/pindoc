import type { Lang } from "../i18n";

const LOCALE_TAGS: Record<Lang, string> = {
  en: "en-US",
  ko: "ko-KR",
};

export function localeTag(lang: Lang): string {
  return LOCALE_TAGS[lang] ?? LOCALE_TAGS.en;
}

export function formatDateTime(d: Date | string, lang: Lang): string {
  return new Date(d).toLocaleString(localeTag(lang));
}

export function formatDate(d: Date | string, lang: Lang): string {
  return new Date(d).toLocaleDateString(localeTag(lang));
}

export function formatTime(d: Date | string, lang: Lang): string {
  return new Date(d).toLocaleTimeString(localeTag(lang));
}

export function formatNumber(value: number, lang: Lang): string {
  return new Intl.NumberFormat(localeTag(lang)).format(value);
}
