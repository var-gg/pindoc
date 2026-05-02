## Current task

The current sample task is to explain how a Pindoc agent leaves enough context for the next session to continue safely.

## Completed work

The previous session created generic sample artifacts for Analysis, Decision, Task, Debug, Glossary, and Flow types.

## Pending checks

- [ ] Confirm the sample project appears only when `PINDOC_WITH_SAMPLE=true`.
- [ ] Confirm deleting the sample project does not affect the operator's real project.

## Evidence

The fixture manifest lists every artifact slug, type, area, title, tag set, and markdown file used by the loader.

## Next MCP calls

Call `pindoc.project.current` with `project_slug="pindoc-tour"` to inspect the sample project, then call `pindoc.artifact.read` on any sample slug.
