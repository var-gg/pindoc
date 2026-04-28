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
