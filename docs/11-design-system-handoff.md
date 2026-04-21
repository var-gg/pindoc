# Design System Handoff — v0 (2026-04-21)

First Claude Design export landed. Everything imported under
`web/public/design-system/`, unmodified, plus a minimal Vite + React + TS shell
at `web/` so the prototypes render in a local browser.

## Source

- Claude Design project: `Pindoc Design System` (created 2026-04-21).
- Handoff bundle URL: `https://api.anthropic.com/v1/design/h/MGZ7pe-WHpR9mtmxYzNlzA`
  (fetched via WebFetch during this session; 261 KB gzip, 445 KB uncompressed).
- Bundle contents as exported — see [web/public/design-system/README.md](../web/public/design-system/README.md)
  and [web/public/design-system/SKILL.md](../web/public/design-system/SKILL.md)
  for the design-side brief.

## What was imported

```
web/public/design-system/
├── README.md                    agent-facing brief (voice, anti-patterns, tokens)
├── SKILL.md                     compact agent skill shim
├── colors_and_type.css          OKLCH tokens — light default, dark via .theme-dark
├── fonts/                       Inter (400/500/600) + JetBrains Mono Variable (woff2)
├── assets/                      logo.svg, wordmark.svg
├── preview/                     8 design-system preview cards
│   ├── type.html
│   ├── colors-neutral.html
│   ├── colors-status.html
│   ├── spacing.html
│   ├── iconography.html
│   ├── components.html
│   ├── logo.html
│   ├── agent-avatars.html
│   └── task-pills.html
└── ui_kits/reader/
    ├── reader.html              Wiki Reader · Inbox · Graph (three surfaces + ⌘K)
    ├── chrome.html              Project Switcher + Area tree sidebar shell
    └── tasks.html               Task artifact detail + list view
```

## What was scaffolded

- [web/package.json](../web/package.json), Vite config, two `tsconfig.*`, `index.html`,
  `src/main.tsx`, `src/App.tsx`, `src/surfaces.ts`, `src/styles/shell.css`.
- Shell loads `colors_and_type.css` globally (`<link>` in `index.html`) so the
  React shell itself uses Pindoc tokens — no shadow design system.
- Nav lists every surface from the bundle. Each opens the original HTML in an
  iframe. No React re-implementation yet.

## How to preview

```
cd web && pnpm install && pnpm dev
```

`http://localhost:5173` opens with the Wiki Reader link at the top of the nav.

## Notes the designer flagged to the user

Direct from [web/public/design-system/README.md](../web/public/design-system/README.md):

> **FLAG for user:** If Pindoc has a real icon library (custom SVG set, icon
> font, sprite), swap Lucide for it and retune stroke/size.

> The design system was built from the product brief alone — no codebase, Figma,
> or existing prototype was attached. Treat it as a **v0 visual proposal** ready
> for grounding against real product code when it exists.

## Boundaries

- `web/public/design-system/` is **generated output**. Edits go back to Claude
  Design, not here.
- `web/src/` may grow. Once we React-ify a surface (M1.5), the iframe route for
  that slug is replaced with a real route, and the raw HTML stays as reference.
- The Go MCP server is **out of scope for this handoff**. It lands at
  `cmd/pindoc-server/` (empty today) in M2.

## Alignment with decisions

- Embedding, MCP-client scope, and conflict threshold are already resolved in
  [decisions.md](decisions.md) — none of them affect this handoff.
- Task schema was confirmed in [04-data-model.md](04-data-model.md) before the
  Task Artifact surface was generated, so the prototype matches the spec
  (status pill, priority chip, blocked-by warning, acceptance checklist,
  agent-attempts timeline, detail + list toggle).
