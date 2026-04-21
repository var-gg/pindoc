# Pindoc · Design Skill

When asked to design anything inside Pindoc — the agent-writable, human-approvable project wiki — follow these rules. This file is the compact, agent-invocable version of `README.md`. Read both before starting.

## Product in one line
Pindoc is a typed, graph-linked project wiki that **humans never edit directly**. Agents (Claude Code, Cursor, Codex) write over MCP; humans review, approve, reject, and read.

## Design DNA (in priority order)
1. **Developer tool at rest.** Linear-density lists. Obsidian-spacious reader. GitHub-PR review flow.
2. **Dark mode is Day 1.** Light mode is `:root.theme-light`, retuned from the same tokens.
3. **Borders define surfaces, not shadows.** Shadows appear only on floating UI (⌘K, menus, modals, toasts).
4. **Status color is the only saturated color.** `draft` amber · `live` emerald · `stale` rose · `archived` zinc · `rejected` red. Accent indigo is restrained — focus, selection, one primary action per view.
5. **Monospace is a signal.** JetBrains Mono marks the typed/code-adjacent: artifact IDs, type chips (`ADR`, `FEATURE`), area labels (`platform/auth`), agent names.

## Do NOT design
- WYSIWYG editors, rich-text toolbars, inline edit pencils. Humans do not write.
- Decorative illustrations, marketing hero art, gradient-heavy cards.
- Bluish-purple SaaS gradients. Capsule pills (`9999px`). Emoji in chrome.
- Welcoming/cheerful copy (`Welcome!`, `Let's get started ✨`, `Awesome!`).

## Tokens
Import `colors_and_type.css`. Use the variables — never hard-code hex or px from the palette. Spacing scale `4 · 8 · 12 · 16 · 20 · 24 · 32 · 48 · 64`. Radii `2 · 4 · 6 · 8`.

## Density rule
Two densities, by surface: **compact** (lists, sidebar, queue, graph inspector — 32px rows, 13px text) and **spacious** (reader body — 16px / 1.6). Never mix within one pane.

## Voice
Technical, direct, lowercase by default for status/actions (`draft`, `promote`, `approve`). Title Case only for proper nouns, primary buttons, and headings. No exclamation marks. No adjectives that sell.

## Iconography
Lucide, 1.5 stroke, `currentColor`. Icons are signal, never decoration. No emoji in chrome. Flag for the user if a custom Pindoc icon library exists and swap.

## Imagery
Pindoc has almost no imagery. Exceptions: the logo mark; agent avatars (monogram tiles, fixed 6-color palette, 4px radius — humans get circles, agents get squares).

## Components to reuse
See `preview/components.html` for canonical buttons, chips, inputs, artifact rows, reader headers, and cards. See `ui_kits/reader/reader.html` for the three primary surfaces (Wiki, Inbox, Graph) and the ⌘K palette.

## When in doubt
- Ask what exists in the real codebase. This system was built from a brief alone, not from source.
- Pick the pattern closer to Linear/Obsidian/GitHub PR, not the one closer to Notion/Confluence.
- Strip, don't add. The feature probably doesn't need an icon, a header, or a banner.
