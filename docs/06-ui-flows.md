# 06. UI Flows

주요 UI 화면과 상호작용 흐름.

## UI 전반 원칙

1. **사람은 타이핑하지 않는다** — Artifact 본문 편집 UI 없음. 수정은 "에이전트에게 지시" → 에이전트 propose.
2. **Wiki Reader가 1차 UX** — 매일 들어오는 첫 화면.
3. **Command Palette (Cmd+K)가 모든 네비의 1급 시민** — Linear 패턴. 메뉴·사이드바는 100+ artifact 넘으면 무용.
4. **Referenced Confirmation** — 에이전트 채팅 내 확인 요청은 항상 링크 동반.
5. **딥링크 없이 동작** — URL을 에이전트 채팅에 던지면 `pindoc.artifact.read(url)` fetch.
6. **Project Switcher 1급** — Multi-project 기본이므로 상단 항시 노출.
7. **Custom Dashboard Slot** — 운영자가 hero/sidebar/footer/ads 슬롯에 자유 주입. OSS core 중립.
8. **Raw 세션 UI 없음** — "너절한 채팅 로그는 Claude 앱/Codex 앱에서". Pindoc은 정제된 artifact만.

---

## 영감원 (Reference)

| 영역 | 모델 | 채택 요소 |
|---|---|---|
| 속도 & 키보드 | **Linear** | Cmd+K, 모든 액션 단축키, 즉각 응답 |
| Reader | **Obsidian / GitBook** | 넓은 본문, backlinks, Related Resources 사이드 |
| 네비 | **Linear Sidebar + Obsidian Graph** | Type 축 / Area 축 전환, Graph view 1급 |
| 피드백 | **GitHub PR** | "수정 요청" 피드백 스타일 (에이전트에 지시) |
| 정보 밀도 | **Vercel / Linear** | 뉴트럴 + 상태색, 다크 Day 1 |

## 기존 Wiki Pain → Pindoc 해결

| 기존 불편 | Pindoc 해결 |
|---|---|
| 검색 개차반 (특히 한국어) | Artifact 의미 검색(F6) + pgvector + Area/Type 필터 + Cmd+K |
| 트리 깊어지면 길 잃음 | Cmd+K + Graph view + 2축 전환 |
| "이 문서 언제 적었지" | Pin + stale + source_session ref + last_verified_at |
| 관련 문서 연결 수동 | Graph edge (derived) + Related Resources 사이드 |
| 포맷 제각각 | Typed Documents (Tier A/B) 강제 |
| 코드와 분리 | Pin + Git Outbound + Fast Landing |
| 사람이 쓰니 흐물흐물 | Agent-only write + 스키마 강제 |
| 대시보드 커스터마이징 약함 | Custom Dashboard Slot |
| 세션 원본 찾기 지옥 | "그건 해당 앱에서" + promote된 것만 Pindoc 관리 |

---

## 화면 맵

```
┌──────────────────────────────────────────────────────┐
│ Topbar:  [Project ▼]  🔍 Cmd+K   [📥 Inbox]  [User] │
├─────┬────────────────────────────────────────────────┤
│Side │                 Main Content                    │
│bar  │                                                  │
│     │    (선택한 화면)                                 │
│Wiki │                                                  │
│Rev. │                                                  │
│Stal │                                                  │
│Grph │                                                  │
│Dash │                                                  │
│Set  │                                                  │
└─────┴────────────────────────────────────────────────┘
```

**7개 화면** (Sessions 화면 삭제됨 — Pindoc이 raw 세션을 저장하지 않음):

1. **Wiki Reader** (★ 1차) — 트리·본문·Related Resources·Graph·Status 사이드
2. **Review Queue** — 엣지 케이스, `sensitive_ops: confirm` 모드에서만
3. **Stale Dashboard** — 낡은·전파 대기
4. **Graph Explorer** — 관계 시각화
5. **Dashboard** — Stats + Custom Slot (운영자 구성)
6. **Project Switcher** — Topbar
7. **Settings**

추가로 UI 없는 **Agent-side UX (MCP 응답)** 이 1급 설계 대상.

---

## Flow 0: Onboarding (`pindoc init` 7단계)

첫 설치 완주 **5분 이내** 목표.

```
$ cd my-project
$ pindoc init

[1/7] Server 감지
  로컬 localhost:5733 감지 → 자동 연결
  또는 "Pindoc 서버 URL" 입력
  또는 "docker compose up 할까요?" 자동 기동

[2/7] 인증
  로컬: 자동 (~/.pindoc/token)
  도메인: GitHub OAuth 브라우저 오픈

[3/7] Project 선택/생성
  기존:
    ◉ [새로 만들기]
    ○ shop-fe
    ○ shop-be
  
  (새로 만들기)
  Project 이름?   shop-fe
  Slug?          shop-fe
  연결 repo?     github.com/myorg/shop-fe (자동 감지, 확인)

[4/7] Domain Pack 선택 (신규 Project만)
  ☑ Web SaaS/SI (stable, 권장)
  ☐ Game (skeleton)
  ☐ ML/AI (skeleton)
  ☐ Mobile (skeleton)

[5/7] Agent token 자동 발급
  ✓ ~/.pindoc/tokens/shop-fe.token
  ✓ 서버에 writer role 등록

[6/7] MCP 클라이언트 자동 설정
  ✓ Claude Code → ~/.config/claude-code/mcp.json
  ✓ Cursor      → ~/.cursor/mcp.json
  ○ Cline       (미설치)
  ○ Codex       (미설치)

[7/7] Harness 설치
  ✓ PINDOC.md 생성 (Domain Pack 반영)
  ✓ CLAUDE.md + AGENTS.md + .cursorrules 참조 추가

✓ Setup complete in 4m 12s.

다음 단계:
  1. Claude Code 여세요
  2. 평소처럼 대화 시작
  3. 체크포인트 제안 뜨면 Pindoc 작동 중
```

### 실패 대응

자동화 실패 시 **정확한 copy-paste 명령** 제시:
```
[6/7] Cursor 자동 설정 실패 (권한 에러)
  다음을 ~/.cursor/mcp.json 에 추가:

  {
    "mcpServers": {
      "pindoc": {
        "url": "http://localhost:5733/mcp",
        "headers": { "Authorization": "Bearer pindoc_xxx..." }
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
│      │     Debug · system/api · live   │ 📁 Code                │
│ 📂 system/api ★                         │ retry.ts  [→ GitHub]  │
│  ├Feature                                │ gateway.ts [→ GitHub] │
│  ├Debug ★                                │ api/payment.ts        │
│  ├Flow                                   │                        │
│  └ADR                                    │ 🌐 External            │
│ 📂 experience/ui                         │ PG provider docs      │
│ 📂 governance/review                     │                        │
│ 📂 misc                                  │ ─── Graph ──           │
│                                          │ ← references           │
│ ─ Type ─                                 │   ADR-042              │
│ 📁 Decision                              │ → validates            │
│ 📁 Analysis                              │   TC-payment-retry     │
│ 📁 Debug ★                               │                        │
│ 📁 Flow        ## Symptom                │ ─── Status ──          │
│ 📁 Task        결제 요청 중 3%가 504... │ live                   │
│ 📁 TC                                    │ 📌 pinned retry.ts     │
│ 📁 Glossary    ## Hypotheses             │ 🟢 verified 2h ago     │
│                - [rejected] 서버 과부하  │ Source: claude-code    │
│                - [confirmed] 단일요청 ★  │   @ 2h ago [open]      │
│                                          │                        │
│                [▶ 이 문서로 대화 이어가기]│                        │
│                [✏ 수정 요청 (에이전트)]   │                        │
└──────┴─────────────────────────────────┴────────────────────────┘
```

**핵심 요소**:
- **Project Switcher** (Topbar) — 현재 shop-fe. 클릭 시 접근 가능 Project 목록.
- **Cmd+K** — 모든 artifact/action 즉시. Type/Area/Completeness 필터.
- **2축 트리** — Type / Area. `layout_preference` 기본값.
- **본문** — 마크다운, **편집 버튼 없음**.
- **Related Resources 사이드** — 본문 분리. `type: code` 클릭 → GitHub outbound. `completeness: draft` 는 disabled.
- **Graph 사이드** — neighbor 이동.
- **Status 사이드** — **뱃지 단순화**: draft / live / stale / archived. pin 상태, verified 시각.
- **Source 사이드** — `source_session: SessionRef` 정보. "claude-code @ 2h ago" + [open] 링크 (있으면 해당 클라이언트로 딥링크 시도).
- **"이 문서로 대화 이어가기"** — URL 복사 + 안내.
- **"수정 요청 (에이전트)"** — 에이전트 채팅으로 이동 (URL + "다음을 수정: ..." 템플릿).

### Surface · Type · Area 위계

> Reader 는 Surface(뷰 모드) 를 최상위 축으로, Type 과 Area 를 그 위에 얹는 보조 필터 축으로 둔다. Surface 는 URL segment(`/p/:project/wiki` vs `/tasks`) 가 truth 이고, Area·Type 은 `?area=…&type=…` query string 으로 왕복한다 — 링크를 공유하면 필터 조합까지 그대로 복원된다. Wiki Surface 는 Task 를 제외한 모든 type 의 자연 집합이고 Tasks Surface 는 type=Task 로 고정되어 Sidebar 의 Type 섹션이 "Task · locked" 라벨로 바뀐다. Sidebar Area/Type 카운터는 "현재 Surface + 다른 축 필터" 기준으로 재계산되어 "UI 8" 뱃지인데 본문 6개 같은 counter drift 가 구조적으로 생기지 않는다(Linear / GitHub Issues 관습). Surface 전환 시 Area 는 탐색 연속성을 위해 유지되고 Type 은 Wiki↔Tasks 의미가 달라 리셋된다. Graph Surface 는 M1.5 React-ify 전까지 iframe stub 이라 필터 연동은 동일 시점에 들어온다. Decision `decision-reader-ia-hierarchy` + Task `task-reader-ia-refactor` 참조.

Tasks Surface 는 필터 적용 상태를 본문 상단에서 다시 선언한다. `Type=Task` 는 고정 chip 으로 보여 주고 Area 필터는 해제 가능한 chip 으로 표시한다. 필터 때문에 칸반 컬럼이나 전체 결과가 비면 "현재 필터에는 없음"과 "전체에는 몇 건 있음"을 함께 보여 주며, chip의 해제 버튼 또는 `Esc` 로 즉시 전체 scope 로 되돌아가게 한다. 이렇게 sidebar 선택 음영만으로 필터를 암시하지 않고, Task 탭 제목도 `Task · <scope> · 대기 N / 전체 M` 형태로 현재 scope 와 count 를 자기선언한다.

Tasks 칸반 카드는 dual-action으로 동작한다. 카드 body 클릭은 URL을 바꾸지 않고 우측 Inspector를 채워 status, priority, assignee, due date 같은 operational metadata를 제자리 편집하게 한다. 제목 클릭, `Enter`, double-click은 전체 Wiki 상세로 진입한다. `↑`/`↓`는 현재 필터 내 카드 선택을 이동하며, 선택이 없을 때 우측 패널은 "카드를 클릭하면 Inspector가 채워집니다" empty state를 보여 준다. Status 편집은 `open` / `claimed_done` / `blocked` / `cancelled`만 허용하고, `verified`는 VerificationReport + verify tool 경유 원칙을 유지한다.

Wiki 상세 Surface 에서 Area 선택은 본문을 닫는 필터가 아니라 **상세 scope** 로 live-bind 된다. 사용자가 상세 화면에서 Area 트리의 다른 항목을 클릭하면 article은 유지하고 상단 scope bar만 새 Area path로 갱신한다. 현재 artifact가 그 Area에 속하면 `[` / `]` 또는 `이전` / `다음` 버튼으로 같은 Area의 sibling artifact를 `updated_at desc` 순서로 순회한다. 현재 artifact가 선택된 Area 밖이면 sibling nav를 비활성화하고 "해당 area 목록 보기" CTA로 목록 전환을 명시한다.

본문 안의 `## 연관`, `## 역참조`, `## 리소스 경로`, `## References` 처럼 Sidecar의 structured data 와 의미가 겹치는 H2 섹션은 Reader render-layer에서 기본 접힘 상태로 표시한다. 원본 markdown 은 유지하되, 해당 블록 상단에 "Sidecar에 live 데이터가 있습니다" chip 을 두어 단일 진실 원본이 Sidecar임을 드러낸다. 단, Sidecar edge·backlink·pin 데이터가 모두 비어 있으면 본문이 유일한 단서일 수 있으므로 collapse 를 적용하지 않는다.

### 상태 뱃지 단순화

내부 3축(completeness/status/review_state) 조합을 UI 4뱃지로 축약:

| 뱃지 | 조건 |
|---|---|
| **draft** | `completeness=draft` |
| **live** | `status=published` & `completeness≥partial` & `review_state ∈ {auto_published, approved}` |
| **stale** | `status=stale` |
| **archived** | `status ∈ {archived, superseded}` |

`pending_review` 는 Wiki 에 노출되지 않음 — Review Queue에만.

### Trust Card + Sidecar Provenance

> Reader 상세 화면의 title 바로 아래에 **Trust Card** 1줄(3–5 secondary 뱃지)이 붙어 "이 지식을 믿어도 되는가 / 왜 여기 있는가 / 다음 세션에 들어가나"를 3초 안에 답한다. 구성: Trust class(Verified / Partially verified / Unverified / Conversation-derived) · Source summary(Code · N pins / Mixed / External / User chat) · Next-session policy(default / opt-in / excluded) · Confidence(low만 강조) · Audience(owner_only / approvers만 노출). artifact_meta 가 없는 legacy row 는 "Unclassified" 단일 뱃지로 graceful fallback. Sidecar의 **Provenance 블록**은 pins(kind별 그룹) · source_session_ref · next_context_policy rationale · age-based stale signal을 함께 내려 주며, 기존 draft/live/stale/archived 상태 뱃지와 시각 위계가 겹치지 않도록 secondary 톤을 쓴다. Task `reader-trust-card-sidecar-provenance-...` 참조.

Sidecar 섹션 위계는 C1 이후 **Identity strip → Quick actions → Relations → Provenance → Policy → Timeline → Meta** 순서다. Identity는 type chip + slug + lifecycle status를 한 줄/두 줄로 압축하고, Quick actions는 verify/update/share/agent-ref/history 같은 agent workflow 진입점만 아이콘 버튼으로 둔다. Relations는 outgoing/incoming을 하나의 섹션으로 합치고 방향 아이콘(`→` / `←`)으로 구분한다. Relation kind는 큰 text chip 대신 소형 icon pill로 낮추며, relation 이름은 tooltip으로 보존하고 artifact title을 1순위 타이포로 둔다. Provenance, Policy, Timeline, Meta는 low-frequency 정보라 기본 collapsed이며, 펼침 상태는 localStorage에 저장한다.

Trust Card 배지는 locale bundle을 통해 label과 tooltip을 모두 현지화한다. Tooltip은 단순 번역이 아니라 **의미 / 나타나는 조건 / 변경 경로** 3요소를 짧게 포함해야 한다. 예를 들어 `source_type=user_chat`은 "대화 기반" label과 함께 consent_state를 명시하라는 변경 경로를 제시하고, `RECOMMEND_READ_BEFORE_CREATE` warning은 "중복 후보" label과 함께 기존 artifact의 `update_of` 사용을 안내한다. Pin count도 영어식 plural 문구를 그대로 노출하지 않고 locale별 규칙(`핀 N`, `N pins`)을 따른다.

Reader의 visual language는 Pindoc 안의 Glossary artifact `visual-language-reference`가 단일 출처다. `/help/design-legend`는 기본 프로젝트의 해당 artifact로 redirect하며, badge popover나 shortcut overlay의 "전체 legend" 링크도 같은 artifact를 가리킨다. Type badge, trust badge, area count, relation icon, status/priority pill의 의미가 바뀌는 변경은 코드와 함께 이 Glossary를 같은 PR/작업 단위에서 갱신한다.

Reader는 `?` 또는 `Shift+/` 전역 키로 shortcut + symbol overlay를 연다. 이 overlay는 현재 Surface(Wiki / Tasks / Inbox / Graph)에 맞는 키보드 동작과 현재 화면에 등장하는 type badge, trust badge, area chip, relation icon, task status/priority pill의 경량 설명을 함께 보여준다. 입력 필드·textarea·select·contenteditable에 포커스가 있을 때는 `?`를 일반 입력으로 둔다. `Esc` 또는 같은 키 재입력으로 닫고, 하단의 "전체 legend 보기" 링크는 `visual-language-reference` artifact로 이어진다.

### Cmd+K Palette

```
┌───────────────────────────────────────────┐
│  🔍 [pg 타임아웃                 ]         │
├───────────────────────────────────────────┤
│  📄 Debug: PG사 API 타임아웃 재시도 오류   │
│  📄 Analysis: 결제 실패율 (PG 섹션)        │
│  📄 Flow: 결제 처리 V3                     │
│                                            │
│  ── Actions ──                             │
│  ⚡ Switch Project → shop-be               │
│  ⚡ New Session with this context          │
│  ⚡ Review Queue (2)                        │
│  ⚡ Settings                                │
│                                            │
│  ↑↓ navigate · ↵ open · ⌘↵ preview         │
└───────────────────────────────────────────┘
```

키보드만으로 모든 네비 완료.

### Continuation via URL

1. "이 문서로 대화 이어가기" → URL 복사
2. 에이전트 채팅에 붙여넣기: `https://pindoc.myproject.dev/a/doc_xxx 에 대해 이어서 논의`
3. 에이전트가 `pindoc.artifact.read(url)` → Continuation Context → 대화 재개

---

## Flow 2: Agent-side UX (MCP 응답 예시)

UI 없는 플로우. 에이전트 채팅창이 UX 표면.

### Checkpoint Proposal

```
Agent: 이 작업 묶어 정리할 만합니다.
  
  정리 대상:
    - Debug "PG사 API 타임아웃 재시도 오류"
    - Project: shop-fe · Area: system/api
  
  관련 기존 문서:
    - [ADR-042 결제 재시도 정책](https://pindoc.example.com/a/adr-042)
    - [Analysis 결제 실패율](https://pindoc.example.com/a/analysis-fail)
  
  영향 코드 (pin 후보):
    - [retry.ts L10-55](https://github.com/org/app/blob/a3f5e2c/src/payment/retry.ts#L10-L55)
  
  Preview: https://pindoc.example.com/drafts/xyz
  
  어떻게 할까요?
    [a] 이대로 publish (partial)   ← 기본값
    [b] completeness=settled 로    ← sensitive_ops="confirm" 이면 Review Queue
    [c] 편집 지시 (자유 텍스트)
    [d] 아직 저장하지 마
```

### Conflict Detected

```
Agent: 관련 문서가 이미 있습니다:
  - [88% 유사] 결제 타임아웃 재시도 문제
    https://pindoc.example.com/a/doc_old (bob, 2주 전)
  
  업데이트하시겠습니까, 별개 이슈로 새로 만드시겠습니까?
  별개라면 이유를 알려주세요.
```

### 발행 통지

```
Agent: ✓ 발행했습니다.
  - Debug: https://pindoc.example.com/a/doc_xyz (live, partial)
  - Pin: retry.ts
  - 파생 TC 3건, Task 1건 제안 — 만들까요?
```

---

## Flow 3: Review Queue (Sensitive Ops, Optional)

`Project.settings.sensitive_ops == "confirm"` 일 때만 활성. 기본 `auto` 는 이 화면 비어있음.

```
┌────────────────────────────────────────────────────────────┐
│         Review Queue · shop-fe · (3 pending)                │
│                                                              │
│  ⚠ sensitive_ops=confirm 모드. 일반 publish는 Review 없이    │
│    자동. 이 큐는 민감 작업만.                                │
├────────────────────────────────────────────────────────────┤
│                                                              │
│ 🔴 archive · Debug · system/api                              │
│ PG사 API 타임아웃 재시도 오류 (이전 버전)                     │
│ alice-claude · 3m ago · supersede 대상: doc_new               │
│ [Preview ▸]  [OK]  [NO]  [피드백 요청]                       │
│                                                              │
│ 🟠 new Area · system/provider-retry                          │
│ alice-claude · 10m ago                                       │
│ 이유: "PG별 재시도 로직 분리 필요"                             │
│ [OK]  [NO]  [피드백 요청]                                    │
│                                                              │
│ 🟡 settled 승격 · Flow · system/api                          │
│ 결제 처리 플로우 V3 → settled                                 │
│ bob-cursor · 1h ago                                          │
│ [Preview ▸]  [OK]  [NO]  [피드백 요청]                       │
└────────────────────────────────────────────────────────────┘
```

**일반 publish는 이 큐에 올라오지 않음** — 이게 이전 "Approve Inbox" 와 다른 점.

---

## Flow 4: Stale Dashboard

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

## Flow 5: Graph Explorer

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
- V1 간단 인접 리스트, V1.1 d3/Cytoscape

---

## Flow 6: Project Switcher

Topbar 클릭:

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

Cmd+K 에서도 접근 가능.

---

## Flow 7: Dashboard (Custom Slot)

각 Project의 Dashboard. 운영자가 slot 구성.

### 기본 (Custom Slot 미설정)

```
┌────────────────────────────────────────────────────────────┐
│             Dashboard · shop-fe                             │
├────────────────────────────────────────────────────────────┤
│  ── Stats ──                                                 │
│  Artifact  142  (partial 87 · settled 48 · draft 7)          │
│  최근 Promoted 12 (7일)                                      │
│  Stale     12  [▸ Stale Dashboard]                          │
│                                                              │
│  ── Recent Promoted ──                                       │
│  • Debug PG 타임아웃 재시도 (2h ago) · live                  │
│  • Feature 결제 재시도 settled (1d ago)                      │
│  • Area system/provider-retry 신설 (3d ago)                  │
│                                                              │
│  ── Activity ──                                              │
│  (graph: 주간 promote 건수)                                  │
└────────────────────────────────────────────────────────────┘
```

> **Recent Promoted** 가 이전 설계의 "Sessions 화면"을 대체. 사용자는 "최근 뭐 promote됐나" 만 보면 되고, raw 세션 리스트는 없음.

### 공개 인스턴스 (pindoc.org 등)

운영자가 `settings.yaml` 활성:

```
┌────────────────────────────────────────────────────────────┐
│ ── [hero slot: markdown] ────                                │
│ Welcome to Pindoc (Public Demo).                             │
│ This is meta-dogfooding: Pindoc 프로젝트 자체가               │
│ 이 인스턴스에서 관리됩니다.                                   │
│                                                              │
│ ── [default Dashboard] ──                                    │
│ (Stats / Recent / Activity)                                  │
│                                                              │
│ ── [sidebar slot: html] ────                                 │
│ ❤ Support:                                                   │
│   [GitHub Sponsors](https://github.com/sponsors/var-gg)      │
│ 💰 Hosting: $28/month (transparent)                          │
│                                                              │
│ ── [ads slot: ethicalads] ──                                 │
│ (개발자 타겟, privacy-first 광고)                              │
│                                                              │
│ ── [footer slot: html] ────                                  │
│ Open source · AGPL-3.0 · Built by var.gg                    │
└────────────────────────────────────────────────────────────┘
```

OSS core 기본은 모든 slot null. 설정은 `settings.yaml` (server-side config, 운영자 영역).

---

## UX 원칙 요약

1. **사람은 타이핑하지 않는다** — UI 어디에도 편집 버튼 없음
2. **Wiki Reader가 일상 화면**
3. **Cmd+K 1급**
4. **Referenced Confirmation** — 확인 요청 = 항상 링크 동반
5. **Raw 세션 UI 없음** — 정제된 것만 Pindoc 관심사
6. **충돌·에러 화면이 기회** — 에이전트가 링크로 맥락 설명
7. **AI 자동 제안 + 사람 방향 제시 반복**
8. **URL → 대화 재개가 1급 이동 경로**
9. **Related Resources는 본문 밖 사이드 패널**
10. **Review Queue는 sensitive ops + confirm 모드에만**
11. **Custom Dashboard Slot**: 운영 자율성

## 디자인 가이드라인

- **톤**: Linear 깔끔 + Obsidian 집중
- **색**: 뉴트럴 + 상태색 (draft 회색, live 파랑, stale 노랑, archived 음영, conflict 빨강)
- **밀도**: 정보 밀도 높게
- **키보드 우선**: Cmd+K 전면, 모든 액션 단축키
- **다크 모드**: Day 1
- **한글 타이포**: Pretendard 또는 시스템
