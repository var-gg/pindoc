# Varn

> **The wiki you never type into.**
> Where agent work becomes lasting memory.

**Varn**은 사람이 직접 타이핑하지 않는 위키입니다.
모든 쓰기는 코딩 에이전트(Claude Code / Cursor / Cline / Codex)를 통해 일어나고,
사람은 승인·거절·방향 제시만 합니다.

`var.gg` 생태계의 첫 플래그십 제품.

---

## Why Varn

2026년의 개발자는 에이전트와 협업합니다. 하지만 에이전트의 출력은 세 갈래로 사라집니다:

1. **휘발** — 터미널 세션 닫히면 디버깅 2시간이 증발
2. **검색 지옥** — 월 단위로 쌓인 세션·채팅 중 "그때 그거" 못 찾음
3. **파편화** — Notion/Linear/Slack에 흩어진 흔적만

그 결과 개인·팀은 같은 문제를 N번 풉니다. 신입 에이전트는 매번 "오늘 입사"처럼 시작합니다.

**Varn은 이 흐름을 바꿉니다.** 에이전트가 세션에서 만든 가치를 사람이 한 번 OK 하면, 구조화된 자산이 되고, 다음 세션의 컨텍스트로 자동 주입됩니다.

## Core Loop

```
Harness (VARN.md)
   │
   ▼
Session ── checkpoint ──▶ Promote ──▶ Artifact ──▶ Graph ──▶ Next Session
(raw)     (에이전트 제안)   (에이전트 주도,          (typed,                  ▲
           ↕                사람 OK만)              pinned,                   │
          VARN.md 휴리스틱                          area-tagged)              │
                                                                              │
                                                   URL → agent fetch ─────────┘
                                                   (Continuation Context)
```

## What makes Varn different

- **Agent-only write surface** — UI에 편집 버튼 없음. 오탈자부터 아키텍처까지 전부 에이전트 경유.
- **Harness Reversal** — Varn MCP가 연결되면 `VARN.md`로 에이전트의 base 규약을 주입. 에이전트는 이 규율 아래 움직임.
- **Tool-driven Pre-flight Check** — `propose` 요청은 즉답 대신 체크리스트로 에이전트에게 **더 많은 일을 지시**. MCP가 응답 서버가 아니라 regulator.
- **Referenced Confirmation** — 에이전트가 사용자에게 확인 요청할 때 **항상 링크 동반**. 단편 설명 없이 맥락 위에서 판단.
- **Typed Documents (Tier A/B/C)** — Decision/Analysis/Debug/Flow/Task/TC/Glossary + Domain Pack. 포맷 드리프트 차단.
- **Git-pinned artifacts** — 커밋/PR/파일 경로 고정. 코드 변경 시 stale 자동.
- **Fast Landing** — "완벽한 인덱스" 대신 "핵심 리소스 1~3개로의 빠른 착륙". 쓰면서 점점 정확해짐 (M7 자가 검증).
- **TC as first-class citizen** — Feature close 조건으로 강제.

## Target Users

- **Solo 개발자** — F6(세션 검색 지옥) 해결만으로도 가치. 1급 시민.
- **2~10인 소규모 팀** — 에이전트 사용자 최소 1명.
- **자율 에이전트 환경 (OpenClaw 등)** — VARN.md mode=auto로 human-out-of-the-loop 지원.

## Status

🚧 **Design phase** — 이 repo는 지금 설계 문서 + 첫 dogfooding 기록. 구현 전.

**이 리포지토리 자체가 Varn의 첫 meta-dogfooding 사례입니다.** Varn이 아직 없어서 설계 문서를 수동 작성하고 있지만, V1 공개 시점에는 이 문서들이 Varn 인스턴스로 마이그레이션됩니다.

## Read the design

- [00 Vision](docs/00-vision.md) — 왜 만드는가, 북극성, 디자인 원칙
- [01 Problem](docs/01-problem.md) — 풀려는 실패 모드 F1–F6
- [02 Concepts](docs/02-concepts.md) — Harness / Session / Checkpoint / Artifact / Graph / Promote
- [03 Architecture](docs/03-architecture.md) — 시스템 구조, MCP Layer, Web UI
- [04 Data Model](docs/04-data-model.md) — Tier A/B 타입, Area, Pin vs Related Resource, Graph
- [05 Mechanisms](docs/05-mechanisms.md) — M0 Harness Reversal, M0.5 Pre-flight, M0.6 Referenced Confirmation, M1–M7
- [06 UI Flows](docs/06-ui-flows.md) — Wiki Reader, Approve Inbox, Agent-side UX
- [07 Roadmap](docs/07-roadmap.md) — V1 / V1.1 / V1.x / V2
- [08 Non-Goals](docs/08-non-goals.md) — 하지 않을 것들 (원칙 1부터)

## License

To be decided. Candidate: **AGPL-3.0** (Wiki.js와 동일, 네트워크 사용까지 카피레프트).
