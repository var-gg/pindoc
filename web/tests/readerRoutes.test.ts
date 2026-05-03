import {
  DEFAULT_READER_ORG_SLUG,
  isReaderDevSurfaceEnabled,
  matchReaderRoutePath,
  normalizeReaderSurfaceSegment,
  projectBaseRedirectPath,
  projectSurfacePath,
} from "../src/readerRoutes";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testTaskAliasNormalizesToTasks(): void {
  assertEqual(
    normalizeReaderSurfaceSegment("task"),
    "tasks",
    "singular task surface should map to canonical tasks",
  );
}

function testUnknownSurfaceFallsThrough(): void {
  assertEqual(
    normalizeReaderSurfaceSegment("__nope__"),
    null,
    "unknown project surfaces should be handled by fallback UI",
  );
}

function testProjectSurfacePathPreservesCanonicalTasks(): void {
  assertEqual(
    projectSurfacePath("pindoc", "tasks"),
    "/p/pindoc/tasks",
    "tasks board route",
  );
  assertEqual(
    projectSurfacePath("pindoc", "tasks", "task-a"),
    "/p/pindoc/tasks/task-a",
    "task detail route",
  );
}

function testProjectSurfacePathAddsOrgContextWhenPresent(): void {
  assertEqual(
    projectSurfacePath("pindoc", "wiki", "artifact-a", "curioustore"),
    "/curioustore/p/pindoc/wiki/artifact-a",
    "org-scoped wiki detail route",
  );
  assertEqual(
    projectSurfacePath("pin doc", "wiki", "a/b", "curio store"),
    "/curio%20store/p/pin%20doc/wiki/a%2Fb",
    "org-scoped route segments should be encoded independently",
  );
}

function testProjectBaseRedirectPathPreservesCanonicalOrg(): void {
  assertEqual(
    projectBaseRedirectPath("pindoc", "default"),
    "/default/p/pindoc/today",
    "canonical project base should redirect to the same org-scoped today surface",
  );
}

function testProjectBaseRedirectPathAddsDefaultOrgForLegacyBase(): void {
  assertEqual(
    projectBaseRedirectPath("pindoc"),
    "/default/p/pindoc/today",
    "legacy project base should redirect to default org-scoped today surface",
  );
}

function testReaderRouteMatchAcceptsOrgScopedAndLegacyPaths(): void {
  const orgScoped = matchReaderRoutePath("/curioustore/p/pindoc/wiki/artifact-a");
  assertEqual(orgScoped?.orgSlug, "curioustore", "org-scoped route org slug");
  assertEqual(orgScoped?.projectSlug, "pindoc", "org-scoped route project slug");
  assertEqual(orgScoped?.surface, "wiki", "org-scoped route surface");
  assertEqual(orgScoped?.slug, "artifact-a", "org-scoped route artifact slug");
  assertEqual(orgScoped?.legacyProjectRoute, false, "org-scoped route should not be legacy");

  const legacy = matchReaderRoutePath("/p/pindoc/wiki/artifact-a");
  assertEqual(legacy?.orgSlug, DEFAULT_READER_ORG_SLUG, "legacy route default org fallback");
  assertEqual(legacy?.projectSlug, "pindoc", "legacy route project slug");
  assertEqual(legacy?.surface, "wiki", "legacy route surface");
  assertEqual(legacy?.slug, "artifact-a", "legacy route artifact slug");
  assertEqual(legacy?.legacyProjectRoute, true, "legacy route marker");
}

function testDevSurfaceGateRequiresDevQueryInProduction(): void {
  assertEqual(
    isReaderDevSurfaceEnabled("", false),
    false,
    "production default hides dev-only surfaces",
  );
  assertEqual(
    isReaderDevSurfaceEnabled("?dev=1", false),
    true,
    "explicit dev query opens dev-only surfaces",
  );
  assertEqual(
    isReaderDevSurfaceEnabled("", true),
    true,
    "vite dev server opens dev-only surfaces",
  );
}

testTaskAliasNormalizesToTasks();
testUnknownSurfaceFallsThrough();
testProjectSurfacePathPreservesCanonicalTasks();
testProjectSurfacePathAddsOrgContextWhenPresent();
testProjectBaseRedirectPathPreservesCanonicalOrg();
testProjectBaseRedirectPathAddsDefaultOrgForLegacyBase();
testReaderRouteMatchAcceptsOrgScopedAndLegacyPaths();
testDevSurfaceGateRequiresDevQueryInProduction();
