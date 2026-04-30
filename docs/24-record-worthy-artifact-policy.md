# Record-worthy Artifact Policy

<p>
  <a href="./24-record-worthy-artifact-policy.md"><img alt="English record-worthy artifact policy" src="https://img.shields.io/badge/lang-English-2563eb.svg?style=flat-square"></a>
  <a href="./24-record-worthy-artifact-policy-ko.md"><img alt="Korean record-worthy artifact policy" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-6b7280.svg?style=flat-square"></a>
</p>

Pindoc is not a raw chat archive. It records the parts of AI-assisted work that
future teammates and future agents can reuse.

Use this rule before creating or updating an artifact:

```text
Record it only if it will help someone understand, discuss, verify, or continue
the project later.
```

## Record

Record these when they have project value beyond the current conversation:

- decisions with rationale, alternatives, consequences, or ownership impact,
- analysis that explains a non-obvious finding, tradeoff, user need, or system behavior,
- debugging paths that include failed attempts, root cause, fix direction, or reproduction details,
- task closeouts with acceptance, verification, and commit/file evidence,
- test or QA evidence that future work should trust or re-run,
- issues that cannot be fixed immediately but need shared evidence for team discussion,
- context needed to prevent duplicate documents, repeated investigations, or conflicting decisions.

## Do Not Record

Do not create artifacts for:

- raw chat transcripts,
- temporary thought streams,
- routine progress narration,
- mechanical typo or formatting edits with no future context value,
- private conversations that should not become team memory,
- summaries that repeat existing artifacts without changing status, evidence, or interpretation,
- implementation details that belong only in code comments, tests, or commit messages.

## Type Examples

| Type | Record-worthy example | Non-example |
| --- | --- | --- |
| Task | A multi-step change with acceptance checks, verification notes, and commit pins. | A one-line typo fix already explained by the commit. |
| Decision | A product or architecture choice with rationale and consequences. | A personal preference with no durable team impact. |
| Analysis | A finding that reframes a user problem, launch strategy, data model, or workflow. | A generic session summary that adds no new judgment. |
| Debug | A root cause path with reproduction, failed attempts, and validated fix direction. | A copied error message with no investigation. |
| TC | A test or QA result that future agents or teammates should rely on. | A command log with no interpretation or reuse value. |
| Glossary | A term boundary that prevents repeated ambiguity across artifacts. | A dictionary entry everyone already understands. |

## Update Before Creating

Prefer updating, superseding, or relating to an existing artifact when:

- the same decision or analysis already exists,
- the new information changes the status of an existing task,
- the new evidence confirms or invalidates an earlier claim,
- the work belongs to an existing story path or launch track,
- the only new value is a commit, test result, or wording clarification.

Create a new artifact when:

- the subject is materially new,
- the artifact needs its own lifecycle or review state,
- the existing artifact would become confusing if expanded,
- a new Task is needed to carry acceptance criteria.

## Public Summary

Pindoc keeps curated project memory, not every message. Agents record decisions,
analyses, debug paths, task closeouts, and verification evidence when that
knowledge will help future teammates or future agents. If the content is only a
temporary thought, a raw chat log, or a duplicate of existing memory, it should
not become a new artifact.

## Dogfood Sample Check

Use this policy against a mixed sample before changing the harness:

- `task-readme-multilingual-landing` - record-worthy Task; public-facing docs and launch acceptance.
- `gpt-pro-strategic-review-intake-collaborative-ai-insight-memory` - record-worthy Analysis; filters external review into accepted/rejected product judgment.
- `task-public-release-trust-gates` - record-worthy Task; release trust gates and CI/security evidence.
- `decision-artifact-format-leak-mitigation` - record-worthy Decision; durable harness behavior and user-facing response quality.
- a one-line README badge color tweak - usually not record-worthy as a new artifact; attach it to an existing README Task or commit history instead.
