export type Surface = { slug: string; label: string; path: string };

export const surfaces: { ui_kits: Surface[]; preview: Surface[] } = {
  ui_kits: [
    { slug: "reader",  label: "Wiki Reader",    path: "/design-system/ui_kits/reader/reader.html" },
    { slug: "chrome",  label: "Sidebar Chrome", path: "/design-system/ui_kits/reader/chrome.html" },
    { slug: "tasks",   label: "Task Artifact",  path: "/design-system/ui_kits/reader/tasks.html"  },
  ],
  preview: [
    { slug: "type",              label: "Typography",    path: "/design-system/preview/type.html" },
    { slug: "colors-neutral",    label: "Neutral ramp",  path: "/design-system/preview/colors-neutral.html" },
    { slug: "colors-status",     label: "Status colors", path: "/design-system/preview/colors-status.html" },
    { slug: "spacing",           label: "Spacing",       path: "/design-system/preview/spacing.html" },
    { slug: "iconography",       label: "Iconography",   path: "/design-system/preview/iconography.html" },
    { slug: "components",        label: "Components",    path: "/design-system/preview/components.html" },
    { slug: "logo",              label: "Logo",          path: "/design-system/preview/logo.html" },
    { slug: "agent-avatars",     label: "Agent avatars", path: "/design-system/preview/agent-avatars.html" },
    { slug: "task-pills",        label: "Task pills",    path: "/design-system/preview/task-pills.html" },
  ],
};
