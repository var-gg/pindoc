# Public Read-Only Demo Plan

<p>
  <a href="./22-public-demo.md"><img alt="English public demo plan" src="https://img.shields.io/badge/lang-English-2563eb.svg?style=flat-square"></a>
  <a href="./22-public-demo-ko.md"><img alt="Korean public demo plan" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-6b7280.svg?style=flat-square"></a>
</p>

This document defines the launch demo shape for Pindoc. The demo is intended to
show real Pindoc usage, not a marketing mockup: visitors should be able to read
actual artifacts from Pindoc itself and selected owner projects.

## Goal

The public demo should make the product understandable in one click:

- a visitor opens a real Reader URL,
- sees typed artifacts such as Task, Decision, Analysis, and Debug,
- follows links between project memory and code evidence,
- understands that agents write Pindoc and humans review, discuss, and steer the result.

The live demo is the primary proof. A GIF or video can still help an
announcement post, but it is not required for the repository README.
The curated launch route is tracked in [Public Demo Story Path](25-public-demo-story-path.md).

## Public URL

The exact domain is a release decision. Use a subdomain when possible, for
example:

```text
pindoc.<personal-domain>
```

Avoid publishing placeholder demo links in `README.md`. Add the final URL only
after DNS, TLS, reverse-proxy restrictions, and scrub checks are complete.

## Access Model

Do not expose a normal writable daemon as the public demo. The public edge
should be read-only.

Recommended shape:

```text
public internet
    |
    v
reverse proxy: read-only allowlist
    |
    v
private Pindoc daemon
```

Publicly allowed:

- `GET`, `HEAD`, `OPTIONS`
- `/`
- `/assets/...`
- `/p/{project}/...`
- `/api/config`
- `/api/health`
- `/api/projects`
- `/api/user/current`
- `/api/p/{project}`
- `/api/p/{project}/areas`
- `/api/p/{project}/artifacts`
- `/api/p/{project}/artifacts/{idOrSlug}`
- `/api/p/{project}/artifacts/{idOrSlug}/revisions`
- `/api/p/{project}/artifacts/{idOrSlug}/diff`
- `/api/p/{project}/search`
- `/api/p/{project}/change-groups`
- `/api/p/{project}/inbox`
- `/api/p/{project}/read-states`
- `/api/p/{project}/artifacts/{idOrSlug}/read-state`

Reader-hidden project expansion (`include_hidden=true`, `include_internal=true`,
`ops=1`, `debug=ops`) is owner-only. Public callers must receive the normal
filtered `/api/projects` and task-flow project lists even when they add those
query tokens.

Publicly blocked:

- `/mcp`
- `/auth/...`
- `/join`
- `POST /api/projects`
- `POST /api/onboarding/identity`
- `/api/instance/providers`
- `PATCH /api/p/{project}/settings`
- `POST /api/p/{project}/invite`
- `/api/p/{project}/members`
- `/api/p/{project}/invites`
- `POST /api/p/{project}/inbox/{idOrSlug}/review`
- `POST /api/p/{project}/read-mark`
- `POST /api/p/{project}/read-events`
- `POST /api/p/{project}/artifacts/{idOrSlug}/task-meta`
- `POST /api/p/{project}/artifacts/{idOrSlug}/task-assign`

Block these unless every referenced repository is public and source browsing is
an explicit part of the demo:

- `/api/p/{project}/git/repos`
- `/api/p/{project}/git/changed-files`
- `/api/p/{project}/git/commit`
- `/api/p/{project}/git/commits/{sha}/referencing-artifacts`
- `/api/p/{project}/git/blob`
- `/api/p/{project}/git/diff`

## Reverse Proxy Sketch

This is a policy sketch, not a drop-in production config:

```nginx
location /mcp {
  return 403;
}

if ($request_method !~ ^(GET|HEAD|OPTIONS)$) {
  return 403;
}

location ~ ^/api/p/[^/]+/git/ {
  return 403;
}

location / {
  proxy_pass http://127.0.0.1:5830;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-Proto $scheme;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

The actual config should be tested against the live Reader because some UI
features may attempt best-effort read-state writes. Those writes must fail
closed without breaking read navigation.

## Project Selection

Good demo projects:

- show real Pindoc use,
- have enough artifacts to make navigation meaningful,
- contain no private client/customer information,
- can tolerate public titles, commit summaries, and file paths,
- reference public source code or have git preview routes blocked.

Initial candidates:

- `pindoc`
- selected `var.gg` artifacts that are already public-safe
- small personal projects explicitly approved for public reading

## First Screen

Use Today as the default entry point:

```text
/p/pindoc/today
```

Today shows recent Change Groups and gives visitors a natural way to see that
Pindoc is a living project memory system rather than a static documentation
site. Link deeper examples from the README or demo landing after the public data
set is approved.

## Story Path

The first public route should show collaboration value, not just a populated
Reader UI. Prefer paths where visitors can move from Today to a Task, then to
an Analysis, Debug, or Decision artifact, and finally to code evidence. The
current shortlist lives in [Public Demo Story Path](25-public-demo-story-path.md).

## Scrub Checklist

Before publishing a project, sample artifact bodies, pins, metadata, and API
responses for:

- API tokens and OAuth secrets,
- secret names that reveal private infrastructure,
- local home-directory paths and usernames,
- private IPs and internal hostnames,
- unpublished domain names,
- private repository names,
- customer names or private product plans,
- raw chat logs,
- git blob/diff content from private repositories.

Scrub at the data level before relying on reverse-proxy controls. A read-only
proxy cannot make private artifact content safe.

## README Asset Rule

For the first public README, use:

1. the public demo link,
2. one curated screenshot,
3. a short caption explaining what the visitor is seeing.

Only add GIF or MP4 after the live demo is stable and there is a clear
announcement or social-sharing use case.

Suggested asset path:

```text
docs/assets/pindoc-public-demo-reader.png
```

Suggested first screenshot: the Today screen for `pindoc`, with a visible recent
Task or Analysis change group and no private paths, emails, or unpublished
domains. Suggested caption:

```text
Real Pindoc artifacts from the Pindoc project itself. Agents turn useful work
into code-pinned team memory; humans review, discuss, and steer it.
```

## Smoke Tests

Run these against the final public URL:

```bash
curl -fsS https://<demo-host>/api/health
curl -fsS https://<demo-host>/api/projects
curl -I https://<demo-host>/p/pindoc/today
curl -i -X POST https://<demo-host>/api/projects
curl -i https://<demo-host>/mcp
curl -i https://<demo-host>/api/p/pindoc/git/blob
```

Expected:

- read routes return `200` or an intentional redirect,
- mutating routes return `403`, `404`, or another explicit denial,
- `/mcp` is not publicly reachable,
- git preview routes are blocked unless approved.
