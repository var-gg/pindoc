---
project_slug: pindoc
project_id: ddbbfa62-4511-41c2-af07-110f534fb6e4
locale: ko
schema_version: 1
---

# PINDOC.md - Harness for Pindoc

<!-- Generated for task-pindoc-md-frontmatter dogfood. Re-run pindoc.harness.install after the server restarts on the new renderer. -->

This file is the Pindoc Harness for this workspace. Agents load it at
session start through `AGENTS.md` and must keep Pindoc Task state aligned
with code work.

## Project

- slug: pindoc
- id: ddbbfa62-4511-41c2-af07-110f534fb6e4
- primary_language: ko
- canonical Reader URL: `/p/pindoc/wiki`
- MCP scope: pass `project_slug="pindoc"` on project-scoped tool calls.

## Session Pre-flight

Before starting non-trivial work:

1. Call `pindoc.project.current(project_slug="pindoc")` once to pin scope.
2. Check assigned work with `pindoc.task.queue(project_slug="pindoc", assignee="agent:codex")`.
3. Read the target Task context from Pindoc before editing code.
4. Use `pindoc.area.list(project_slug="pindoc")` before creating artifacts; never invent an `area_slug`.
5. For create-path `pindoc.artifact.propose`, call `pindoc.context.for_task` or `pindoc.artifact.search` first and pass the same-session `basis.search_receipt`.

## Task Work Protocol

- Keep Task body acceptance checkboxes and `task_meta.status` consistent.
- Mark acceptance items with `pindoc.artifact.propose(shape="acceptance_transition")`.
- Leave `task_meta.status="open"` while implementation or validation is incomplete.
- Move to `claimed_done` only after all in-scope acceptance criteria are done and validation has been attempted.
- If a validation step is blocked by local environment, mark the acceptance item `[~]` with a concrete reason instead of claiming full completion.

## Section 12 - Task lifecycle (chip / parallel work)

When this project's agent spawns a worktree-based chip / parallel sub-session:

### Before spawn

- Ensure a Pindoc Task artifact exists for this work. If not, create via
  `pindoc.artifact.propose` with `task_meta.status="open"` and
  `assignee="agent:<implementer>"`, for example `agent:codex`.
- Task body must contain acceptance criteria using `- [ ] ...` lines.
- The chip spawn prompt references the Task slug, for example
  `Closes pindoc://task-foo-bar` in the commit or PR description.

### During chip work

- Task remains `task_meta.status="open"`.
- The chip may update body or assignee but does not transition status
  without acceptance evidence.
- If scope changes, update the Task body or split/defer acceptance items
  before continuing.

### After chip merge to main

- The orchestrating agent, or the chip on exit, calls
  `pindoc.task.claim_done(slug_or_id=<task>, commit_sha=<sha>)`
  after all acceptance checkboxes are ticked or explicitly deferred. Passing
  `commit_sha` lets the server auto-pin implementation references from the
  commit diff.
- Include validation notes in the Task revision or final handoff.

### If interrupted / abandoned

- Task stays open.
- The orchestrator decides whether to re-spawn, reassign, block, or cancel.
- Cancel only with an explicit reason through `task_meta.status="cancelled"`.

### Retroactive policy

- Ad-hoc solo dev work without a pre-defined Task can rely on git history.
- Public OSS or team work should create a retroactive Task with
  `task_meta.status="claimed_done"` for auditability.

## Response Discipline

- User-facing replies stay conversational and concise; artifact bodies may use structured ADR/task shorthand.
- Do not paraphrase user statements as confirmed intent unless the user explicitly confirmed them in the same session.
- When asking for approval, include the relevant human URL, not only an internal `pindoc://` reference.

## When In Doubt

- `pindoc.project.current` - lost project scope.
- `pindoc.task.queue` - need current assigned work.
- `pindoc.scope.in_flight` - need unresolved acceptance checkboxes.
- `pindoc.artifact.search` - need related artifacts before creating or updating memory.
- `pindoc.artifact.revisions` / `pindoc.artifact.diff` - need current revision or change history.
