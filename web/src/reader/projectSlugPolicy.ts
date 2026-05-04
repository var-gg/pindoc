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
export const PROJECT_CREATE_WEB_LANGUAGES = ["en", "ko"] as const;

const projectSlugRe = new RegExp(PROJECT_SLUG_PATTERN);
const reservedSlugs = new Set(PROJECT_RESERVED_SLUGS);
const webPrimaryLanguages = new Set<string>(PROJECT_CREATE_WEB_LANGUAGES);
const reservedSlugCategories = {
  admin: "system",
  api: "system",
  app: "system",
  www: "system",
  public: "system",
  static: "system",
  assets: "system",
  health: "system",
  new: "system",
  home: "system",
  index: "system",
  p: "system",
  design: "system",
  ui: "system",
  preview: "system",
  login: "auth",
  signup: "auth",
  logout: "auth",
  auth: "auth",
  billing: "billing",
  pricing: "billing",
  docs: "docs",
  help: "docs",
  blog: "docs",
  about: "docs",
  terms: "docs",
  privacy: "docs",
  security: "docs",
  support: "service",
  status: "service",
  mail: "service",
  contact: "service",
  dashboard: "workspace",
  settings: "workspace",
  wiki: "workspace",
  tasks: "workspace",
  graph: "workspace",
  inbox: "workspace",
} as const satisfies Record<(typeof PROJECT_RESERVED_SLUGS)[number], string>;

export type ProjectReservedSlugCategory = (typeof reservedSlugCategories)[keyof typeof reservedSlugCategories];

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

export function projectReservedSlugCategory(rawSlug: string): ProjectReservedSlugCategory | null {
  const slug = rawSlug.trim().toLowerCase();
  return reservedSlugs.has(slug) ? reservedSlugCategories[slug as keyof typeof reservedSlugCategories] : null;
}

export function projectCreateErrorKey(
  code: string | null | undefined,
  options?: { slug?: string },
): string {
  const normalized = normalizeProjectCreateErrorCode(code);
  if (normalized === "SLUG_RESERVED" && options?.slug) {
    const category = projectReservedSlugCategory(options.slug);
    if (category) return `new_project.error.SLUG_RESERVED.${category}`;
  }
  return `new_project.error.${normalized}`;
}

export function projectCreateErrorMessage(
  t: (key: string, ...args: Array<string | number>) => string,
  code: string | null | undefined,
  options?: { slug?: string },
): string {
  return t(projectCreateErrorKey(code, options));
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
    !webPrimaryLanguages.has(input.primaryLanguage.trim()) ||
    validateProjectSlugInput(input.slug) !== null
  );
}
