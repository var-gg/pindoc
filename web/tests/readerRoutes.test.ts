import {
  normalizeReaderSurfaceSegment,
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

testTaskAliasNormalizesToTasks();
testUnknownSurfaceFallsThrough();
testProjectSurfacePathPreservesCanonicalTasks();
