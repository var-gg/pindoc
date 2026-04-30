# Pindoc

<p>
  <a href="./README.md"><img alt="English README" src="https://img.shields.io/badge/lang-English-6b7280.svg?style=flat-square"></a>
  <a href="./README-ko.md"><img alt="Korean README" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-2563eb.svg?style=flat-square"></a>
</p>

[![CI](https://github.com/var-gg/pindoc/actions/workflows/ci.yml/badge.svg)](https://github.com/var-gg/pindoc/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![MCP](https://img.shields.io/badge/MCP-agent%20memory-4b5563.svg)](docs/README-ko.md)

> **The wiki you never type into.**
> 에이전트 작업을 다음 세션이 다시 읽을 수 있는 프로젝트 기억으로 남깁니다.

Pindoc은 AI 코딩 세션을 위한 self-hosted 프로젝트 메모리 시스템입니다.
사람은 방향과 승인에 집중하고, 코딩 에이전트가 MCP를 통해 지속 가능한
문서를 남깁니다. 모든 artifact는 타입, area, 코드/커밋/파일/URL pin,
관련 artifact를 함께 가집니다.

## 왜 필요한가

AI 코딩 세션은 빠르지만 맥락이 쉽게 사라집니다.

- 터미널 세션이 끝나면 디버깅 경로가 사라집니다.
- 새 에이전트에게 같은 결정을 반복 설명해야 합니다.
- 채팅, 이슈, 문서, 커밋 메시지 사이에 프로젝트 기억이 흩어집니다.

Pindoc은 에이전트가 만든 유효한 작업 흔적을 검색 가능하고 코드에 고정된
메모리 레이어로 바꿉니다.

## 차별점

- **Agent-only write surface**: Reader UI는 읽기와 검토 중심이고, 의미 있는 쓰기는 에이전트를 거칩니다.
- **MCP-native workflow**: `context_for_task`, `artifact.propose`, `task.queue` 같은 도구가 에이전트 행동을 규율합니다.
- **Typed artifacts**: Decision, Analysis, Debug, Flow, Task, TC, Glossary 등을 지원합니다.
- **Code-pinned memory**: 커밋, 파일, 라인, URL, 다른 artifact와 연결됩니다.
- **Multi-project daemon**: 하나의 `/mcp` 엔드포인트가 여러 프로젝트를 처리하고, 각 tool call이 `project_slug`를 가집니다.
- **Self-host first**: Docker Compose로 Postgres, pgvector, Pindoc daemon, Reader SPA를 함께 띄웁니다.

## 공개 데모

첫 공개 릴리스에서는 개인 도메인의 read-only Pindoc demo를 붙일 계획입니다.
Pindoc 자체와 실제 작업 프로젝트 일부를 보여주되, write surface는 막고
민감정보는 scrub합니다.

운영안은 [공개 데모 운영안](docs/22-public-demo-ko.md)에 정리합니다. GIF나
영상은 필수가 아니라 보조 홍보 자산으로 둡니다. 1차 증거는 직접 눌러볼 수
있는 live demo와 대표 screenshot입니다.

## 빠른 시작

사전 요구사항:

- Docker 27+
- host-native 개발 시 Go 1.25+
- web 개발 시 Node 20.15+ 및 pnpm 10+

```bash
git clone https://github.com/var-gg/pindoc.git
cd pindoc
docker compose up -d --build
```

Reader:

```text
http://localhost:5830/
```

빈 인스턴스에서는 첫 프로젝트를 만듭니다.

```text
http://localhost:5830/projects/new
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
기본 project slug를 저장합니다.

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

공개 read-only demo는 daemon을 그대로 인터넷에 쓰기 가능하게 여는 방식이
아니라, reverse proxy에서 `/mcp`와 mutating HTTP route를 막는 방식으로
운영해야 합니다. 자세한 내용은 [보안 정책](SECURITY-ko.md)과
[공개 데모 운영안](docs/22-public-demo-ko.md)을 봅니다.

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

Windows에서 로컬 C toolchain이 없으면 Docker로 Go test를 실행합니다.

```powershell
docker run --rm -v "${PWD}:/work" -w /work golang:1.25 go test ./...
```

## 문서

- [문서 허브](docs/README-ko.md)
- [공개 데모 운영안](docs/22-public-demo-ko.md)
- [공개 릴리스 체크리스트](docs/23-public-release-checklist-ko.md)
- [기여 안내](CONTRIBUTING-ko.md)
- [보안 정책](SECURITY-ko.md)
- [설계 원본 노트](docs/README-ko.md#설계-원본-노트)

## 상태

Pindoc은 현재 실제 dogfood 중입니다. 로컬 self-host 경로, Reader UI,
project/area 모델, artifact propose, task queue, revision history, summary,
real embedding provider 경로가 구현되어 있습니다. 공개 OSS 런치 트랙은
first-run reliability, read-only public demo, CI, 보안 문서, README 정비에
집중합니다.

## 라이선스

Apache License 2.0. [LICENSE](LICENSE)를 참고하세요.
