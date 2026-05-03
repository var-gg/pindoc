# 공개 Pindoc 인스턴스 OAuth 설정

이 문서는 loopback이 아닌 환경에서 Pindoc daemon을 외부 기기에서 열고,
GitHub 로그인으로 보호하며, 여러 MCP client를 동시에 붙이는 첫 설정 절차를
정리한다.

## 1. Public URL 결정

사용자와 MCP client가 접근할 URL을 `PINDOC_PUBLIC_BASE_URL`로 정한다. 같은
머신 밖에서 접근한다면 HTTPS를 사용한다.

```env
PINDOC_BIND_ADDR=0.0.0.0:5830
PINDOC_PUBLIC_BASE_URL=https://pindoc.example.com
PINDOC_AUTH_PROVIDERS=github
```

GitHub callback URL은 항상 다음 규칙으로 결정된다.

```text
${PINDOC_PUBLIC_BASE_URL}/auth/github/callback
```

위 예시에서는 `https://pindoc.example.com/auth/github/callback`을 GitHub에
등록한다.

## 2. GitHub OAuth App 만들기

1. <https://github.com/settings/applications/new>를 연다.
2. **Homepage URL**에 `PINDOC_PUBLIC_BASE_URL`을 넣는다.
3. **Authorization callback URL**에
   `${PINDOC_PUBLIC_BASE_URL}/auth/github/callback`을 넣는다.
4. 앱을 만든 뒤 Client ID와 Client Secret을 복사한다.

GitHub는 사용자 신원 확인용 IdP다. MCP client가 받는 OAuth token은 Pindoc
daemon이 직접 발급한다.

## 3. Daemon 환경변수

공개 인스턴스의 최소 `.env` 형태는 다음과 같다.

```env
PINDOC_BIND_ADDR=0.0.0.0:5830
PINDOC_PUBLIC_BASE_URL=https://pindoc.example.com
PINDOC_AUTH_PROVIDERS=github
PINDOC_GITHUB_CLIENT_ID=Iv1.example
PINDOC_GITHUB_CLIENT_SECRET=github-secret
PINDOC_OAUTH_SIGNING_KEY_PATH=/var/lib/pindoc/cache/oauth-signing.pem

# boot-time seed 전용. 런타임 client는 /admin/providers 또는
# POST /oauth/register로 추가한다.
PINDOC_OAUTH_CLIENT_ID=claude-desktop
PINDOC_OAUTH_CLIENT_SECRET=
PINDOC_OAUTH_REDIRECT_URIS=http://127.0.0.1:3846/callback,http://localhost:3846/callback
```

`PINDOC_OAUTH_CLIENT_ID`, `PINDOC_OAUTH_CLIENT_SECRET`,
`PINDOC_OAUTH_REDIRECT_URIS`는 부팅 시 초기 client 1개를 seed한다. 이후에는
`/admin/providers`의 OAuth Clients 섹션이나 Dynamic Client Registration으로
client를 추가한다.

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

## 4. HTTPS reverse proxy

Caddy 예시:

```caddyfile
pindoc.example.com {
  reverse_proxy 127.0.0.1:5830
}
```

nginx 예시:

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

## 5. MCP client 연결

각 MCP client는 account-level endpoint 하나에 연결한다.

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

첫 연결에서 client는 `/.well-known/oauth-protected-resource`를 발견하고,
DCR을 지원하면 `/oauth/register`로 자신을 등록한다. 사용자는 필요하면 GitHub
로그인을 거치고, Pindoc consent 화면에서 client와 scope를 확인한 뒤
`/oauth/token`으로 code를 token으로 교환한다.

## 현재 한계

OAuth consent는 user, client, scope 조합별로 명시적으로 저장된다. 같은 scope의
재요청은 화면 없이 통과하고, 더 넓은 scope 요청은 다시 consent를 요구한다.

single-client seed 문제는
[task-qa-oauth-dcr-missing](http://localhost:5830/p/pindoc/wiki/task-qa-oauth-dcr-missing),
consent 부재 문제는
[task-qa-oauth-consent-missing](http://localhost:5830/p/pindoc/wiki/task-qa-oauth-consent-missing),
bootstrap fallback 문제는
[task-qa-oauth-bootstrap-fallback](http://localhost:5830/p/pindoc/wiki/task-qa-oauth-bootstrap-fallback)에서
추적한다.

## 로컬 OAuth QA

loopback `/mcp` 호출은 기본적으로 bearer auth를 우회한다. 같은 머신에서 OAuth
bearer path를 검증하려면 다음을 설정한다.

```env
PINDOC_FORCE_OAUTH_LOCAL=true
```

이 flag는 개발과 OSS QA 전용이므로 daemon은 부팅 시 warning을 남긴다.
