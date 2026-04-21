# 06. UI Flows

주요 UI 화면과 상호작용 흐름.

## UI 전반 원칙

### 1. 사람은 타이핑하지 않는다
Artifact 본문 편집 UI 없음. 어디에도. 수정은 "에이전트에게 지시" → 에이전트 propose.

### 2. Wiki Reader가 1차 UX
매일 들어오는 첫 화면.

### 3. Command Palette (Cmd+K)가 모든 네비의 1급 시민
Linear가 입증한 패턴. 메뉴·사이드바는 100+ artifact 넘으면 무용.

### 4. Referenced Confirmation
에이전트 채팅 내 확인 요청은 항상 링크 동반.

### 5. 딥링크 없이 동작
URL을 에이전트 채팅에 던지면 `varn.wiki.read` fetch. 커스텀 스킴 의존 없음.

### 6. Project Switcher 1급
Multi-project가 기본이므로 상단에 항시 노출.

### 7. Custom Dashboard Slot
운영자가 hero/sidebar/footer/ads 슬롯에 자유 주입. OSS core는 중립.

---

## 영감원 (Reference)

UI 원형으로 참고하는 제품들:

| 영역 | 모델 | 채택 요소 |
|---|---|---|
| 속도 & 키보드 | **Linear** | Cmd+K, 모든 액션 단축키, 즉각 응답 |
| Reader | **Obsidian / GitBook** | 넓은 본문, backlinks, 사이드 Related Resources |
| 네비 | **Linear Sidebar + Obsidian Graph** | Type 축 / Area 축 전환, Graph view 1급 |
| Review | **GitHub PR** | "수정 요청" 피드백 스타일 |
| 정보 밀도 | **Vercel / Linear** | 뉴트럴 + 상태색, 다크 Day 1 |

## 기존 Wiki Pain → Varn 해결

| 기존 불편 | Varn 해결 |
|---|---|
| 검색 개차반 (특히 한국어) | 세션 의미 검색(F6) + pgvector + Area/Type 필터 + Cmd+K |
| 트리 깊어지면 길 잃음 | Cmd+K + Graph view + 2축 전환 |
| "이 문서 언제 적었지" | Pin + stale + source_session 링크 + `last_verified_at` |
| 관련 문서 연결 수동 | Graph edge 자동 + Related Resources 사이드 |
| 포맷 제각각 | Typed Documents (Tier A/B) 강제 |
| 코드와 분리 | Pin + Git Outbound + Fast Landing |
| 사람이 쓰니 흐물흐물 | Agent-only write + 스키마 강제 |
| 대시보드 커스터마이징 약함 | Custom Dashboard Slot (core 기본) |

---

## 화면 맵

```
┌─────────────────────────────────────────────────────┐
│ Topbar:  [Project ▼]  Cmd+K 🔍  [Inbox ●]  [User]  │
├─────┬───────────────────────────────────────────────┤
│Side │                 Main Content                   │
│bar  │                                                 │
│     │    (선택한 화면에 따라)                         │
│Wiki │                                                 │
│Rev. │                                                 │
│Ses. │                                                 │
│Stal │                                                 │
│Grph │                                                 │
│Dash │                                                 │
│Set  │                                                 │
└─────┴───────────────────────────────────────────────┘
```

8개 화면:
1. **Wiki Reader** (★ 1차)
2. **Review Queue** (엣지 케이스, sensitive_ops=confirm 인 경우만)
3. **Sessions** (F6 검색)
4. **Stale Dashboard**
5. **Graph Explorer**
6. **Dashboard** (Custom Slot 포함)
7. **Project Switcher** (Topbar)
8. **Settings**

추가로 UI 없는 **Agent-side UX (MCP 응답)** 이 1급 설계 대상.

---

## Flow 0: Onboarding (`varn init` 7단계)

첫 설치 완주까지 **5분 이내**가 목표.

### CLI 흐름

```
$ cd my-project
$ varn init

[1/7] Server 감지
  로컬 localhost:5733 감지됨 → 자동 연결
  또는 "Varn 서버 URL" 입력
  또는 "docker compose up 하시겠습니까?" (자동 기동)

[2/7] 인증
  로컬: 자동 (~/.varn/token 생성)
  도메인: GitHub OAuth 브라우저 오픈 → 승인 → 토큰 수령

[3/7] Project 선택 / 생성
  기존 Project 목록:
    ◉ [새로 만들기]
    ○ shop-fe
    ○ shop-be
  
  (새로 만들기 선택 시)
  Project 이름? shop-fe
  Slug?        shop-fe
  연결 repo?   github.com/myorg/shop-fe (자동 감지, 확인)

[4/7] Domain Pack 선택 (신규 Project만)
  ☑ Web SaaS/SI (stable, 권장)
  ☐ Game (skeleton)
  ☐ ML/AI (skeleton)
  ☐ Mobile (skeleton)
  (다중 선택 가능. 나중에 추가 가능)

[5/7] Agent token 자동 발급
  ✓ 토큰 생성: ~/.varn/tokens/shop-fe.token
  ✓ 서버에 writer role 등록

[6/7] MCP 클라이언트 자동 설정
  감지된 에이전트:
    ✓ Claude Code  → ~/.config/claude-code/mcp.json 업데이트
    ✓ Cursor       → ~/.cursor/mcp.json 업데이트
    ○ Cline        (미설치, skip)
    ○ Codex        (미설치, skip)

[7/7] Harness 설치
  ✓ VARN.md 생성 (Domain Pack 반영)
  ✓ CLAUDE.md 에 '@VARN.md 참조' 추가
  ✓ AGENTS.md 에 '@VARN.md 참조' 추가
  ✓ .cursorrules 에 '@VARN.md 참조' 추가

✓ Setup complete in 4m 12s.

다음 단계:
  1. Claude Code를 여세요: cd . && claude
  2. "이 repo 훑어보고 Feature 스켈레톤 만들어줘" 같이 평소대로 대화
  3. 체크포인트 제안이 뜨면 Varn이 작동 중입니다.
```

### 실패 대응

각 단계에서 자동화 실패 시 **정확한 copy-paste 명령** 제시:
```
[6/7] Cursor 자동 설정 실패 (권한 에러)
  다음을 수동으로 ~/.cursor/mcp.json 에 추가하세요:

  {
    "mcpServers": {
      "varn": {
        "url": "http://localhost:5733/mcp",
        "headers": { "Authorization": "Bearer varn_xxx..." }
      }
    }
  }
```

---

## Flow 1: Wiki Reader + Continuation (★ 1차)

### 레이아웃

```
┌─────────────────────────────────────────────────────────────────┐
│ [shop-fe ▼]  Cmd+K 🔍 [Debug PG 타임아웃   ]  [📥 Inbox]  [User]│
├──────┬─────────────────────────────────┬────────────────────────┤
│Tree  │  # PG사 API 타임아웃 재시도 오류   │ ─── Related ───        │
│      │     Debug · /Payment · partial   │ 📁 Code                │
│ 📂 /Payment ★                            │ retry.ts  [→ GitHub]  │
│  ├Feature                                │ gateway.ts [→ GitHub] │
│  ├Debug ★                                │ api/payment.ts        │
│  ├Flow                                   │                        │
│  └ADR                                    │ 🌐 External            │
│ 📂 /Cart                                 │ PG provider docs      │
│ 📂 /Auth                                 │                        │
│ 📂 /Misc                                 │ ─── Graph ──           │
│                                          │ ← references           │
│ ─ Type ─                                 │   ADR-042              │
│ 📁 Decision                              │ → validates            │
│ 📁 Analysis                              │   TC-payment-retry     │
│ 📁 Debug ★                               │                        │
│ 📁 Flow        ## Symptom                │ ─── Status ──          │
│ 📁 Task        결제 요청 중 3%가 504... │ ✓ partial              │
│ 📁 TC                                    │ 📌 pinned retry.ts     │
│ 📁 Glossary    ## Hypotheses             │ 🟢 verified 2h ago     │
│                - [rejected] 서버 과부하  │                        │
│                - [confirmed] 단일요청 ★  │                        │
│                                          │                        │
│                [▶ 이 문서로 대화 이어가기]│                        │
│                [✏ 수정 요청 (에이전트)]   │                        │
│                                          │                        │
└──────┴─────────────────────────────────┴────────────────────────┘
```

**핵심 요소**:
- **Project Switcher** (Topbar): 현재 shop-fe. 클릭 시 접근 가능한 Project 목록
- **Cmd+K**: 모든 artifact/action 즉시 검색 (Type/Area/완결도/최근 필터)
- **2축 트리**: Type 축 / Area 축, 사용자 설정으로 기본값 (`layout_preference`)
- **본문**: 마크다운, **편집 버튼 없음**
- **Related Resources 사이드**: 본문 오염 없음. `type: code` 클릭 → GitHub outbound. `completeness: draft`는 disabled.
- **Graph 사이드**: neighbor 바로 이동
- **Status 사이드**: completeness, pin, verified
- **"이 문서로 대화 이어가기"**: URL 클립보드 복사 + 에이전트 붙여넣기 안내
- **"수정 요청 (에이전트)"**: 에이전트 채팅으로 이동 (URL + "다음을 수정: ..." 템플릿)

### Cmd+K Palette

```
┌───────────────────────────────────────────┐
│  🔍 [pg 타임아웃                 ]         │
├───────────────────────────────────────────┤
│  📄 Debug: PG사 API 타임아웃 재시도 오류   │
│  📄 Analysis: 결제 실패율 (PG 타임아웃 섹션)│
│  📄 Flow: 결제 처리 V3                     │
│                                            │
│  ─ Actions ─                               │
│  ⚡ Switch Project → shop-be               │
│  ⚡ New Session with this artifact         │
│  ⚡ Review Queue (2)                        │
│  ⚡ Settings                                │
│                                            │
│  ↑↓ navigate · ↵ open · ⌘↵ preview         │
└───────────────────────────────────────────┘
```

키보드 중심. 마우스 없이 Project 전환, artifact 오픈, 설정 전부 가능.

### Continuation via URL

1. Wiki에서 "이 문서로 대화 이어가기" 버튼 → URL 복사
2. 에이전트 채팅에 붙여넣기: `https://varn.myproject.dev/a/doc_debug_xyz 에 대해 이어서 논의`
3. 에이전트가 `varn.wiki.read(url)` → Continuation Context 수령 → 대화 재개

딥링크 없음. 이 플로우가 기본이자 유일.

---

## Flow 2: Agent-side UX (MCP 응답 예시)

**UI 없는 플로우**. 에이전트 채팅창이 UX 표면.

### Checkpoint Proposal

```
Agent: 이 작업 묶여서 정리할 만합니다.
  
  정리 대상:
    - Debug "PG사 API 타임아웃 재시도 오류"
    - Project: shop-fe · Area: /Payment
  
  관련 기존 문서:
    - [ADR-042 결제 재시도 정책](https://varn.example.com/a/adr-042)
    - [Analysis 결제 실패율](https://varn.example.com/a/analysis-fail)
  
  영향 코드 (pin 후보):
    - [retry.ts L10-55](https://github.com/org/app/blob/a3f5e2c/src/payment/retry.ts#L10-L55)
  
  Preview: https://varn.example.com/drafts/xyz
  
  어떻게 할까요?
    [a] 이대로 publish (partial)   ← 기본값, 대부분 여기
    [b] completeness=settled 로    ← sensitive_ops="confirm" 이면 Review Queue로
    [c] 편집 지시 (자유 텍스트)
    [d] 아직 저장하지 마
```

### Pre-flight NOT_READY

MCP 응답이 checklist로 돌아오면 에이전트 내부 루프 — 사용자에게는 보통 보이지 않음. 다만 에이전트가 잠시 "관련 문서 확인 중..."처럼 진행상황 표시.

### Conflict Detected

```
Agent: 관련 문서가 이미 있습니다:
  - [88% 유사] 결제 타임아웃 재시도 문제
    https://varn.example.com/a/doc_debug_old (bob, 2주 전)
  
  업데이트하시겠습니까, 별개 이슈로 새로 만드시겠습니까?
  별개라면 이유를 알려주세요.
```

### 발행 통지

```
Agent: ✓ 발행했습니다.
  - Debug: https://varn.example.com/a/doc_debug_xyz (partial)
  - Pin: retry.ts
  - 파생 TC 3건, Task 1건 제안 — 만들까요?
```

---

## Flow 3: Review Queue (Sensitive Ops, Optional)

`Project.settings.sensitive_ops == "confirm"` 이어야 활성. 기본은 `auto`라 이 화면은 비어있음.

### 리스트

```
┌────────────────────────────────────────────────────────────┐
│         Review Queue · shop-fe · (3 pending)                │
│                                                              │
│  ⚠ sensitive_ops=confirm 모드입니다. 일반 publish는          │
│    Review 없이 자동 처리됩니다. 이 큐는 민감 작업만.          │
├────────────────────────────────────────────────────────────┤
│                                                              │
│ 🔴 archive · Debug · /Payment                                │
│ PG사 API 타임아웃 재시도 오류 (오래된 버전)                   │
│ alice-claude · 3m ago · supersede 대상: doc_debug_new         │
│ [Preview ▸]  [OK]  [NO]  [피드백 요청]                       │
│                                                              │
│ 🟠 new Area · /Payment/Retry                                 │
│ alice-claude · 10m ago                                       │
│ 이유: "PG별 재시도 로직 분리 필요"                             │
│ [OK]  [NO]  [피드백 요청]                                    │
│                                                              │
│ 🟡 settled 승격 · Flow · /Payment                            │
│ 결제 처리 플로우 V3 → completeness: settled                   │
│ bob-cursor · 1h ago                                          │
│ [Preview ▸]  [OK]  [NO]  [피드백 요청]                       │
└────────────────────────────────────────────────────────────┘
```

**일반 publish는 이 큐에 절대 올라오지 않음**. 이게 Approve Inbox와 다른 점.

---

## Flow 4: Sessions (F6 핵심)

```
┌──────────────────────────────────────────────────────────────┐
│ [shop-fe ▼]       Agent Sessions                              │
├──────────────────────────────────────────────────────────────┤
│ 🔍 [결제 타임아웃         ] 의미 검색 ▼                       │
│ [Agent: 전체 ▼] [기간 30일 ▼] [내 세션만 ▼]                   │
│                                                                │
│ ────────────────────────                                      │
│ 2h ago · Claude Code · alice · 2h 14m · 127 turns              │
│ 🏷 결제 타임아웃 · retry · PG API                              │
│ ⭐ 1 artifact (Debug)                                          │
│ [Open] [Promote ▸]                                             │
│                                                                │
│ 3d ago · Cursor · alice · 47m · 32 turns                       │
│ 🏷 결제 실패율 분석                                             │
│ [Open]                                                          │
└──────────────────────────────────────────────────────────────┘
```

- 의미 검색 (F6 해결): 키워드+벡터 hybrid, 한국어↔영어 gap 해소
- 교차 에이전트 (Claude Code + Cursor + Cline + Codex 세션 통합)
- Promoted 세션 ⭐ 표시

---

## Flow 5: Stale Dashboard

```
┌──────────────────────────────────────────────────────────────┐
│             🔔 Stale Dashboard · shop-fe                      │
├──────────────────────────────────────────────────────────────┤
│  🔴 HIGH (3)                                                   │
│  Debug: 결제 타임아웃 재시도 문제                               │
│  원인: src/payment/retry.ts 수정 (2h ago)                      │
│  [에이전트에게 최신화 요청 ▸] [Dismiss]                         │
│                                                                │
│  🟡 MEDIUM (7)  ·  🔵 LOW (12)                                 │
└──────────────────────────────────────────────────────────────┘
```

V1 간단 리스트, V1.1 3-tier 풍부 UX.

---

## Flow 6: Graph Explorer

```
┌──────────────────────────────────────────────────────────────┐
│             Graph · Feature: 결제 재시도                      │
├──────────────────────────────────────────────────────────────┤
│          ┌────────────────┐                                    │
│          │ Feature:       │                                    │
│          │ 결제 재시도    │                                     │
│          └──┬──┬──────────┘                                    │
│   implements│  │ validates                                     │
│   ┌─────────┘  └──────┐                                        │
│   ▼                   ▼                                        │
│ [Task done]  [TC passing]                                      │
│                                                                 │
│ derives_from                                                    │
│   ▼                                                             │
│ [Debug pinned 🔴]                                               │
│                                                                 │
│ ── cross-project ──                                             │
│ references → shop-be:API "POST /cart/retry"                    │
└──────────────────────────────────────────────────────────────┘
```

- Cross-project edge 표시
- V1 간단 인접 리스트, V1.1 d3/Cytoscape 인터랙티브

---

## Flow 7: Project Switcher

Topbar의 Project 이름 클릭:

```
┌──────────────────────────────┐
│ Switch Project               │
├──────────────────────────────┤
│ ◉ shop-fe   writer + approver│
│ ○ shop-be   approver         │
│ ○ side-game admin            │
│                              │
│ ⚡ 새 Project 만들기           │
└──────────────────────────────┘
```

- Role 표시
- Cmd+K에서도 "Switch Project → ..." 로 접근

---

## Flow 8: Dashboard (Custom Slot)

각 Project의 Dashboard 화면. 운영자가 slot 구성.

### 기본 (Custom Slot 미설정)

```
┌────────────────────────────────────────────────────────────┐
│             Dashboard · shop-fe                             │
├────────────────────────────────────────────────────────────┤
│                                                              │
│  ── Stats ──                                                 │
│  Artifact  142  (partial 87 · settled 48 · draft 7)          │
│  Sessions  38 (this week)                                   │
│  Stale     12  [▸ Stale Dashboard]                          │
│                                                              │
│  ── Recent ──                                                │
│  • Debug PG 타임아웃 재시도 (2h ago)                         │
│  • Feature 결제 재시도 settled (1d ago)                      │
│  • Area /Cart/Retry 신설 (3d ago)                            │
│                                                              │
│  ── Activity ──                                              │
│  (graph: 주간 promote 건수)                                  │
│                                                              │
└────────────────────────────────────────────────────────────┘
```

### 공개 인스턴스 (`varn.var.gg` 등)의 Custom Slot

운영자가 `settings.yaml`로 활성화:

```
┌────────────────────────────────────────────────────────────┐
│                                                              │
│ ─── [hero slot: markdown] ────────                           │
│ Welcome to Varn (Public Demo).                               │
│ This is meta-dogfooding: Varn 프로젝트 자체가                │
│ 이 인스턴스에 관리되고 있습니다.                              │
│                                                              │
│ ─── [default Dashboard 본체] ──                              │
│ (Stats / Recent / Activity)                                  │
│                                                              │
│ ─── [sidebar slot: html] ────                                │
│ ❤ Support:                                                   │
│   [GitHub Sponsors](https://github.com/sponsors/var-gg)      │
│ 💰 Hosting cost: $28/month (transparent)                     │
│                                                              │
│ ─── [ads slot: ethicalads] ──                                │
│ (EthicalAds 개발자 타겟 광고 1칸, privacy-first)             │
│                                                              │
│ ─── [footer slot: html] ────                                 │
│ Open source · AGPL-3.0 · Built by var.gg                    │
└────────────────────────────────────────────────────────────┘
```

**OSS core 기본은 전부 null**. 슬롯 설정은 `settings.yaml` (server-side config) — 에이전트 경유 원칙의 예외 (운영자 영역).

---

## UX 원칙 요약

1. **사람은 타이핑하지 않는다** — UI 어디에도 편집 버튼 없음
2. **Wiki Reader가 일상 화면** — 가장 정성 들임
3. **Cmd+K 1급** — 모든 네비·액션
4. **Referenced Confirmation** — 에이전트 확인 요청 = 항상 링크 동반
5. **충돌·에러 화면이 기회** — 에이전트가 링크로 맥락 보여주며 설명
6. **AI 자동 제안 + 사람 방향 제시** 반복
7. **URL → 대화 재개가 1급 이동 경로**
8. **Related Resources는 본문 밖 사이드 패널**
9. **Review Queue는 sensitive ops에만** (기본 auto)
10. **Custom Dashboard Slot**: 운영 자율성 흡수

## 디자인 가이드라인

- **톤**: Linear 깔끔 + Obsidian 집중
- **색**: 뉴트럴 + 상태색 (fresh 초록, stale 노랑, conflict 빨강, draft 회색, settled 파랑)
- **밀도**: 정보 밀도 높게
- **키보드 우선**: Cmd+K 전면, 모든 액션 단축키
- **다크 모드**: Day 1
- **한글 타이포**: Pretendard 또는 시스템 폰트
