# Contributing to Pindoc

Thanks for your interest in improving Pindoc. This document covers how
to file issues, submit pull requests, and sign the Contributor License
Agreement so your changes can be merged.

## Before you start

- **Bug reports**: open an issue with reproduction steps, the commit
  you reproduced on, and what you expected to happen.
- **Feature proposals**: open an issue first. Pindoc has an opinionated
  philosophy ("agent-only write surface", "code-pinned artifacts",
  "promote not store"); proposals that collide with it are usually
  better as separate tools rather than PRs.
- **Documentation fixes**: welcomed without prior discussion. Same CLA
  still applies — it covers every contribution, not just code.

## Development setup

See the `README.md` Quick Start section for toolchain requirements
(Go, Docker, Node, pnpm). `make help` lists the day-to-day dev
targets.

Run the smoke loop before submitting a PR:

```bash
docker compose up -d db embed
go build ./... && go vet ./...
cd web && pnpm exec tsc --noEmit
```

## Contributor License Agreement (CLA)

Pindoc is licensed under the Apache License, Version 2.0. To keep
the project legally coherent — and to preserve the ability to
respond to future licensing questions as the ecosystem evolves — we
ask every contributor to sign a Contributor License Agreement (CLA)
before their first merge.

The CLA is:

- **One-time**. Signing it covers all your future contributions.
- **Automated**. A bot checks every pull request and posts a signing
  link in the PR thread the first time you contribute. Click the
  link, review the agreement, click "I agree".
- **Individual or corporate**. Pick the version that matches your
  situation. If you're contributing on behalf of your employer, use
  the corporate CLA.

The agreement text lives in `CLA.md`. It is modeled on the standard
Apache-style CLA: you retain copyright on your contributions and grant
the project a perpetual license to use them under the same Apache 2.0
terms the rest of the codebase ships under.

We will never merge a PR that fails the CLA check; if the bot
hasn't posted a link yet, comment `@cla-assistant check` or tag a
maintainer.

## Pull request process

1. Fork the repo and create a feature branch from `main`.
2. Make your change in small, self-contained commits. Each commit
   message should explain *why*, not just *what*.
3. Run the smoke loop above; make sure Go builds, `go vet` is clean,
   and TypeScript typechecks.
4. Open a PR against `main`. Describe the user-visible change and
   link any related issue.
5. The CLA bot will post a signing link on your first PR. Sign it.
6. A maintainer will review. Expect comments on naming, scope, and
   whether the change fits Pindoc's write-regulator philosophy.
7. Once approved and CLA-signed, a maintainer will merge.

## Style notes

- Go: `gofmt` + `go vet` must pass. Errors get `fmt.Errorf("context: %w", err)` wrapping; don't swallow.
- TypeScript: strict mode; `pnpm exec tsc --noEmit` must pass.
- Commits: follow the existing Pindoc commit style — present tense,
  short first line, body explains why. Co-author credit is fine.
- Docs: if you add a new user-facing flag, tool, or HTTP endpoint,
  update the matching `docs/*.md` in the same PR.

## Out of scope

Pindoc intentionally does not ship:

- Direct human-editable wiki UI (see `docs/08-non-goals.md`).
- Raw session / chat log ingestion.
- A task tracker's full feature set (sprints, burndown charts,
  time tracking).

PRs that reintroduce any of these will be closed with an explanation.
Not every good idea is a good idea for this project.

## Code of conduct

Be civil. Disagree about technical decisions freely; don't attack
people. Maintainers will close threads that cross that line and
may ban repeat offenders.
