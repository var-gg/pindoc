# Pindoc · web (M1 visual skeleton)

Minimal Vite + React + TypeScript shell whose only job (at this milestone) is to
render the Claude Design handoff prototypes locally so we can dogfood the visual
surfaces while the Go MCP server is scaffolded separately.

## Prerequisites

- Node 20+
- pnpm (or npm/yarn — lockfile intentionally omitted at this step)

## Run

```
pnpm install
pnpm dev
```

Opens `http://localhost:5173`. Left nav lists every surface shipped in the
handoff: the three `ui_kits/reader/*.html` prototypes (Wiki Reader, Sidebar
Chrome, Task Artifact) plus the `preview/*.html` design-system cards.

## What this is NOT

This is not production code. The prototypes are loaded in iframes as-is from
`public/design-system/`. React-ifying each surface is a later pass — per the
handoff README, we recreate pixel-perfect in React once the MCP server wire-up
lands, not by cargo-culting the prototype DOM.

## Layout

```
web/
├── index.html                          shell; globally imports design-system CSS
├── public/
│   └── design-system/                  untouched handoff bundle (v0, 2026-04-21)
│       ├── colors_and_type.css         OKLCH tokens, light default
│       ├── fonts/                      Inter (400/500/600) + JetBrains Mono Variable
│       ├── assets/                     logo.svg, wordmark.svg
│       ├── preview/                    8 design-system preview cards
│       └── ui_kits/reader/             the 3 primary surfaces
├── src/
│   ├── main.tsx                        entry
│   ├── App.tsx                         nav shell + iframe preview route
│   ├── surfaces.ts                     surface registry (slug → HTML path)
│   └── styles/shell.css                shell-only CSS; uses tokens, adds none
└── package.json
```

## Do not

- Edit anything under `public/design-system/`. If the bundle needs changes, go
  back to Claude Design, iterate there, re-export via Handoff to Claude Code.
- Introduce new design tokens in `src/styles/`. Everything you need is in
  `colors_and_type.css`. If a token is missing, that's a design-system gap — fix
  it upstream.
- Change the default theme from light. Light mode is documented research-backed
  default for Pindoc's long-form reading surfaces ([decisions.md](../docs/decisions.md)).

## Next

- **M1.5** — React-ify the Wiki Reader surface (stop iframing, own the DOM).
- **M2** — Scaffold the Go MCP server at `cmd/pindoc-server/` and wire the
  Harness install + `pindoc.artifact.propose` tools.
- **M3** — Dogfood on the Pindoc repo itself (the docs under `../docs/` are the
  first artifacts to import).
