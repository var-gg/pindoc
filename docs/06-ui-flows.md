# 06. UI Flows

주요 UI 화면과 상호작용 흐름. 와이어프레임은 ASCII로, 실제 디자인은 구현 단계에서.

## 핵심 원칙 (UI 전반)

1. **사람은 타이핑하지 않는다** — Artifact 본문의 편집 UI 없음. "수정이 필요하다" → 에이전트에게 피드백.
2. **Wiki Reader가 1차 UX** — 사용자가 가장 많은 시간을 보내는 화면.
3. **Referenced Confirmation** — 에이전트 채팅 내 확인 요청은 항상 링크 동반.
4. **딥링크 없이 동작** — URL을 에이전트 채팅에 던지면 `varn.wiki.read`로 fetch. 커스텀 스킴 의존 없음.

## 화면 맵

```
┌─────────────────────────────────────────────┐
│              메인 레이아웃                    │
│                                              │
│  ┌─────────┐  ┌──────────────────────────┐ │
│  │ Sidebar │  │   Main Content            │ │
│  │         │  │   (선택한 화면에 따라)     │ │
│  │ Wiki ★  │  │                           │ │
│  │ Approve │  │                           │ │
│  │ Sessions│  │                           │ │
│  │ Stale   │  │                           │ │
│  │ Graph   │  │                           │ │
│  │ Settings│  │                           │ │
│  └─────────┘  └──────────────────────────┘ │
└─────────────────────────────────────────────┘
```

주요 화면 6개:
1. **Wiki Reader** (★ 1차) — Artifact 열람, Area/Type 트리 네비, Related Resources
2. **Approve Inbox** — 에이전트 draft 승인/거절 (편집 없음)
3. **Sessions** — 세션 리스트 + 의미 검색 (F6)
4. **Stale Dashboard** — 낡은·전파 대기
5. **Graph Explorer** — 관계 시각화
6. **Settings** — Git repo, Domain Pack, 멤버, VARN.md mode

추가로 UI가 아닌 **에이전트-side UX**(Flow 2)가 1급 설계 대상.

---

## Flow 0: Harness Install & 첫 Checkpoint

**처음 한 번** 실행되는 플로우.

### Step 1: Install

```bash
$ cd my-project
$ varn install
→ VARN.md 생성 (프로젝트 루트)
→ CLAUDE.md / AGENTS.md / .cursorrules 에
  "See ./VARN.md for this project's agent protocol." 삽입
→ Varn 서버 주소, agent token, Domain Pack 선택 질문
```

### Step 2: Domain Pack 선택 (install-time)

```
┌─────────────────────────────────────────────┐
│  이 프로젝트의 Domain Pack 선택              │
├─────────────────────────────────────────────┤
│  ☑ Web SaaS/SI      (stable, 권장)           │
│  ☐ Game             (skeleton)               │
│  ☐ ML/AI            (skeleton)               │
│  ☐ Mobile           (skeleton)               │
│  ☐ CS Desktop       (skeleton, V2)           │
│  ☐ Library/SDK      (skeleton, V2)           │
│  ☐ Embedded         (skeleton, V2)           │
│                                              │
│  다중 선택 가능. 나중에 추가·변경 가능.        │
│                        [완료]                 │
└─────────────────────────────────────────────┘
```

### Step 3: 첫 세션

사용자가 평소처럼 Claude Code / Cursor 실행:
- 에이전트가 CLAUDE.md → VARN.md 자동 로드
- "이 프로젝트는 Varn이 연결되어 있습니다. Checkpoint mode: auto." 메시지
- 사용자가 뭔가 작업하다가 결론 도달 → 에이전트가 **역으로** 제안:

```
Agent: 이 부분 정리할 만한 단위가 된 것 같습니다.
  제안: Debug artifact 생성
    - scope: /Payment
    - title: "PG사 API 타임아웃 재시도 오류"
    - completeness: partial
  
  Preview: https://varn.myproject.dev/drafts/xyz
  
  승인하시면 저장합니다. (Y/n/편집지시)
```

---

## Flow 1: Wiki Reader + Continuation (★ 1차)

**매일 쓰는 화면.**

### Wiki Reader 기본 레이아웃

```
┌─────────────────────────────────────────────────────────────────┐
│  [🔍 결제 타임아웃          ]  [Area ▼]  [Type ▼]  [Completeness ▼]│
├──────┬─────────────────────────────────┬────────────────────────┤
│Tree  │  # PG사 API 타임아웃 재시도 오류  │ ─── Related ───        │
│      │     Debug · /Payment              │ 📁 Code                │
│ 📂 /Payment                              │ retry.ts              │
│  ├Feature                                │ gateway.ts            │
│  ├Debug ★                                │ api/payment.ts        │
│  ├Flow                                   │ [→ GitHub]            │
│  └ADR                                    │                        │
│ 📂 /Cart       ## Symptom                │ 🌐 External            │
│ 📂 /Auth       결제 요청 중 3%가 504...  │ PG provider docs      │
│                                          │                        │
│ ─ Type ─       ## Reproduction           │ ─── Graph ──           │
│ 📁 Decision    1. 결제 요청 (prod)      │ ← references           │
│ 📁 Analysis    2. 네트워크 지연 > 3s    │   ADR-042              │
│ 📁 Debug ★                               │ → validates            │
│ 📁 Flow        ## Hypotheses Tried       │   TC-payment-retry     │
│ 📁 Task        - [rejected] 서버 과부하  │                        │
│ 📁 TC          - [confirmed] 단일요청 ★  │ ─── Status ──          │
│ 📁 Glossary                              │ ✓ partial              │
│                ## Root Cause              │ 📌 pinned to retry.ts  │
│                retry.ts 단일 요청 구조    │ 🟢 verified 2h ago     │
│                                          │                        │
│                [▶ 이 문서로 대화 이어가기]│                        │
│                                          │                        │
└──────┴─────────────────────────────────┴────────────────────────┘
```

**핵심 요소**:
- **2축 트리 네비**: Area 축 / Type 축 (Tier A + 활성 Tier B)
- **본문 영역**: 마크다운 렌더, **편집 버튼 없음**
- **Related Resources 사이드 패널**: 별도 섹션, 본문 오염 없음. `type: code` 클릭 시 GitHub로 outbound
- **Graph 사이드**: 관계 artifact 바로 이동
- **Status 사이드**: completeness, pin 상태, verified 시각 (M7)
- **"이 문서로 대화 이어가기"** 버튼 — 클릭 시 artifact URL이 클립보드에 복사되고 간단한 도움말 (에이전트 채팅에 붙여넣기하세요)

### Continuation: URL → 에이전트 채팅

사용자 흐름:
1. Wiki에서 "이 문서로 대화 이어가기" → URL 복사
2. 에이전트 채팅에 URL 붙여넣기: `https://varn.myproject.dev/a/doc_debug_xyz 에 대해 이어서 논의하고 싶어`
3. 에이전트는 VARN.md 규약에 따라 `varn.wiki.read(url)` 호출
4. Continuation Context 수령 (본문 + neighbors + related_resources + recent_changes)
5. 대화 재개

**딥링크 없이도 완전 동작**. 커스텀 스킴·파트너십 불필요.

### "수정이 필요할 때"

사용자는 직접 편집 못 함. 대신:
- Wiki에서 **"수정 요청"** 버튼 → 에이전트 채팅으로 이동 (URL + "다음을 수정: ..." 템플릿)
- 에이전트가 변경 사항 반영한 propose 제출
- 사용자는 Approve Inbox에서 승인

---

## Flow 2: Checkpoint Proposal (Agent-side UX, MCP 응답)

**UI가 없는 플로우**. 사용자는 평소처럼 에이전트와 대화하고, 중요한 순간 에이전트가 먼저 제안.

### 에이전트가 사용자에게 (VARN.md 준수)

```
Agent: 이 작업 꽤 묶여서 정리할 단위가 됐습니다.
  
  정리 대상:
    - Debug "PG사 API 타임아웃 재시도 오류"
    - /Payment 에 속함
  
  관련 기존 문서:
    - [ADR-042 결제 재시도 정책](https://varn.example.com/a/adr-042)
    - [Analysis 결제 실패율](https://varn.example.com/a/analysis-payment-fail)
  
  영향 코드 (pin 후보):
    - [retry.ts L10-55](https://github.com/org/app/blob/a3f5e2c/src/payment/retry.ts#L10-L55)
  
  Preview: https://varn.example.com/drafts/xyz
  
  어떻게 할까요?
    [a] 이대로 저장 (partial)
    [b] 완결(settled)으로 저장
    [c] 편집 지시
    [d] 아직 저장하지 마 (거절)
```

**Referenced Confirmation 준수**: 1줄 요약 + URL + repo line range + 대안.

### Conflict 발견 시 (MCP 응답 → 에이전트 → 사용자)

내부 MCP 응답:
```json
{
  "status": "conflict_detected",
  "draft_id": "draft_xxx",
  "conflicts": [
    {"artifact_id": "doc_debug_old", "similarity": 0.88,
     "title": "결제 타임아웃 재시도 문제", "url": "https://varn.example.com/a/doc_debug_old"}
  ],
  "suggested_actions": [
    {"action": "update_existing", "target": "doc_debug_old"},
    {"action": "prove_distinct", "requires": "reason"}
  ]
}
```

에이전트가 사용자에게:
```
Agent: 관련 문서가 이미 있습니다:
  - [88% 유사] 결제 타임아웃 재시도 문제 (https://varn.example.com/a/doc_debug_old)
  
  업데이트하시겠습니까, 별개 이슈로 새로 만드시겠습니까?
  별개라면 이유를 알려주세요.
```

**MCP 응답이 에이전트 UX를 결정함**. 응답 스키마 설계 = 에이전트-side UX 설계.

---

## Flow 3: Approve Inbox

에이전트 propose가 Pre-flight + Conflict + Schema 전부 통과한 후 **사람의 최종 OK** 받을 때.

### 리스트 뷰

```
┌────────────────────────────────────────────────────────────┐
│                   Approve Inbox (3)                         │
├────────────────────────────────────────────────────────────┤
│                                                              │
│ 🟡 new · Debug · /Payment                                    │
│ PG사 API 타임아웃 재시도 오류                                 │
│ alice의 claude-code · 3m ago · draft_xyz                    │
│ [Preview ▸]  [OK]  [NO]  [피드백 요청]                       │
│ ─────────────────────────────────────                        │
│                                                              │
│ 🔵 modification · Flow · /Payment                            │
│ 결제 처리 플로우 V3 (retry 흐름 업데이트)                      │
│ alice의 claude-code · 15m ago                                │
│ [Preview ▸]  [OK]  [NO]  [피드백 요청]                       │
│ ─────────────────────────────────────                        │
│                                                              │
│ 🟢 related_resource verify · Feature · /Cart                 │
│ 장바구니 재시도 Feature · resources 2건 갱신 제안             │
│ bob의 cursor · 1h ago                                        │
│ [Diff ▸]  [OK]  [NO]                                        │
└────────────────────────────────────────────────────────────┘
```

### Preview (편집 없음)

```
┌──────────────────────────────────────────────────────────────┐
│  Preview — draft_xyz                                          │
├──────────────────────────────────────────────────────────────┤
│ Type: Debug · Area: /Payment · completeness: partial         │
│──────────────────────────────────────────────────────────────│
│                                                                │
│ # PG사 API 타임아웃 재시도 오류                                 │
│                                                                │
│ ## Symptom                                                     │
│ 결제 요청 중 약 3%가 504 Gateway Timeout...                    │
│                                                                │
│ ## Reproduction                                                │
│ 1. 결제 요청 (프로덕션)                                        │
│ ...                                                            │
│                                                                │
│ ## Hypotheses Tried                                            │
│ - [rejected] PG사 서버 과부하                                   │
│ - [rejected] 클라이언트 네트워크                                │
│ - [confirmed] Retry 없는 단일 요청                              │
│                                                                │
│ ── Pin 제안 ──                                                 │
│ 📁 src/payment/retry.ts @ a3f5e2c  [✓]                         │
│                                                                │
│ ── Related Resources 제안 ──                                   │
│ 📁 src/payment/gateway.ts  [✓]                                 │
│ 🌐 https://pg-provider.example/docs                            │
│                                                                │
│ ── 파생 제안 ──                                                 │
│ □ TC 3건                                                       │
│ □ Task 1건 "모니터링 알람 추가"                                  │
│                                                                │
│                 [← Back]  [피드백 요청]  [NO]  [OK]             │
└──────────────────────────────────────────────────────────────┘
```

**편집 없음** — 대신:
- **[OK]** → 그대로 commit
- **[NO]** → draft 폐기
- **[피드백 요청]** → 에이전트에게 자유 텍스트 피드백 전달 → 에이전트가 수정된 propose 재제출 → 다시 Inbox로

---

## Flow 4: Sessions (F6 해결)

```
┌──────────────────────────────────────────────────────────────┐
│                     Agent Sessions                            │
├──────────────────────────────────────────────────────────────┤
│  🔍 [결제 타임아웃            ]  의미 검색 ▼                   │
│  [필터: 내 세션 ▼]  [기간: 최근 30일 ▼]  [Agent: 전체 ▼]       │
│                                                                │
│  ────────────────────────────────────────────────              │
│  2h ago · Claude Code · alice · 2h 14m · 127 turns             │
│  🏷 결제 타임아웃 / retry / PG API                              │
│  [Open] [Promote ▸]  ⭐ 1 artifact                              │
│                                                                │
│  3d ago · Cursor · alice · 47m · 32 turns                      │
│  🏷 결제 실패율 분석                                            │
│  [Open] [Promote ▸]                                             │
│                                                                │
│  2w ago · Claude Code · alice · 1h 5m · 68 turns               │
│  🏷 PG사 타임아웃 재시도 / gateway / timeout                    │
│  [Open] [Promote ▸]  ⭐ 1 artifact                              │
│                                                                │
│  ────────────────────────────────────────────────              │
└──────────────────────────────────────────────────────────────┘
```

- **의미 검색** (F6 핵심): 키워드 + 벡터 유사도 hybrid. 한국어↔영어 gap 해소.
- **자동 태그**: 세션 내용 기반 자동 추출
- **Promoted 세션 마크**: ⭐ + 연결된 artifact 개수
- **교차 에이전트 검색**: Claude Code + Cursor + Cline 세션 통합

---

## Flow 5: Stale Dashboard

```
┌──────────────────────────────────────────────────────────────┐
│             🔔 Stale Dashboard                                │
├──────────────────────────────────────────────────────────────┤
│                                                                │
│  🔴 HIGH (3)                                                   │
│  ─────────                                                     │
│  Debug: "결제 타임아웃 재시도 문제"                              │
│  원인: src/payment/retry.ts 수정됨 (2h ago)                     │
│  최신 커밋: "Exponential backoff 도입"                          │
│  [에이전트에게 최신화 요청 ▸] [Dismiss]                         │
│                                                                │
│  ...                                                           │
│                                                                │
│  🟡 MEDIUM (7)  · 🔵 LOW (12)                                  │
└──────────────────────────────────────────────────────────────┘
```

- **"최신화 요청"**: 에이전트에 새 세션 task 던짐 → 에이전트가 pinned 코드 diff 보고 업데이트 propose
- V1은 간단 리스트, V1.1에서 3-tier 풍부한 UX

---

## Flow 6: Graph Explorer

```
┌──────────────────────────────────────────────────────────────┐
│                   Graph Explorer                              │
├──────────────────────────────────────────────────────────────┤
│  Search: [Feature: 결제 재시도        ]                        │
│                                                                │
│            ┌────────────────┐                                  │
│            │ Feature:       │                                  │
│            │ 결제 재시도    │                                   │
│            └──┬──┬──┬──────┘                                  │
│    implements │  │  │ validates                               │
│   ┌───────────┘  │  └──────────┐                             │
│   ▼              ▼              ▼                              │
│ [Task done]  [TC passing]  [TC passing]                        │
│                                                                │
│ derives_from                                                   │
│   ▼                                                            │
│ [Debug pinned 🔴]                                              │
│                                                                │
│ related_resource                                               │
│   ▼                                                            │
│ [code: retry.ts] [code: gateway.ts] [doc: PG external]         │
└──────────────────────────────────────────────────────────────┘
```

- V1: 간단 인접 리스트
- V1.1: d3/Cytoscape 기반 인터랙티브

---

## UX 원칙 요약

1. **사람은 타이핑하지 않는다** — UI에 편집 버튼 없음, 어디에도
2. **Wiki Reader가 제일 정성 들인 화면** — 매일 쓰는 곳
3. **Referenced Confirmation** — 에이전트 확인 요청은 항상 링크 동반 (이 프로토콜이 Varn을 다른 에이전트-wiki와 구분하는 매일의 경험)
4. **충돌·에러 화면이 기회** — "왜 막혔는지"를 에이전트가 사용자에게 링크와 함께 설명
5. **AI 자동 제안 + 사람 OK 패턴 반복** — 사람이 처음부터 쓰게 하지 않음
6. **URL → 대화 재개가 1급 이동 경로** — 딥링크 없이도 완전 동작
7. **Related Resources는 본문 밖 사이드 패널** — 메타데이터 분리
8. **Slack 공유는 1클릭** (V1.1) — 자체 메신저 없으니 연결 매끄럽게

## 디자인 가이드라인 (구현 단계)

- **톤**: Linear처럼 깔끔, Obsidian처럼 집중. 장식 최소화.
- **색**: 뉴트럴 + 상태별 (fresh 초록, stale 노랑, conflict 빨강, draft 회색)
- **밀도**: 정보 밀도 높게. 개발자 도구는 아이패드 앱이 아님.
- **키보드 우선**: 모든 주요 액션 단축키. `Cmd+K` 검색, `Cmd+Shift+A` Approve Inbox 등.
- **다크 모드**: Day 1부터.
