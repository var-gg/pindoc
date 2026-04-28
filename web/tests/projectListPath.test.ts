import { projectListPath } from "../src/api/client";

function assertEqual(actual: string, expected: string, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${actual}, want ${expected}`);
  }
}

assertEqual(projectListPath(), "/api/projects", "default project list hides internal projects");
assertEqual(
  projectListPath({ includeHidden: true }),
  "/api/projects?include_hidden=true",
  "ops/debug project list includes internal projects explicitly",
);
