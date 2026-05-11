# Pindoc

<p>
  <a href="./README.md"><img alt="English README" src="https://img.shields.io/badge/lang-English-6b7280.svg?style=flat-square"></a>
  <a href="./README-ko.md"><img alt="Korean README" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-2563eb.svg?style=flat-square"></a>
</p>

[![CI](https://github.com/var-gg/pindoc/actions/workflows/ci.yml/badge.svg)](https://github.com/var-gg/pindoc/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![MCP](https://img.shields.io/badge/MCP-agent%20memory-4b5563.svg)](docs/README-ko.md)

> **AI-assisted 개발을 위한 code-pinned team memory.**
> 에이전트가 durable record를 쓰고, 사람은 읽고 토론하고 방향을 정합니다.

Pindoc은 AI 코딩 에이전트와 함께 일하는 팀을 위한 self-hosted 프로젝트
메모리 시스템입니다. 에이전트가 발견한 분석, 결정, 디버그 경로, task
closeout, 검증 근거를 typed artifact로 남기고, 각 artifact는 area와
코드/커밋/파일/URL/resource pin, 관련 artifact를 함께 가집니다.

출발점은 AI로 분석한 이슈와 통찰을 기존 wiki/task tracker에 정리해 팀과
공유하고 공론화하던 경험입니다. Pindoc은 그 과정을 agent-native하게 만들되,
raw chat archive가 아니라 미래 팀원과 미래 에이전트가 재사용할 수 있는 핵심만
남깁니다.

## 왜 필요한가

AI 코딩 세션은 빠르지만 팀 맥락은 여전히 쉽게 사라집니다.

- 터미널 세션이 끝나면 디버깅 경로가 사라집니다.
- 새 에이전트에게 같은 결정을 반복 설명해야 합니다.
- 유효한 분석이 한 operator의 채팅 안에만 남고 팀 지식이 되지 못합니다.
- wiki, issue tracker, PR, commit message 사이에 중복 문서가 늘어납니다.
- 실제 프로젝트에서는 책임소재와 내부 관행 때문에 문제를 발견해도 바로 고칠 수 없고, 근거 있는 문서로 먼저 공론화해야 할 때가 많습니다.

Pindoc은 에이전트가 만든 유효한 작업 흔적을 검색 가능하고 코드에 고정된
팀 메모리 레이어로 바꿉니다.

## 차별점

- **Collaborative memory layer**: artifact는 개인 채팅 요약이 아니라 팀원과 미래 에이전트가 다시 읽는 지식입니다.
- **Agent-only write surface**: Reader UI는 읽기와 검토 중심이고, 의미 있는 쓰기는 에이전트를 거칩니다.
- **MCP-native workflow**: `context_for_task`, `artifact.propose`, `task.queue` 같은 도구가 에이전트 행동을 규율합니다.
- **Typed artifacts**: Decision, Analysis, Debug, Flow, Task, TC, Glossary 등을 지원합니다.
- **Code-pinned memory**: 커밋, 파일, 라인, URL, 다른 artifact와 연결됩니다.
- **Record-worthy by design**: raw chat log를 저장하지 않고, 미래 작업에 가치가 있는 결정, 분석, 디버그 경로, 검증, task 맥락만 남깁니다.
- **Multi-project daemon**: 하나의 `/mcp` 엔드포인트가 여러 프로젝트를 처리하고, 각 tool call이 `project_slug`를 가집니다.
- **Self-host first**: Docker Compose로 Postgres, pgvector, Pindoc daemon, Reader SPA를 함께 띄웁니다.

## 공개 데모

read-only 공개 demo는 OSS 릴리스와 분리된 후속 트랙으로 두고, 이번 OSS
범위에는 포함하지 않습니다. demo가 뜨기 전까지 README, [docs/](docs/README-ko.md),
self-host clone이 1차 증거입니다. Pindoc을 end-to-end로 평가하려는 운영자는
`docker compose up -d --build`로 띄워 본인 artifact를 직접 살펴보면 됩니다.

후속 demo 운영안은 hosted instance 시점에 다시 사용할 수 있도록
[공개 데모 운영안](docs/22-public-demo-ko.md)에 그대로 둡니다.

## 빠른 시작

사전 요구사항:

- Docker 27+
- 로컬 dogfood나 소규모 팀 사용은 CPU 2 core, RAM 4 GB 권장
- Docker image, Postgres data, embedding cache를 위한 여유 disk 5 GB 권장,
  fresh clone light smoke는 2 GB minimum
- 첫 실행 시 bundled EmbeddingGemma model/runtime을 cache하기 위한 outbound
  HTTPS
- host-native 개발 시 Go 1.25+
- web 개발 시 Node 20.15+ 및 pnpm 10+

기본 Docker 경로에는 bundled EmbeddingGemma Q4 ONNX provider 기반 의미 검색이
포함되어 있어 별도 embedding sidecar가 필요하지 않습니다. 최소/권장 사양과
선택 배포 profile은 [권장 사양](docs/26-system-requirements-ko.md)에
정리합니다.

```bash
git clone https://github.com/var-gg/pindoc.git
cd pindoc
docker compose up -d --build
```

Reader:

```text
http://localhost:5830/
```

빈 인스턴스에서는 `/`가 먼저 소유자 정보(표시 이름과 이메일)를 받은 뒤 첫
프로젝트 wizard로 이동합니다. 소유자 정보 설정 후 프로젝트 wizard를 직접
열려면:

```text
http://localhost:5830/projects/new?welcome=1
```

## MCP 클라이언트 연결

Docker daemon은 account-level MCP 엔드포인트 하나를 노출합니다.

```jsonc
{
  "mcpServers": {
    "pindoc": {
      "type": "http",
      "url": "http://127.0.0.1:5830/mcp"
    }
  }
}
```

프로젝트 scope는 URL이 아니라 각 tool input의 `project_slug`로 결정됩니다.
`pindoc.harness.install`이 만든 워크스페이스는 `PINDOC.md` frontmatter에
기본 project slug를 저장합니다. `pindoc.workspace.detect`는 현재
워크스페이스에 맞는 slug를 찾아주지만 daemon-wide `PINDOC_PROJECT` 기본값을
바꾸지는 않습니다. 여러 프로젝트가 한 Docker daemon에 붙어 있으면 세션 sweep
후에도 감지된 `project_slug`를 명시해서 넘기는 것이 안전합니다.

`completeness=draft`는 비공개 초안이 아니라 성숙도/trust 상태입니다. MCP
write가 accepted 되면 artifact는 published 상태가 되고, visibility가 허용하면
Reader에 보입니다. 일반 사용자 surface에 보이면 안 되는 내용은
`visibility=private` 또는 review workflow를 써야 합니다.

## Docker Desktop / Windows asset upload

`pindoc.asset.upload(local_path=...)`는 Windows 클라이언트 경로가 아니라 MCP
server host/container 안의 경로를 읽습니다. Docker Desktop에서는 먼저 파일을
`pindoc-server-daemon` 컨테이너 안으로 복사합니다.

```powershell
pwsh -File tools/push-asset.ps1 A:\path\image.png -ProjectSlug survival-manager
```

스크립트는 `pindoc.asset.upload`에 넣을 JSON input과
`/tmp/pindoc-asset-upload/...` container-local path를 출력합니다.

Reader에 inline image가 실제로 보이려면 두 단계가 모두 필요합니다.

1. `body_markdown`에 `![alt](<asset.blob_url>)`를 넣습니다. 렌더링 source입니다.
2. `pindoc.asset.attach`를 `role="inline_image"`로 호출합니다. revision metadata와 evidence입니다.

## 설정

기본 Docker 경로는 1인 self-host와 loopback-only를 전제로 합니다.

| 변수 | 기본값 | 설명 |
| --- | --- | --- |
| `PINDOC_DAEMON_PORT` | `5830` | Docker Compose host port |
| `PINDOC_PROJECT` | `pindoc` | 일부 unscoped read/config의 기본 프로젝트 |
| `PINDOC_PUBLIC_BASE_URL` | `http://127.0.0.1:${PINDOC_DAEMON_PORT}` | 링크와 OAuth metadata에 쓰이는 public base URL |
| `PINDOC_BIND_ADDR` | `127.0.0.1:5830` | 보안 의도. non-loopback이면 IdP 또는 명시적 unauth opt-in 필요 |
| `PINDOC_AUTH_PROVIDERS` | empty | 외부 요청용 IdP. 현재 `github` 지원 |
| `PINDOC_ALLOW_PUBLIC_UNAUTHENTICATED` | `false` | 인증 없는 외부 노출 명시 opt-in |
| `PINDOC_FORCE_OAUTH_LOCAL` | `false` | 개발용 flag. loopback `/mcp` 호출도 OAuth bearer auth를 타게 함 |
| `PINDOC_ALLOWED_ORIGINS` | empty | CORS 허용 origin 콤마 목록. 빈 값이면 same-origin only |
| `PINDOC_DEV_MODE` | `false` | 로컬 도구용 wildcard CORS 개발 flag. 공개 인스턴스에서는 사용 금지 |

공개 read-only demo는 daemon을 그대로 인터넷에 쓰기 가능하게 여는 방식이
아니라, reverse proxy에서 `/mcp`와 mutating HTTP route를 막는 방식으로
운영해야 합니다. 자세한 내용은 [보안 정책](SECURITY-ko.md)과
[공개 데모 운영안](docs/22-public-demo-ko.md)을 봅니다.
Daemon은 자체적으로 `nosniff`, clickjacking 방어, referrer policy,
asset-blob hardened CSP 같은 baseline security header도 부착합니다.

쓰기 가능한 공개 또는 cross-device 인스턴스는
[docs/oauth-setup-ko.md](docs/oauth-setup-ko.md)를 따릅니다. GitHub OAuth App
생성, `${PINDOC_PUBLIC_BASE_URL}/auth/github/callback` callback 규칙, 런타임
MCP client 등록, `PINDOC_FORCE_OAUTH_LOCAL` 로컬 QA 절차를 포함합니다.

## 개발

```bash
go test ./...

cd web
pnpm install --frozen-lockfile
pnpm typecheck
pnpm test:unit
pnpm build

docker build -t pindoc-server:local .
```

`127.0.0.1`로 접속하면서도 OAuth bearer path를 검증하려면
`PINDOC_FORCE_OAUTH_LOCAL=true`를 설정합니다. daemon은 boot warning을 남기고
loopback `/mcp` 호출에도 Bearer token을 요구합니다.

Windows에서 로컬 C toolchain이 없으면 Docker로 Go test를 실행합니다.

```powershell
docker run --rm -v "${PWD}:/work" -w /work golang:1.25 go test ./...
```

## 문서

- [문서 허브](docs/README-ko.md)
- [공개 데모 운영안](docs/22-public-demo-ko.md)
- [공개 데모 story path](docs/25-public-demo-story-path-ko.md)
- [Record-worthy artifact 정책](docs/24-record-worthy-artifact-policy-ko.md)
- [공개 릴리스 체크리스트](docs/23-public-release-checklist-ko.md)
- [기여 안내](CONTRIBUTING-ko.md)
- [보안 정책](SECURITY-ko.md)
- [설계 원본 노트](docs/README-ko.md#설계-원본-노트)

## 상태

Pindoc은 현재 실제 dogfood 중입니다. 로컬 self-host 경로, Reader UI,
project/area 모델, artifact propose, task queue, revision history, summary,
real embedding provider 경로가 구현되어 있습니다. 공개 OSS 런치 트랙은
first-run reliability, read-only dogfood demo, CI, 보안 문서, 협업형
포지셔닝 정비에 집중합니다.

## 피드백

장문 질문, 기능 제안, 설계 토론은
[GitHub Discussions](https://github.com/var-gg/pindoc/discussions)에 남겨주세요.
버그 리포트는 [GitHub Issues](https://github.com/var-gg/pindoc/issues)로 가
주세요. 메인테이너는 보통 하루 안에 답합니다.

## 라이선스

Apache License 2.0. [LICENSE](LICENSE)를 참고하세요.
