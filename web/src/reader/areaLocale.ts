// localizedAreaName — UI-locale translation of an Area name / slug.
// When the i18n bundle has an `area.{slug}` key, return its translation;
// otherwise fall back to the caller-provided label so custom Areas (or
// slugs the bundle hasn't seen yet) still render cleanly. Detection of
// a missing key relies on the fact that web/src/i18n/index.tsx returns
// the key itself when no translation exists.
//
// Shared between Sidebar (renders Area rows) and CmdK (renders search
// hits), so both surfaces display the same user-visible label for the
// same slug — Task task-area-name-i18n acceptance on Cmd+K consistency.
export function localizedAreaName(
  t: (key: string) => string,
  slug: string,
  fallback: string,
): string {
  const key = `area.${slug}`;
  const translated = t(key);
  return translated === key ? fallback : translated;
}
