# Varn

> Where agent work becomes team memory.

**Varn**은 코딩 에이전트가 쓰고, 팀이 읽는 지식/태스크 워크벤치입니다.
MCP로 연결만 하면, 에이전트의 너저분한 세션 로그가 구조화된 문서와 태스크로 승격(promote)됩니다.

`var.gg` 생태계의 첫 플래그십 제품.

---

## Why Varn

2026년의 개발팀은 에이전트와 협업합니다. 하지만 에이전트의 출력은 대부분 세 갈래로 사라집니다.

1. **휘발** — 터미널 세션이 닫히면 디버깅 2시간이 증발
2. **고립** — 각자 터미널에 갇혀 팀에 공유되지 않음
3. **파편화** — Notion, Linear, Slack에 흩어진 흔적만 남음

그 결과 팀은 같은 문제를 여러 번 풉니다. 신입 에이전트는 매번 "오늘 입사한 신입"처럼 시작합니다. 머리만 최신이고 꼬리는 stale인 상태가 반복됩니다.

**Varn은 이 흐름을 바꿉니다.** 에이전트가 세션에서 만든 가치를 사람이 한 번 검수하면, 그것이 구조화된 팀 자산이 되고, 다음 에이전트 세션의 컨텍스트로 자동 로딩됩니다.

## Core Loop

```
Session (너저분한 원석)
    ↓  Promote
Artifact (구조화된 문서 / 태스크)
    ↓  Graph
Team Memory (검색·참조·컨텍스트 주입)
    ↓  Inject
Next Session
```

## What makes Varn different

- **MCP write-path 강제**: 에이전트는 자유롭게 못 씁니다. 의도(intent)를 선언하고, 기존 문서와의 충돌을 심사받고, 타입 스키마를 지켜야 발행됩니다.
- **Typed documents**: 분석/ADR/디버그/플로우/피처 — 타입별 스키마 네이티브. 포맷 드리프트 차단.
- **Git-pinned artifacts**: 모든 문서는 커밋/PR/파일경로에 고정. 코드 변경 시 stale 자동 표시.
- **TC as first-class citizen**: 테스트 케이스가 태스크의 1급 객체. Close 조건으로 강제.
- **Propagation ledger**: 문서 변경이 연관 태스크/TC/코드경로에 전파되어 "지금 실제와 어긋난 것"을 추적.

## Status

🚧 **Design phase** — 이 repo는 지금 설계 문서만 있습니다. 구현 전.

## Read the design

- [00 Vision](docs/00-vision.md) — 왜 만드는가, 북극성
- [01 Problem](docs/01-problem.md) — 풀려는 실패 모드들
- [02 Concepts](docs/02-concepts.md) — Session / Artifact / Promote / Graph
- [03 Architecture](docs/03-architecture.md) — 시스템 구조
- [04 Data Model](docs/04-data-model.md) — Typed documents, TC, dependency graph
- [05 Mechanisms](docs/05-mechanisms.md) — Write-intent router, propagation ledger 등
- [06 UI Flows](docs/06-ui-flows.md) — Promote UI, stale dashboard
- [07 Roadmap](docs/07-roadmap.md) — V1 / V1.x / V2
- [08 Non-Goals](docs/08-non-goals.md) — 하지 않을 것들

## License

To be decided. Candidate: AGPL-3.0 (Wiki.js와 동일, 네트워크 사용까지 카피레프트).
