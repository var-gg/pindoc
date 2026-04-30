# Public Release Checklist

This is the minimum trust gate before making the repository public or linking a
public demo from the README.

## Repository

- `README.md` is the English primary landing page.
- `README-ko.md` exists and is linked from the first screen.
- `LICENSE` is Apache 2.0 and README/license references agree.
- `SECURITY.md` explains loopback trust, external exposure, and read-only demo
  constraints.
- No README or first-run docs describe stale M1 scaffold or stub-default
  behavior as current.

## CI

Required checks:

```bash
go test ./...
cd web && pnpm typecheck && pnpm test:unit && pnpm build
docker build -t pindoc-server:local .
git diff --check
```

Windows developers without a local C toolchain can run Go tests through Docker:

```powershell
docker run --rm -v "${PWD}:/work" -w /work golang:1.25 go test ./...
```

## Docker Quick Start

Fresh clone smoke:

```bash
docker compose up -d --build
curl -fsS http://127.0.0.1:5830/health
curl -fsS http://127.0.0.1:5830/api/config
```

Manual browser check:

```text
http://localhost:5830/
```

Expected: the Reader or first-project onboarding opens without a stack trace.

## Public Demo

Before adding a public demo URL to README:

- DNS and TLS are live.
- `/mcp` is blocked from the public internet.
- Public non-`GET` methods are blocked.
- Git preview routes are blocked unless every referenced repo is public and
  source browsing is explicitly approved.
- Demo data passes the scrub checklist in [22-public-demo.md](22-public-demo.md).
- The default public entry point is `/p/pindoc/today` unless a later launch
  decision changes it.
- One curated screenshot is committed under `docs/assets/`.

## Remaining User Decisions

- final public demo domain,
- final list of projects exposed in the demo,
- whether git blob/diff preview is public or blocked,
- whether a GIF/MP4 is worth producing for the announcement.
