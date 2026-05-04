# Security Policy

<p>
  <a href="./SECURITY.md"><img alt="English security policy" src="https://img.shields.io/badge/lang-English-2563eb.svg?style=flat-square"></a>
  <a href="./SECURITY-ko.md"><img alt="Korean security policy" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-6b7280.svg?style=flat-square"></a>
</p>

Pindoc is self-hosted software that stores project memory, code references,
agent-written artifacts, and optional identity-provider configuration. Treat a
Pindoc instance as sensitive infrastructure for the projects it indexes.

## Supported Versions

Pindoc has not cut a stable release yet. Security fixes target `main` until the
first tagged release series exists. After that, this file will list supported
release branches.

## Reporting a Vulnerability

Use GitHub Security Advisories when available. If private advisories are not yet
enabled for the repository, open a minimal public issue that does not include
exploit details, secrets, tokens, private URLs, or sensitive project data, and
ask maintainers for a private contact path.

Please include:

- affected commit or version,
- deployment mode: loopback-only, LAN, public reverse proxy, or hosted demo,
- whether `PINDOC_AUTH_PROVIDERS` or `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED` is set,
- the smallest safe reproduction you can share.

## Local Trust Model

The default Docker setup is intended for a single operator on loopback:

- `PINDOC_BIND_ADDR=127.0.0.1:5830`
- `PINDOC_AUTH_PROVIDERS` empty
- `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED=false`
- Docker publishes the daemon to `127.0.0.1:${PINDOC_DAEMON_PORT}`

Loopback requests are trusted and mapped to the local owner identity. This is
deliberate for local agent workflows; it is not a public internet security
model.

MCP asset uploads add one stricter boundary: `local_path` is accepted only from
loopback principals because it makes the daemon read from its host filesystem.
Non-loopback OAuth agents must send `bytes_base64` or `content_base64` instead;
the daemon still validates size, MIME type, and project membership before
storing the asset.

## External Exposure

If `PINDOC_BIND_ADDR` is non-loopback, Pindoc refuses to start unless one of
these is true:

- `PINDOC_AUTH_PROVIDERS` enables an identity provider such as `github`, or
- `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED=true` explicitly opts into an
  unauthenticated network model.

Do not set `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED=true` for a writable public
internet deployment. Use it only behind a trusted LAN or reverse proxy with
additional access controls.

Pindoc's HTTP daemon is default-deny for cross-origin browser requests. Set
`PINDOC_ALLOWED_ORIGINS` to a comma-separated allowlist when a trusted frontend
must call the daemon from another origin. `PINDOC_DEV_MODE=true` enables
wildcard CORS for local tooling only and should not be used on public instances.
The daemon also emits baseline security headers and serves asset blobs with a
hardened CSP; keep equivalent or stricter headers if a reverse proxy rewrites
responses.

## Read-Only Public Demo

A public demo should not expose a normal writable daemon. Prefer this shape:

- keep the Pindoc daemon private or behind a reverse proxy,
- block `/mcp` from the public internet,
- block non-`GET`/`HEAD`/`OPTIONS` methods publicly,
- block admin and mutation routes such as project creation, provider admin,
  invite/member management, task metadata writes, inbox review, read events,
  onboarding identity, and joins,
- block git preview endpoints unless every referenced repository is already
  public and the demo owner has explicitly approved source-code browsing.

See [docs/22-public-demo.md](docs/22-public-demo.md) for the operational
checklist.

## Secrets and Sensitive Data

Before publishing a demo, scrub or exclude:

- API tokens, OAuth client secrets, signing keys, and database URLs,
- local usernames, home-directory paths, private machine names, and internal IPs,
- private repo names, branch names, deployment hosts, and unpublished customer or
  project names,
- artifact bodies that quote private chat logs or issue trackers,
- git blob/diff preview routes for private repositories.

Pindoc's public Reader is useful only if the data set was curated for public
viewing. Reverse-proxy read-only controls do not sanitize already-stored
artifact content.

## Dependency and Build Security

The Docker image builds the web UI and Go server from source, runs as the
non-root `pindoc` user, and stores runtime cache under `/var/lib/pindoc/cache`.
Operators should pin image tags for production, keep the base image updated, and
avoid mounting writable source trees into publicly exposed containers.
