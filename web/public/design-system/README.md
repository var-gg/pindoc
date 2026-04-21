# Pindoc Design System

> The wiki you never type into.

## Product context

**Pindoc** is an agent-writable, human-approvable, typed & graph-linked project wiki. The core inversion of the category: **humans never edit documents directly**. All writes flow through AI coding agents (Claude Code, Cursor, Codex) over MCP. Humans review, approve, reject, and read.

### Core primitives
- **Project** — the top-level container; a codebase or product surface.
- **Harness** — the connection between a project and its authoring agents. Defines what agents can write, to where, under what rules.
- **Promote** — the act of moving an artifact from `draft` → `live` (or `live` → `archived`). Mirrors git/PR mental model.
- **Artifact** — a typed document node: `Feature`, `ADR`, `Debug`, `Runbook`, `Spec`, etc. Every artifact has a type, an area, an agent author, and a status.
- **Graph** — artifacts are nodes; edges are typed relationships (`supersedes`, `depends-on`, `mentions`, `implements`). First-class view, not a back-of-the-napkin feature.

### Primary surfaces
1. **Wiki Reader** — the home screen. Spacious article view with backlinks, type chip, agent provenance, graph sidecar. Obsidian / GitBook feel.
2. **Inbox / Review queue** — pending agent writes awaiting human approval. GitHub PR review mental model — diff, comment, approve, reject, request changes.
3. **Graph view** — zoomable force-directed graph of artifacts and typed edges. First-class, not hidden.
4. **Command Palette (⌘K)** — every action. Navigate, approve, promote, filter, open graph, invoke agent. Linear-grade keyboard.

### Design inspirations
- **Linear** — speed, density, keyboard-first, ⌘K.
- **Obsidian** — reader-first, backlinks, graph.
- **GitHub PR review** — approve / reject / request changes flow, diff gutter, reviewer avatars.
- **Muji / minimalism** — neutral base, restraint, no decorative illustration.

### Explicit anti-patterns (do NOT design these)
- WYSIWYG editors or rich-text toolbars. Pindoc is **read-only to humans**.
- Inline edit pencils, "click to edit" affordances.
- Drag-drop reordering of documents.
- Decorative illustrations, marketing-style hero images, gradient-heavy cards.
- Emoji as UI — allowed sparingly in agent-authored body copy; never in chrome.
- Bluish-purple SaaS gradients.

## Source materials

This design system was built from the product brief alone — no codebase, Figma, or existing prototype was attached. Treat it as a **v0 visual proposal** ready for grounding against real product code when it exists.

- Brand description: see inline notes in `/CLAUDE.md` (if added) or the original chat.
- No Figma link provided.
- No codebase attached.
- No slide deck provided.

If/when real product code, Figma, or brand assets become available, the next pass should re-verify: (1) logo/wordmark, (2) exact type stack, (3) exact color ramps, (4) component-by-component recreation in `ui_kits/`.

---

## Index

Root files:
- `README.md` — this file.
- `SKILL.md` — agent-invocable skill shim (cross-compatible with Claude Code skills).
- `colors_and_type.css` — all design tokens in **OKLCH**. Light mode is default; dark mode via `:root.theme-dark`, persisted to `localStorage` under key `pindoc.theme`.

Folders:
- `fonts/` — webfonts (Inter for UI, JetBrains Mono for type/area labels).
- `assets/` — logo, wordmark, agent avatars, icon notes.
- `preview/` — static HTML cards that populate the **Design System** tab.
- `ui_kits/reader/` — the Wiki Reader UI kit. Click-thru prototype of the primary surface with Review Queue, Graph, and ⌘K.
- `ui_kits/reader/tasks.html` — Task artifact: detail view and list view. Own rendering rules (status pills, priority chips, acceptance-criteria checklist, agent-attempts timeline) on top of the shared artifact header.

---

## Content fundamentals

**Voice: technical, honest, minimal.** Pindoc speaks like a senior engineer writing a README for other engineers. It is not cheerful, not friendly, not explanatory. It assumes you know what a typed graph is and what an ADR is. It does not over-explain.

**Tone rules**
- **Direct.** "Promote" not "Publish to production". "Reject" not "Kindly decline".
- **Lowercase by default** in UI microcopy and status chips: `draft`, `live`, `stale`, `archived`, `promote`, `approve`, `reject`. Title Case only for proper nouns, primary buttons, and headings.
- **No marketing adjectives.** No "powerful", "seamless", "effortless", "magical". If a feature needs an adjective to sell, it isn't ready.
- **"You" sparingly.** Pindoc addresses a reader in docs ("You can filter by type"), but chrome and empty states prefer impersonal ("No drafts pending") or imperative ("Filter by area").
- **No emoji in chrome.** Allowed in agent-authored body copy if the agent chose to use them.
- **No exclamation marks.** Ever. In chrome.
- **Numbers are numerals.** `3 pending`, not `three pending`.

**Status vocabulary** (canonical, do not synonymize)
- `draft` — written by agent, not yet approved.
- `live` — approved, current truth.
- `stale` — live but flagged by an agent as potentially out of date.
- `archived` — intentionally retired; kept for history.
- `rejected` — proposed and declined; lives in audit log only.

**Artifact type labels** (always monospace, always uppercase or title case in mono)
- `FEATURE` · `ADR` · `DEBUG` · `RUNBOOK` · `SPEC` · `POSTMORTEM` · `NOTE`

**Microcopy examples**
- Empty inbox: `No drafts pending. Agents are quiet.`
- Empty search: `No artifacts match "foo".`
- Approve confirm: `Promote to live?`
- Reject confirm: `Reject and close? This cannot be undone.`
- Graph empty: `Graph is empty. Promote an artifact to populate.`
- Error: `Could not reach harness. Retry?`
- Agent provenance: `written by claude-code · 2h ago`

**Never write**
- "Welcome to Pindoc!"
- "Let's get started ✨"
- "Oops, something went wrong"
- "Awesome, you're all set"

---

## Visual foundations

### Core motif
The screen looks like a **developer tool at rest**. Neutral zinc/slate base, dense horizontal rules where information density earns them, generous air around the article body, and precise status color only where status is being communicated. Nothing is purely decorative.

### Color system
- **OKLCH throughout.** Perceptual uniformity — equal `L` values feel equal, a hue rotation doesn't drift lightness. Matches Linear (migrated from HSL → LCH for the same reason), Radix, Tailwind v4.
- **Light is the default.** 2024–25 research (NN/g; MDPI) consistently shows light mode outperforms dark for long-form reading, proofreading, and cognitive tasks in lit environments. Wiki Reader is long-form → light default is the correct call.
- **Dark stays first-class.** Toggle in the top nav. User choice is persisted to `localStorage` (`pindoc.theme`) and marked on `<html data-theme-source="user">` so we stop deferring to `prefers-color-scheme`. Pre-paint bootstrap prevents flash.
- **Neutral ramp** is 9 steps per theme. Light: warm paper (`bg-0 L98.5`) → deep ink (`fg-0 L16`). Dark: deep slate (`bg-0 L16`) → near-white (`fg-0 L97`).
- **Status colors** are the only saturated colors in the product. One per status: draft (amber), live (emerald), stale (rose), archived (zinc), rejected (red). Each retuned per-theme to hit contrast targets (below).
- **Accent** is a single restrained indigo — focus rings, selection, primary action. Never in backgrounds, never in gradients.

### Contrast — APCA, not WCAG 2.x
We target **APCA Lc** (WCAG 3 candidate), not the legacy 4.5:1 ratio. APCA measures perceived lightness contrast and corrects WCAG 2's known overstatement of dark-color contrast and understatement of mid-tones.
- **Body text:** `Lc ≥ 60`
- **Small text & labels (≤ 13px):** `Lc ≥ 75`
- **Large headings (≥ 24px/600):** `Lc ≥ 45` acceptable

All body and UI tokens in both themes meet these. Spot-checks annotated inline in `colors_and_type.css` and on `preview/colors-neutral.html`.

### Typography
- **UI sans**: Inter. Weights 400 / 500 / 600. Letter-spacing slightly negative on headings (`-0.01em`), 0 on body.
- **Mono**: JetBrains Mono. Used for: type chips (`FEATURE`, `ADR`), area tags (`platform/auth`), agent IDs (`claude-code@v2.1`), inline code, diff, artifact IDs. Monospace is a **signal** that something is typed and code-adjacent.
- **Reader serif**: none. We considered a reading serif for the article body but committed to Inter throughout — the dev-tool coherence matters more than a reading feel. The spacious reader layout (wider margins, 66ch measure, 1.6 leading) does the work instead.
- **Scale**: 12 / 13 / 14 / 16 / 18 / 20 / 24 / 32 / 48.
- **Body**: 16px / 1.6 line-height / `--fg-1`.
- **Reader measure**: `max-width: 66ch`. This sits inside the 45–75ch readability band documented by Baymard and Matthew Butterick's *Practical Typography*. 66ch is the sweet spot — wide enough to avoid ragged line-breaks on technical prose (long identifiers, code spans), narrow enough to keep saccade distance low.

### Density
Two densities coexist, by surface:
- **Compact** (lists, queue, sidebar, graph inspector): 32px row, 13px text. Linear-grade.
- **Spacious** (reader article body): 16px text, 1.6 lh, generous margins. Obsidian-grade.
Never mix densities in one pane.

### Spacing
4px base unit. Scale: `4 · 8 · 12 · 16 · 20 · 24 · 32 · 48 · 64`. No half-steps. No arbitrary values.

### Corner radii
- `2px` — chips, status dots.
- `4px` — inputs, buttons, rows.
- `6px` — cards, menus, modals.
- `8px` — the ⌘K palette and media wells.
- No `border-radius: 9999px`. No capsules. Capsules feel consumer; Pindoc is a tool.

### Borders vs shadows
- **Borders do the work.** 1px `--border` separators everywhere. Borders define surfaces, not shadows.
- **Shadows** are reserved for floating surfaces only: ⌘K palette, menus, modals, toasts. Single elevation. No nested shadows.
- `--shadow-float`: `0 8px 24px -12px rgba(0,0,0,0.6), 0 2px 6px -2px rgba(0,0,0,0.4)` in dark.

### Backgrounds
- Flat. No gradients in product chrome.
- No images, no textures, no patterns in product chrome.
- One exception: the graph view may fade canvas edges to `--bg-0` to imply infinity.

### Animation
- Minimal and short. 120ms for state changes (hover, press). 200ms for entrances (palette, toast, menu). 0ms for focus ring appearance — no delay on keyboard feedback.
- Easing: `cubic-bezier(0.2, 0, 0, 1)` for entrances, linear for hover.
- No bounce. No spring. No parallax.
- Status dots have a 1.2s slow pulse when `draft` (pending approval) — the one animated signal in the whole product.

### Hover / press states
- Hover: background lightens by ~4% (`--bg-2` → `--bg-3`). No color shift, no shadow, no scale.
- Press: background lightens by ~6% and a 1px inset top border appears. No scale transform.
- Focus: 2px `--accent` ring with 2px offset. Always keyboard-visible.

### Transparency / blur
- Blur is used in exactly two places: (1) the ⌘K palette backdrop (16px blur over a 40% dim), and (2) sticky table headers (8px blur). Nowhere else.
- Transparency in borders: no. Borders are solid.
- Overlays for modals: `rgba(0,0,0,0.5)`, no blur.

### Cards
- Thin `1px solid --border` on `--bg-1`. No shadow. No radius above 6px.
- A card is a **container with a border**, not a lifted surface.

### Imagery
Pindoc has almost no imagery by design. Exceptions:
- **Agent avatars** — small (16/20/24px) monogram tiles with a per-agent color from a fixed 6-color palette. Not photos, not mascots.
- **Logo mark** — a single geometric glyph (see `assets/logo.svg`).

### Fixed elements (desktop ≥ 1024px)
- Top nav: 48px, sticky, `--bg-1`, 1px bottom border.
- Left sidebar: 256px, `--bg-0`, 1px right border, collapsible to 0.
- Right graph sidecar: 320px, `--bg-1`, 1px left border, collapsible.
- Command palette: centered, 640px wide, 8px radius, float shadow.

### Responsive breakpoints (Day 1)
- **Desktop 1440** — primary design target. Three-pane layout (sidebar · content · sidecar).
- **Tablet 1024 / 768** — sidebar collapses to a slide-out drawer via hamburger; sidecar is hidden. Reader stays 66ch centered.
- **Mobile 375** — read-only Wiki Reader. Sidebar is a drawer. ⌘K becomes a **bottom-sheet search** (full-width, slides up from the bottom, 16px input to prevent iOS zoom). Inbox approve/reject collapses into a **sticky bottom action bar** with 44px hit targets; inline per-row action buttons are suppressed in favor of the bar. Tab labels collapse to icon-only; nav brand wordmark drops.
- **44px minimum touch target** on mobile for all interactive controls.

### Iconography
See the ICONOGRAPHY section below.

---

## Iconography

**System: Lucide Icons** (CDN), 16px default / 20px in larger chrome / 14px inside chips. 1.5px stroke. No fills.

Pindoc's brief is thin-stroke, precise, geometric — Lucide matches that exactly (and is developer-tool standard, used by Linear, Supabase, shadcn/ui). We did not find a Pindoc icon font in any attached materials because no materials were attached, so this is a substitution to flag for the user.

**FLAG for user:** If Pindoc has a real icon library (custom SVG set, icon font, sprite), swap Lucide for it and retune stroke/size.

**Icon usage rules**
- Icons are **signal, not decoration.** Every icon must communicate a specific action or type. No decorative icons in headings, empty states, or cards.
- **No emoji in UI.** Agent-authored body copy may contain emoji if the agent chose to.
- **No unicode glyphs as icons** (no ✓ ✕ → arrows). Use Lucide equivalents.
- Icons inherit `currentColor`. They do not carry their own color.
- Icons sit next to labels, not replace them, in primary chrome — except in compact toolbars where a tooltip is mandatory.

**Status dots** are not icons — they are 6px `border-radius: 50%` filled circles colored by status token. See `preview/status-dots.html`.

**Agent avatars** are not icons — they are monogram tiles. See `preview/agent-avatars.html`.

---
