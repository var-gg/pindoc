export type SurfaceKind = "ui_kit" | "preview";

export type Surface = {
  slug: string;
  label: string;
  sublabel?: string;
  path: string;        // static HTML path under /public/design-system/
  kind: SurfaceKind;
};

export const uiKits: Surface[] = [
  {
    slug: "reader",
    label: "Wiki Reader",
    sublabel: "full product screen · Wiki / Inbox / Graph tabs + ⌘K",
    path: "/design-system/ui_kits/reader/reader.html",
    kind: "ui_kit",
  },
  {
    slug: "chrome",
    label: "Sidebar Chrome",
    sublabel: "isolated view of the Project Switcher + Area tree",
    path: "/design-system/ui_kits/reader/chrome.html",
    kind: "ui_kit",
  },
  {
    slug: "tasks",
    label: "Task Artifact",
    sublabel: "Task detail + list variant (type-specific chrome)",
    path: "/design-system/ui_kits/reader/tasks.html",
    kind: "ui_kit",
  },
];

export const previews: Surface[] = [
  { slug: "type",           label: "Typography",    path: "/design-system/preview/type.html",           kind: "preview" },
  { slug: "colors-neutral", label: "Neutral ramp",  path: "/design-system/preview/colors-neutral.html", kind: "preview" },
  { slug: "colors-status",  label: "Status colors", path: "/design-system/preview/colors-status.html",  kind: "preview" },
  { slug: "spacing",        label: "Spacing",       path: "/design-system/preview/spacing.html",        kind: "preview" },
  { slug: "iconography",    label: "Iconography",   path: "/design-system/preview/iconography.html",    kind: "preview" },
  { slug: "components",     label: "Components",    path: "/design-system/preview/components.html",     kind: "preview" },
  { slug: "logo",           label: "Logo",          path: "/design-system/preview/logo.html",           kind: "preview" },
  { slug: "agent-avatars",  label: "Agent avatars", path: "/design-system/preview/agent-avatars.html",  kind: "preview" },
  { slug: "task-pills",     label: "Task pills",    path: "/design-system/preview/task-pills.html",     kind: "preview" },
];

export const allSurfaces = [...uiKits, ...previews];

export function findSurface(slug: string | undefined): Surface | null {
  if (!slug) return null;
  return allSurfaces.find((s) => s.slug === slug) ?? null;
}
