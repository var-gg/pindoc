export type ReaderSurfaceSegment = "today" | "wiki" | "tasks" | "graph" | "inbox";

export const DEFAULT_READER_ORG_SLUG = "default";

export type ReaderRouteMatch = {
  orgSlug: string;
  projectSlug: string;
  surface: ReaderSurfaceSegment;
  slug?: string;
  legacyProjectRoute: boolean;
};

const canonicalSurfaces = new Set<ReaderSurfaceSegment>([
  "today",
  "wiki",
  "tasks",
  "graph",
  "inbox",
]);

export function normalizeReaderSurfaceSegment(
  segment: string | undefined,
): ReaderSurfaceSegment | null {
  if (!segment) return null;
  if (segment === "task") return "tasks";
  if (canonicalSurfaces.has(segment as ReaderSurfaceSegment)) {
    return segment as ReaderSurfaceSegment;
  }
  return null;
}

export function projectSurfacePath(
  project: string,
  surface: ReaderSurfaceSegment,
  slug?: string,
  orgSlug?: string,
): string {
  const encodedProject = encodeURIComponent(project);
  const encodedSlug = slug ? `/${encodeURIComponent(slug)}` : "";
  const encodedOrg = orgSlug ? `/${encodeURIComponent(orgSlug)}` : "";
  return `${encodedOrg}/p/${encodedProject}/${surface}${encodedSlug}`;
}

export function projectBaseRedirectPath(project: string, orgSlug = DEFAULT_READER_ORG_SLUG): string {
  return projectSurfacePath(project, "today", undefined, orgSlug);
}

export function projectRoutePrefix(project: string, orgSlug?: string): string {
  const encodedProject = encodeURIComponent(project);
  const encodedOrg = orgSlug ? `/${encodeURIComponent(orgSlug)}` : "";
  return `${encodedOrg}/p/${encodedProject}`;
}

export function matchReaderRoutePath(pathname: string): ReaderRouteMatch | null {
  const [pathOnly] = pathname.split(/[?#]/, 1);
  const parts = pathOnly.replace(/^\/+|\/+$/g, "").split("/").filter(Boolean);
  if (parts.length < 3) return null;

  if (parts[0] === "p") {
    const surface = normalizeReaderSurfaceSegment(parts[2]);
    if (!surface) return null;
    return {
      orgSlug: DEFAULT_READER_ORG_SLUG,
      projectSlug: safeDecodeURIComponent(parts[1]),
      surface,
      slug: parts[3] ? safeDecodeURIComponent(parts[3]) : undefined,
      legacyProjectRoute: true,
    };
  }

  if (parts.length >= 4 && parts[1] === "p") {
    const surface = normalizeReaderSurfaceSegment(parts[3]);
    if (!surface) return null;
    return {
      orgSlug: safeDecodeURIComponent(parts[0]),
      projectSlug: safeDecodeURIComponent(parts[2]),
      surface,
      slug: parts[4] ? safeDecodeURIComponent(parts[4]) : undefined,
      legacyProjectRoute: false,
    };
  }

  return null;
}

export function isReaderDevSurfaceEnabled(
  search: string | URLSearchParams | undefined,
  envDev = false,
): boolean {
  if (envDev) return true;
  const params = typeof search === "string"
    ? new URLSearchParams(search.startsWith("?") ? search.slice(1) : search)
    : search;
  return params?.get("dev") === "1";
}

function safeDecodeURIComponent(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}
