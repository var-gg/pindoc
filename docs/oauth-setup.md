# OAuth Setup For Public Pindoc Instances

This guide covers the first non-loopback setup: one Pindoc daemon reachable from
another machine, protected by GitHub login, and usable by multiple MCP clients.

## 1. Choose The Public URL

Set `PINDOC_PUBLIC_BASE_URL` to the URL users and MCP clients will open. Use
HTTPS for anything outside your own machine.

```env
PINDOC_BIND_ADDR=0.0.0.0:5830
PINDOC_PUBLIC_BASE_URL=https://pindoc.example.com
PINDOC_AUTH_PROVIDERS=github
```

The GitHub callback URL is always:

```text
${PINDOC_PUBLIC_BASE_URL}/auth/github/callback
```

For the example above, GitHub must redirect to
`https://pindoc.example.com/auth/github/callback`.

## 2. Create A GitHub OAuth App

1. Open <https://github.com/settings/applications/new>.
2. Set **Homepage URL** to `PINDOC_PUBLIC_BASE_URL`.
3. Set **Authorization callback URL** to
   `${PINDOC_PUBLIC_BASE_URL}/auth/github/callback`.
4. Create the app, then copy the Client ID and generate a Client Secret.

Pindoc only needs GitHub as an identity provider. The MCP-facing OAuth tokens are
issued by the Pindoc daemon.

## 3. Configure The Daemon

Use this minimal `.env` shape for a public instance:

```env
PINDOC_BIND_ADDR=0.0.0.0:5830
PINDOC_PUBLIC_BASE_URL=https://pindoc.example.com
PINDOC_AUTH_PROVIDERS=github
PINDOC_GITHUB_CLIENT_ID=Iv1.example
PINDOC_GITHUB_CLIENT_SECRET=github-secret
PINDOC_OAUTH_SIGNING_KEY_PATH=/var/lib/pindoc/cache/oauth-signing.pem

# Boot-time seed only. Runtime clients can be added from /admin/providers or
# registered dynamically through POST /oauth/register.
PINDOC_OAUTH_CLIENT_ID=claude-desktop
PINDOC_OAUTH_CLIENT_SECRET=
PINDOC_OAUTH_REDIRECT_URIS=http://127.0.0.1:3846/callback,http://localhost:3846/callback
```

`PINDOC_OAUTH_CLIENT_ID`, `PINDOC_OAUTH_CLIENT_SECRET`, and
`PINDOC_OAUTH_REDIRECT_URIS` seed one initial client at boot. After startup,
register more MCP clients through the OAuth Clients section at
`/admin/providers` or through Dynamic Client Registration:

```bash
curl -sS https://pindoc.example.com/oauth/register \
  -H 'content-type: application/json' \
  -d '{
    "client_name": "Codex",
    "redirect_uris": ["http://127.0.0.1:3846/callback"],
    "token_endpoint_auth_method": "none",
    "scope": "pindoc offline_access"
  }'
```

## 4. Put HTTPS In Front

For Caddy:

```caddyfile
pindoc.example.com {
  reverse_proxy 127.0.0.1:5830
}
```

For nginx:

```nginx
server {
  server_name pindoc.example.com;

  location / {
    proxy_pass http://127.0.0.1:5830;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto https;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  }
}
```

## 5. Connect MCP Clients

Each MCP client connects to the account-level endpoint:

```jsonc
{
  "mcpServers": {
    "pindoc": {
      "type": "http",
      "url": "https://pindoc.example.com/mcp"
    }
  }
}
```

On first connect, the client discovers `/.well-known/oauth-protected-resource`,
registers through `/oauth/register` when it supports DCR, sends the user through
GitHub login if needed, shows Pindoc's consent screen, then exchanges the code
at `/oauth/token`.

## Current Limits

OAuth consent is now explicit per user, client, and scope. Repeated requests for
the same scope skip the screen; a broader scope request shows it again.

The previous single-client seed gap is tracked by
[task-qa-oauth-dcr-missing](http://localhost:5830/p/pindoc/wiki/task-qa-oauth-dcr-missing).
The consent gap is tracked by
[task-qa-oauth-consent-missing](http://localhost:5830/p/pindoc/wiki/task-qa-oauth-consent-missing).
The old bootstrap fallback issue is tracked by
[task-qa-oauth-bootstrap-fallback](http://localhost:5830/p/pindoc/wiki/task-qa-oauth-bootstrap-fallback).

## Local OAuth QA

Loopback `/mcp` calls normally bypass bearer auth. To exercise the OAuth bearer
path on the same machine, set:

```env
PINDOC_FORCE_OAUTH_LOCAL=true
```

The daemon logs a warning because this flag is only for development and OSS QA.
