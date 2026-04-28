export type ReaderSurfaceSegment = "today" | "wiki" | "tasks" | "graph" | "inbox";

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
): string {
  const encodedProject = encodeURIComponent(project);
  const encodedSlug = slug ? `/${encodeURIComponent(slug)}` : "";
  return `/p/${encodedProject}/${surface}${encodedSlug}`;
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
