# Public Demo Story Path

<p>
  <a href="./25-public-demo-story-path.md"><img alt="English public demo story path" src="https://img.shields.io/badge/lang-English-2563eb.svg?style=flat-square"></a>
  <a href="./25-public-demo-story-path-ko.md"><img alt="Korean public demo story path" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-6b7280.svg?style=flat-square"></a>
</p>

The first public demo should show why Pindoc matters for collaborative AI work:
an agent discovers something useful, Pindoc turns it into code-pinned team
memory, and the next teammate or agent can follow the evidence without reading
the original chat.

## Launch Route

Default entry:

```text
/p/pindoc/today
```

Preferred visitor flow:

1. Open Today.
2. Choose a visible Change Group tied to public launch, CI, README, or policy work.
3. Open the Task artifact.
4. Follow a related Analysis, Debug, or Decision artifact.
5. Follow commit/file pins or release docs.
6. Return to Today or Graph to see that the memory is connected, not a flat wiki page.

## Candidate Stories

| Story | Start | Follow | Shows |
| --- | --- | --- | --- |
| Multilingual OSS landing | Today or `task-readme-multilingual-landing` | `README.md`, `README-ko.md`, docs hub commits | Agent-written launch docs, acceptance closeout, multilingual navigation. |
| CI trust gate | `task-public-release-trust-gates` | `.github/workflows/ci.yml`, successful CI run, release checklist | Verification evidence, release readiness, code/config pin. |
| Collaborative positioning | `gpt-pro-strategic-review-intake-collaborative-ai-insight-memory` | `task-collaborative-positioning-readme-refresh`, README changes | External review filtered into product judgment and public copy. |
| Record-worthy memory policy | `task-record-worthy-artifact-policy` | `docs/24-record-worthy-artifact-policy.md` | Pindoc records curated team knowledge, not raw chat logs. |
| Read-only demo hardening | `task-readonly-demo-public-site` | `docs/22-public-demo.md`, proxy/scrub checklist | Public demo is real dogfood with write surfaces blocked. |

## Public Safety Check

Use only `pindoc` project artifacts for the first launch path. Do not use
private customer work, private repositories, unpublished domains, local home
paths, or private deployment details in the first screenshot.

Before linking the demo publicly:

- sample artifact bodies from the chosen stories,
- inspect visible pins and commit summaries,
- verify that git blob/diff preview remains blocked unless the referenced repo is public,
- verify that `/mcp` and mutating routes are blocked,
- confirm that the chosen screenshots do not show private paths, emails, domains, or internal hostnames.

## README Caption Candidates

Option A:

```text
Real Pindoc artifacts from the Pindoc project itself. Agents turn useful work
into code-pinned team memory; humans review, discuss, and steer it.
```

Option B:

```text
The public demo is real dogfood: launch tasks, strategy analyses, policies, and
CI fixes written through Pindoc and linked back to commits and docs.
```

## Screenshot Priority

1. Today view with a public launch or policy Change Group.
2. Task article showing acceptance and related artifacts.
3. Related Analysis or Decision article showing filtered judgment.
4. Pin/evidence panel or linked docs/commit evidence.
5. Graph view only if it makes the relationship clearer than the article path.
