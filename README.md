# Pindoc

> **The wiki you never type into.**
> Where agent work becomes lasting memory — pinned to code, written by agents.

**Pindoc**은 사람이 직접 타이핑하지 않는 위키입니다.
모든 쓰기는 코딩 에이전트(Claude Code / Cursor / Cline / Codex)를 통해 일어나고,
사람은 승인·거절·방향 제시만 합니다. 모든 문서는 코드 커밋·파일 경로에 **핀(pin)** 됩니다.

`var.gg` 생태계의 첫 플래그십 제품.
공개 인스턴스: **pindoc.org** (V1 공개 시 오픈 예정).

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
- **Multi-project by default** — 한 인스턴스 = 복수 프로젝트. FE/BE 분리·Solo 사이드 프로젝트·영세 팀 현실 지원.

## Target Users

- **Solo 개발자** — 1급 시민. "promote 안 하면 못 찾음"을 오히려 promote 문화로 해결.
- **2~10인 소규모 팀** — 에이전트 사용자 최소 1명.
- **자율 에이전트 환경 (OpenClaw 등)** — PINDOC.md mode=auto로 human-out-of-the-loop 지원.

## Status

🚧 **Design phase** — 구현 전. 이 repo가 **첫 meta-dogfooding 사례**입니다. Pindoc이 아직 없어 수동으로 작성 중이지만, V1 공개 시 이 문서들이 pindoc.org 인스턴스로 이관됩니다.

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

## License

To be decided. Candidate: **AGPL-3.0**.
