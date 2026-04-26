# Pindoc

> **The wiki you never type into.**
> Where agent work becomes lasting memory — pinned to code, written by agents.

**Pindoc**은 사람이 직접 타이핑하지 않는 위키입니다.
모든 쓰기는 코딩 에이전트(Claude Code / Cursor / Cline / Codex)를 통해 일어나고,
사람은 승인·거절·방향 제시만 합니다. 모든 문서는 코드 커밋·파일 경로에 **핀(pin)** 됩니다.

`var.gg` 생태계의 첫 플래그십 제품. Self-host first. 최종 로컬 부팅 목표는
`docker compose up -d` 한 번으로 Postgres + pgvector + Pindoc HTTP daemon까지
뜨는 흐름이다. 현재 M1 개발 경로는 Docker Compose로 DB를 띄우고,
`pindoc-server -http 127.0.0.1:5830`를 host-native daemon으로 실행한다.

Apache License 2.0. 기여 가이드와 CLA는 [CONTRIBUTING.md](CONTRIBUTING.md) 참조.

---

## Why Pindoc

2026년의 개발자는 에이전트와 협업합니다. 하지만 에이전트의 출력은 세 갈래로 사라집니다:

1. **휘발** — 터미널 세션 닫히면 디버깅 2시간이 증발
2. **검색 지옥** — 월 단위로 쌓인 세션·채팅 중 "그때 그거" 못 찾음
3. **파편화** — Notion/Linear/Slack에 흩어진 흔적만

그 결과 개인·팀은 같은 문제를 N번 풉니다.

**Pindoc은 이 흐름을 바꿉니다.** 에이전트가 세션에서 만든 가치를 에이전트가 정제해 발행하고, 다음 세션의 컨텍스트로 자동 주입됩니다. 사람은 방향만 제시합니다.

## Core Loop

```
Harness (PINDOC.md)
   │
   ▼
Agent Session ── checkpoint ──▶ Promote ──▶ Artifact ──▶ Graph ──▶ Next Session
(외부, 흡수 없음)                (typed,                                 ▲
                                 pinned,                                 │
                                 area-tagged)                            │
                                                                         │
                                        URL → agent fetch ───────────────┘
                                        (Continuation Context)
```

## What makes Pindoc different

- **Agent-only write surface** — UI에 편집 버튼 없음. 오탈자부터 아키텍처까지 전부 에이전트 경유.
- **Harness Reversal** — Pindoc MCP가 연결되면 `PINDOC.md`로 에이전트의 base 규약을 주입.
- **Tool-driven Pre-flight Check** — `propose` 요청은 즉답 대신 체크리스트로 에이전트에게 **더 많은 일을 지시**. MCP가 응답 서버가 아니라 regulator.
- **Referenced Confirmation** — 에이전트가 사용자에게 확인 요청할 때 **항상 링크 동반**.
- **Typed Documents (Tier A/B/C)** — Decision/Analysis/Debug/Flow/Task/TC/Glossary + Domain Pack.
- **Git-pinned artifacts** — 커밋/PR/파일 경로 고정. 코드 변경 시 stale 자동.
- **Fast Landing** — 완벽 인덱스 아님. 핵심 리소스 1~3개로의 빠른 착륙. M7 자가 검증.
- **Multi-project by Design** — 한 인스턴스 = 복수 프로젝트 (schema/URL/UI 모두 `/p/:project/…` 스코프). `pindoc-server`는 두 transport를 지원: stdio(기본, subprocess-per-session)와 `pindoc-server -http <addr>` 데몬 모드. HTTP 데몬은 단일 `/mcp` URL에 모든 워크스페이스가 attach하고, 프로젝트는 각 tool input의 `project_slug`로 결정된다. FE/BE 분리·Solo 사이드 프로젝트·영세 팀 현실 지원.

## Target Users

- **Solo 개발자** — 1급 시민. "promote 안 하면 못 찾음"을 오히려 promote 문화로 해결.
- **2~10인 소규모 팀** — 에이전트 사용자 최소 1명.
- **자율 에이전트 환경 (OpenClaw 등)** — PINDOC.md mode=auto로 human-out-of-the-loop 지원.

## Status

🚧 **M1 스캐폴드 진행 중** — 기획 완료, 구현 착수. 이 repo가 **첫 meta-dogfooding 사례**입니다. Phase 2 완료 시점부터 docs는 Pindoc을 통해서만 수정됩니다.

### M1 현 진행 상태 (2026-04-22)

- ✅ Phase 1 — Docker Compose + Go MCP 서버 스켈레톤 + `pindoc.ping` stdio handshake
- ✅ Phase 2 — Project/Area/Artifact schema + `propose`/`read`/`search` 도구
- ✅ Phase 3 — 임베딩 레이어 (stub default, HTTP provider 준비됨)
- ✅ Phase 4 — Web UI 실데이터 연결 (Reader + Sidebar + Sidecar + ⌘K)
- ✅ Phase 5 — `pindoc.harness.install` + i18n (ko/en)
- ✅ Phase 6 — `docs/*.md` 15개를 Pindoc artifact로 임포트 (meta-dogfood 시드)
- ✅ Phase 7 — revision 시스템 (revisions / diff / summary_since + history·diff UI)
- ✅ Phase 8 — URL 멀티프로젝트 재구조화 (`/p/:project/…` canonical + `pindoc.project.create` MCP tool)
- ✅ Phase 9 — `human_url`/`agent_ref` 분리 + `capabilities` 블록 + spec↔runtime drift 가시화
- ✅ Phase 10 — real embedder dogfood (Docker TEI + `multilingual-e5-base` + pindoc-reembed CLI)
- ✅ Phase 11 — write contract 강화 (`search_receipt` hard enforce + `pins[]` + `expected_version` + `supersede_of` + `relates_to[]` + semantic conflict block + `_unsorted` area)
- ✅ Phase 12 — agent ergonomics (`not_ready`에 `Failed[]`/`NextTools[]`/`Related[]` + `artifact.read(view=brief|full|continuation)` + server-issued `agent_id` + revision `source_session_ref`)
- ✅ Phase 13 — template artifact seed (`_template_{debug,decision,analysis,task}` — 포맷도 evolving artifact, Reader UI "Show templates" 토글)
- ✅ Phase 14 — operator settings + contract hardening (server_settings 테이블 + `pindoc-admin` CLI + `human_url_abs` + `expected_version` hard + `patchable_fields[]` + candidate warning + receipt TTL 30m + auth_mode rename)
- ✅ Phase 15 — dogfood-driven UX 완결 (Task heuristic + Area hierarchy + pin kind enum + task_meta + kanban-lite + Sidecar 연결 카드)
- ✅ Phase 16 — Today first screen + Change Group backend + summary cache + project markdown export (`pindoc.project_export`, `/api/p/:project/export`)

M1 + 3차 peer review 반영 + 1호 사용자 dogfood readiness 완료.

상세: [docs/12-m1-implementation-plan.md](docs/12-m1-implementation-plan.md) · 외부 피어리뷰 판단: [docs/14-peer-review-response.md](docs/14-peer-review-response.md)

## Quick start (현재 M1 개발자 경로)

현재 repo의 `docker-compose.yml`은 Postgres와 Pindoc HTTP daemon을 기본
서비스로 띄운다. TEI embedder는 옵션 profile이고, 기본 embedder는 daemon
프로세스 안의 Gemma provider다.

**사전 요구사항** (Windows/macOS/Linux 공통):
- Docker **27+** (Desktop 또는 engine)
- Go **1.24+** / Node **20.15+** & pnpm **10+** — host-native 개발이나
  Vite dev server를 직접 돌릴 때만 필요

```bash
# Postgres + Pindoc HTTP daemon + Reader SPA
docker compose up -d --build

# 선택: HTTP embedder를 명시적으로 쓸 때만 TEI 기동.
# daemon 컨테이너는 host PINDOC_EMBED_PROVIDER를 읽지 않고
# PINDOC_COMPOSE_EMBED_PROVIDER로만 opt-in한다.
# docker compose --profile tei up -d embed
```

Windows에서 기존 NSSM 서비스가 아직 5830 포트를 점유 중이면, 관리자
PowerShell에서 제거하기 전까지 Docker daemon을 임시 포트로 띄운다.

```powershell
$env:PINDOC_DAEMON_PORT = "5832"
docker compose up -d --build pindoc-server-daemon
```

Host-native 개발 경로가 필요하면 아래처럼 직접 실행할 수 있다.

```bash
docker compose up -d db
go mod tidy
go build -o bin/pindoc-server ./cmd/pindoc-server
./bin/pindoc-server -http 127.0.0.1:5830

# 또는 정적 웹 미리보기 (디자인 시스템 프로토타입, daemon과 같은 포트라 동시에 실행 불가)
cd web && pnpm install && pnpm dev   # http://localhost:5830
```

Windows 개발자는 데몬을 user-mode Scheduled Task로 등록하면 이후 agent가 admin 권한 없이 재시작할 수 있다.

```powershell
# 이전 NSSM 서비스가 있다면 관리자 PowerShell에서 1회만 실행
powershell -ExecutionPolicy Bypass -File scripts\uninstall-service.ps1

# 일반 PowerShell에서 user-mode daemon 등록 + 즉시 시작
powershell -ExecutionPolicy Bypass -File scripts\install-user-mode.ps1

# 코드 변경 후 agent/개발자가 직접 build + restart + health check
powershell -ExecutionPolicy Bypass -File scripts\dev-restart.ps1
```

**MCP 클라이언트에 등록**: 전역 또는 워크스페이스 MCP 설정에 아래 URL 하나를 넣는다. 새 세션에서 `pindoc.ping` 실행하면 handshake 성공. 임시 포트 `5832`로 띄웠다면 URL 포트만 `5832`로 바꾼다.

```jsonc
{ "mcpServers": { "pindoc": { "type": "http", "url": "http://127.0.0.1:5830/mcp" } } }
```

이 URL은 account-level entrypoint다. 프로젝트 scope는 URL path가 아니라 각
MCP tool input의 `project_slug`로 결정된다. 워크스페이스의 기본 project는
`PINDOC.md` frontmatter(`project_slug`)가 명시 source다.

Reader 첫 화면은 `/p/{project}/today`다. `/`도 기본 프로젝트의 Today로
redirect한다. Today는 최근 revision을 Change Group으로 묶고, deterministic
brief 또는 `PINDOC_SUMMARY_LLM_ENDPOINT`가 설정된 local/OpenAI-compatible
endpoint의 source-bound brief를 summary cache에 저장한다. Markdown export는
Reader의 export 버튼 또는 MCP `pindoc.project_export`로 실행한다.

### 데몬 모드 — 다수 워크스페이스에서 같은 Pindoc 인스턴스 attach

여러 워크스페이스에서 각자 다른 프로젝트를 다루려면 `pindoc-server`를 HTTP 데몬으로 한 번만 띄우고 모든 MCP 클라이언트가 같은 `/mcp` URL로 attach한다. 프로젝트 scope는 연결 URL이 아니라 각 tool input의 `project_slug`로 정해진다. 같은 포트가 Reader API(`/api/...`)와 liveness probe(`/health`)도 함께 서빙하므로 별도 `pindoc-api` 데몬을 띄울 필요가 없다.

```bash
# 데몬 띄우기 (1회만)
go build -o bin/pindoc-server ./cmd/pindoc-server
./bin/pindoc-server -http 127.0.0.1:5830
# 또는 PINDOC_HTTP_MCP_ADDR=127.0.0.1:5830 ./bin/pindoc-server
```

각 세션에서 `pindoc.project.current(project_slug="...")`를 호출하면 프로젝트 메타데이터와 capabilities가 반환된다. 현재 HTTP 데몬은 `transport=streamable_http`, `scope_mode=per_call`을 advertise한다. 데몬은 loopback(`127.0.0.1`)에 bind되어 외부에서 접근 불가 — 자기-호스팅 공개 시 인증 도입은 별 작업.

`pindoc.harness.install`이 생성하는 `PINDOC.md`는 YAML frontmatter(`project_slug`, `project_id`, `locale`, `schema_version`)를 포함한다. Frontmatter는 이후 workspace detection의 명시적 source이고, Section 12는 chip/parallel work가 시작·진행·merge·중단될 때 Pindoc Task status와 acceptance checkbox를 어떻게 갱신할지 규정한다.

Windows 기본 운영은 `scripts\install-user-mode.ps1` 이다. 기존
`scripts\install-service.ps1` NSSM 경로는 deprecated legacy 옵션으로만 남긴다.
지금 Windows에서 재시작 권한 문제가 나면 legacy NSSM 서비스가 아직 남아있는
상태다. 관리자 PowerShell에서 `scripts\uninstall-service.ps1`을 한 번 실행한 뒤,
일반 PowerShell에서 `scripts\install-user-mode.ps1`로 전환한다. macOS에서는
`~/Library/LaunchAgents/dev.pindoc.server.plist`에 `RunAtLoad` + `KeepAlive`를
두고 `pindoc-server -http 127.0.0.1:5830`을 실행하면 된다. Linux에서는
`~/.config/systemd/user/pindoc-server.service`를 만들고
`systemctl --user enable --now pindoc-server`를 사용한다.

상세 설계: Decision `mcp-scope-account-level-industry-standard`.

## Read the design

- [00 Vision](docs/00-vision.md) — 북극성, 디자인 원칙
- [01 Problem](docs/01-problem.md) — 실패 모드 F1–F6
- [02 Concepts](docs/02-concepts.md) — 5대 Primitive + 용어집
- [03 Architecture](docs/03-architecture.md) — MCP · Multi-project · 배포 시나리오
- [04 Data Model](docs/04-data-model.md) — Tier A/B, Area, Pin vs Related Resource
- [05 Mechanisms](docs/05-mechanisms.md) — M0 Harness Reversal부터 M7 Freshness까지
- [06 UI Flows](docs/06-ui-flows.md) — Wiki Reader · Cmd+K · Onboarding
- [07 Roadmap](docs/07-roadmap.md) — V1 / V1.1 / V1.x / V2
- [08 Non-Goals](docs/08-non-goals.md) — 하지 않을 것들
- [09 PINDOC.md Spec](docs/09-pindoc-md-spec.md) — Harness 파일 완전 스펙 + 해설
- [10 MCP Tools Spec](docs/10-mcp-tools-spec.md) — Tool input/output 스키마 + 예시
- [19 Area Taxonomy](docs/19-area-taxonomy.md) — 8 concern skeleton + sub-area 운영 규칙
- [20 Sub-area Promotion Policy](docs/20-sub-area-promotion-policy.md) — Tag → sub-area 승격, rename, merge, remove 절차
- [21 Cross-cutting Admission Rule](docs/21-cross-cutting-admission-rule.md) — cross-cutting 입장/해제 기준
- [Glossary](docs/glossary.md) — 모든 용어 정의 + 경계 (Meta-dogfooding 1호)
- [Decisions](docs/decisions.md) — Resolved decisions + Open questions

## License

To be decided. Candidate: **AGPL-3.0**.
