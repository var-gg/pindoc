import type { ProjectListItem } from "../src/api/client";
import {
  groupProjectSwitcherProjects,
  projectSwitcherActionItems,
  projectSwitcherKeyboardIndex,
  projectSwitcherOptionID,
} from "../src/reader/projectSwitcherModel";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function project(slug: string, org: string, name = slug): ProjectListItem {
  return {
    id: `${org}-${slug}`,
    slug,
    organization_slug: org,
    name,
    primary_language: "ko",
    artifacts_count: 0,
    created_at: "2026-05-03T00:00:00Z",
  };
}

function testGroupsByOrganizationSlug(): void {
  const groups = groupProjectSwitcherProjects(
    [
      project("tour", "pindoc-sample", "Tour"),
      project("beta", "default", "Beta"),
      project("alpha", "default", "Alpha"),
    ],
    "default",
  );

  assertEqual(groups.length, 2, "two organizations should produce two groups");
  assertEqual(groups[0]?.orgSlug, "default", "default org should sort before pindoc-sample");
  assertEqual(groups[0]?.projects.map((p) => p.slug).join(","), "alpha,beta", "projects sort within org");
  assertEqual(groups[1]?.orgSlug, "pindoc-sample", "second group should be sample org");
}

function testCreateActionIsIndependentFromSwitching(): void {
  const groups = groupProjectSwitcherProjects([project("pindoc", "default")], "default");
  const actions = projectSwitcherActionItems(groups, "default", true);
  const create = actions.at(-1);

  assertEqual(create?.kind, "create", "create affordance should be an action item");
  assertEqual(create?.href, "/projects/new?welcome=1", "create action should open the web form");
}

function testKeyboardIndexContract(): void {
  assertEqual(projectSwitcherKeyboardIndex(-1, 3, "ArrowDown"), 0, "ArrowDown enters first item");
  assertEqual(projectSwitcherKeyboardIndex(-1, 3, "ArrowUp"), 2, "ArrowUp enters last item");
  assertEqual(projectSwitcherKeyboardIndex(1, 3, "ArrowDown"), 2, "ArrowDown advances");
  assertEqual(projectSwitcherKeyboardIndex(1, 3, "ArrowUp"), 0, "ArrowUp retreats");
  assertEqual(projectSwitcherKeyboardIndex(1, 3, "Home"), 0, "Home jumps first");
  assertEqual(projectSwitcherKeyboardIndex(1, 3, "End"), 2, "End jumps last");
  assertEqual(projectSwitcherKeyboardIndex(0, 0, "ArrowDown"), -1, "empty list has no active descendant");
}

function testOptionIDSafelyNormalizesReactIds(): void {
  const id = projectSwitcherOptionID(":r0:", "project-default/pindoc");
  assert(!id.includes(":"), "option id should not include React id punctuation");
  assert(!id.includes("/"), "option id should not include route punctuation");
}

testGroupsByOrganizationSlug();
testCreateActionIsIndependentFromSwitching();
testKeyboardIndexContract();
testOptionIDSafelyNormalizesReactIds();
