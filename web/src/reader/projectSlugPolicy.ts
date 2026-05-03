import policy from "./projectSlugPolicy.json";

export type ProjectCreateField = "slug" | "name" | "language";

export type ProjectCreateErrorCode =
  | "SLUG_INVALID"
  | "SLUG_RESERVED"
  | "SLUG_TAKEN"
  | "NAME_REQUIRED"
  | "LANG_REQUIRED"
  | "LANG_INVALID"
  | "GIT_REMOTE_URL_INVALID"
  | "BAD_JSON"
  | "INTERNAL_ERROR"
  | "UNKNOWN";

export const PROJECT_SLUG_PATTERN = policy.pattern;
export const PROJECT_SLUG_HTML_PATTERN = policy.htmlPattern;
export const PROJECT_RESERVED_SLUGS = policy.reservedSlugs;

const projectSlugRe = new RegExp(PROJECT_SLUG_PATTERN);
const reservedSlugs = new Set(PROJECT_RESERVED_SLUGS);

const fieldByErrorCode: Partial<Record<ProjectCreateErrorCode, ProjectCreateField>> = {
  SLUG_INVALID: "slug",
  SLUG_RESERVED: "slug",
  SLUG_TAKEN: "slug",
  NAME_REQUIRED: "name",
  LANG_REQUIRED: "language",
  LANG_INVALID: "language",
};

const knownProjectCreateErrors = new Set<ProjectCreateErrorCode>([
  "SLUG_INVALID",
  "SLUG_RESERVED",
  "SLUG_TAKEN",
  "NAME_REQUIRED",
  "LANG_REQUIRED",
  "LANG_INVALID",
  "GIT_REMOTE_URL_INVALID",
  "BAD_JSON",
  "INTERNAL_ERROR",
  "UNKNOWN",
]);

export function normalizeProjectCreateErrorCode(code: string | null | undefined): ProjectCreateErrorCode {
  if (code && knownProjectCreateErrors.has(code as ProjectCreateErrorCode)) {
    return code as ProjectCreateErrorCode;
  }
  return "UNKNOWN";
}

export function projectCreateErrorKey(code: string | null | undefined): string {
  return `new_project.error.${normalizeProjectCreateErrorCode(code)}`;
}

export function projectCreateErrorMessage(
  t: (key: string, ...args: Array<string | number>) => string,
  code: string | null | undefined,
): string {
  return t(projectCreateErrorKey(code));
}

export function fieldForProjectCreateError(code: string | null | undefined): ProjectCreateField | null {
  return fieldByErrorCode[normalizeProjectCreateErrorCode(code)] ?? null;
}

export function validateProjectSlugInput(rawSlug: string): ProjectCreateErrorCode | null {
  const slug = rawSlug.trim();
  if (slug === "") return null;
  if (!projectSlugRe.test(slug)) return "SLUG_INVALID";
  if (reservedSlugs.has(slug)) return "SLUG_RESERVED";
  return null;
}

export function isProjectCreateSubmitDisabled(input: {
  slug: string;
  name: string;
  primaryLanguage: string;
  submitting: boolean;
}): boolean {
  return (
    input.submitting ||
    input.slug.trim() === "" ||
    input.name.trim() === "" ||
    input.primaryLanguage.trim() === "" ||
    validateProjectSlugInput(input.slug) !== null
  );
}
