import type { ProjectListItem } from "../api/client";
import { projectSurfacePath } from "../readerRoutes";

export type ProjectSwitcherGroup = {
  orgSlug: string;
  projects: ProjectListItem[];
};

export type ProjectSwitcherActionItem = {
  key: string;
  kind: "project" | "create";
  href: string;
};

export function groupProjectSwitcherProjects(
  projects: ProjectListItem[],
  fallbackOrgSlug: string,
): ProjectSwitcherGroup[] {
  const byOrg = new Map<string, ProjectListItem[]>();
  for (const project of projects) {
    const org = project.organization_slug || project.org_slug || fallbackOrgSlug;
    byOrg.set(org, [...(byOrg.get(org) ?? []), project]);
  }
  return Array.from(byOrg, ([orgSlug, groupProjects]) => ({
    orgSlug,
    projects: groupProjects.sort((a, b) => {
      const byName = a.name.localeCompare(b.name);
      return byName === 0 ? a.slug.localeCompare(b.slug) : byName;
    }),
  })).sort((a, b) => a.orgSlug.localeCompare(b.orgSlug));
}

export function projectSwitcherActionItems(
  groups: ProjectSwitcherGroup[],
  fallbackOrgSlug: string,
  projectCreateAllowed: boolean,
): ProjectSwitcherActionItem[] {
  const projectItems = groups.flatMap((group) =>
    group.projects.map((project) => ({
      key: projectSwitcherProjectKey(project),
      kind: "project" as const,
      href: projectSurfacePath(
        project.slug,
        "wiki",
        undefined,
        project.organization_slug || project.org_slug || fallbackOrgSlug,
      ),
    })),
  );
  if (!projectCreateAllowed) return projectItems;
  return [...projectItems, { key: "create-project", kind: "create", href: "/projects/new?welcome=1" }];
}

export function projectSwitcherKeyboardIndex(
  currentIndex: number,
  itemCount: number,
  key: string,
): number {
  if (itemCount <= 0) return -1;
  if (key === "Home") return 0;
  if (key === "End") return itemCount - 1;
  if (key === "ArrowDown") return currentIndex < 0 ? 0 : Math.min(currentIndex + 1, itemCount - 1);
  if (key === "ArrowUp") return currentIndex < 0 ? itemCount - 1 : Math.max(currentIndex - 1, 0);
  return currentIndex;
}

export function projectSwitcherOptionID(instanceID: string, itemKey: string): string {
  return `${sanitizeIDPart(instanceID)}-option-${sanitizeIDPart(itemKey)}`;
}

export function projectSwitcherProjectKey(project: ProjectListItem): string {
  return `project-${project.id || project.slug}`;
}

export function sanitizeIDPart(value: string): string {
  return value.replace(/[^a-zA-Z0-9_-]+/g, "-").replace(/^-+|-+$/g, "") || "project-switcher";
}
