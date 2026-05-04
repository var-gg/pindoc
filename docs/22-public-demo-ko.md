# Read-only 공개 데모 운영안

<p>
  <a href="./22-public-demo.md"><img alt="English public demo plan" src="https://img.shields.io/badge/lang-English-6b7280.svg?style=flat-square"></a>
  <a href="./22-public-demo-ko.md"><img alt="Korean public demo plan" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-2563eb.svg?style=flat-square"></a>
</p>

이 문서는 Pindoc 첫 공개 데모의 형태를 정의합니다. 데모는 marketing mockup이
아니라 실제 Pindoc 사용을 보여주는 read-only Reader입니다. 방문자는 Pindoc
자체와 선택된 owner project의 실제 artifact를 읽을 수 있어야 합니다.

## 목표

공개 데모는 한 번의 클릭으로 제품을 이해하게 해야 합니다.

- 방문자가 실제 Reader URL을 엽니다.
- Task, Decision, Analysis, Debug 같은 typed artifact를 봅니다.
- project memory와 code evidence 사이의 link를 따라갑니다.
- agent가 Pindoc을 작성하고 사람이 읽고 토론하며 방향을 정한다는 구조를 이해합니다.

Live demo가 1차 증거입니다. GIF나 video는 announcement post에는 도움이 될 수
있지만 repository README의 필수 조건은 아닙니다.
첫 launch route는 [공개 데모 story path](25-public-demo-story-path-ko.md)에 정리합니다.

## 공개 URL

최종 domain은 release decision입니다. 가능하면 다음처럼 subdomain을 사용합니다.

```text
pindoc.<personal-domain>
```

Placeholder demo link를 `README.md`에 먼저 넣지 마세요. DNS, TLS,
reverse-proxy restriction, scrub check가 끝난 뒤 최종 URL만 추가합니다.

## 접근 모델

공개 데모로 일반 writable daemon을 노출하지 않습니다. Public edge는 read-only여야 합니다.

권장 형태:

```text
public internet
    |
    v
reverse proxy: read-only allowlist
    |
    v
private Pindoc daemon
```

Public allow:

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

Reader-hidden project 확장(`include_hidden=true`, `include_internal=true`,
`ops=1`, `debug=ops`)은 owner 전용입니다. Public caller는 이런 query token을
붙여도 필터링된 기본 `/api/projects` 및 task-flow project list만 받아야 합니다.

Public block:

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

모든 참조 repository가 public이고 source browsing이 데모 범위로 명시 승인된
경우가 아니라면 다음도 차단합니다.

- `/api/p/{project}/git/repos`
- `/api/p/{project}/git/changed-files`
- `/api/p/{project}/git/commit`
- `/api/p/{project}/git/commits/{sha}/referencing-artifacts`
- `/api/p/{project}/git/blob`
- `/api/p/{project}/git/diff`

## Reverse Proxy Sketch

아래는 policy sketch이며 그대로 붙여 넣는 production config가 아닙니다.

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

실제 config는 live Reader에서 검증해야 합니다. 일부 UI 기능은 read-state write를
best-effort로 시도할 수 있습니다. 이런 write는 읽기 navigation을 깨지 않으면서
fail closed해야 합니다.

## 프로젝트 선택

좋은 demo project:

- 실제 Pindoc 사용을 보여줍니다.
- navigation이 의미 있을 만큼 artifact가 있습니다.
- private client/customer 정보가 없습니다.
- public title, commit summary, file path 공개를 감수할 수 있습니다.
- public source code를 참조하거나 git preview route가 차단되어 있습니다.

초기 후보:

- `pindoc`
- 이미 public-safe한 일부 `var.gg` artifact
- 공개 열람을 명시 승인한 작은 personal project

## 첫 화면

기본 entry point는 Today를 사용합니다.

```text
/p/pindoc/today
```

Today는 최근 Change Group을 보여주므로 Pindoc이 정적인 documentation site가
아니라 살아 있는 project memory system이라는 점을 자연스럽게 보여줍니다. 공개
data set 승인 후 README나 demo landing에서 더 깊은 예시로 연결합니다.

## Story Path

첫 공개 route는 단순히 Reader UI가 채워져 있음을 보여주는 데서 끝나면 안 됩니다.
방문자가 Today에서 Task로, 다시 Analysis/Debug/Decision artifact로, 마지막에는
code evidence로 이동하며 협업 가치를 이해할 수 있어야 합니다. 현재 shortlist는
[공개 데모 story path](25-public-demo-story-path-ko.md)에 둡니다.

## Scrub Checklist

Project 공개 전 artifact body, pin, metadata, API response를 샘플링해 다음을 확인합니다.

- API token과 OAuth secret,
- private infrastructure를 드러내는 secret name,
- local home-directory path와 username,
- private IP와 internal hostname,
- 미공개 domain name,
- private repository name,
- customer name 또는 private product plan,
- raw chat log,
- private repository의 git blob/diff content.

Reverse proxy control에 의존하기 전에 data level에서 scrub하세요. Read-only proxy는
private artifact content를 안전하게 만들지 못합니다.

## README Asset Rule

첫 공개 README에는 다음만 사용합니다.

1. public demo link,
2. curated screenshot 1개,
3. 방문자가 무엇을 보고 있는지 설명하는 짧은 caption.

GIF나 MP4는 live demo가 안정화되고 announcement 또는 social sharing 목적이 명확할
때만 추가합니다.

권장 asset path:

```text
docs/assets/pindoc-public-demo-reader.png
```

권장 첫 screenshot은 `pindoc` project의 Today screen입니다. 최근 Task 또는
Analysis change group이 보이되 private path, email, 미공개 domain이 없어야 합니다.
권장 caption:

```text
Real Pindoc artifacts from the Pindoc project itself. Agents turn useful work
into code-pinned team memory; humans review, discuss, and steer it.
```

## Smoke Tests

최종 public URL에 대해 실행합니다.

```bash
curl -fsS https://<demo-host>/api/health
curl -fsS https://<demo-host>/api/projects
curl -I https://<demo-host>/p/pindoc/today
curl -i -X POST https://<demo-host>/api/projects
curl -i https://<demo-host>/mcp
curl -i https://<demo-host>/api/p/pindoc/git/blob
```

기대 결과:

- read route는 `200` 또는 의도된 redirect를 반환합니다.
- mutation route는 `403`, `404`, 또는 명시적 거부를 반환합니다.
- `/mcp`는 public에서 접근되지 않습니다.
- git preview route는 승인 전까지 차단됩니다.
