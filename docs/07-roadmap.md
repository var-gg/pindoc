# 07. Roadmap

> **Taxonomy note**: 이 파일명은 링크 안정성을 위해 유지한다. Area taxonomy에서는 top-level `roadmap`
> Area를 사용하지 않고, roadmap 성격의 artifact는 `strategy/roadmap` sub-area에 둔다.
> 전체 규칙은 [19 Area Taxonomy](19-area-taxonomy.md)를 따른다.

## 원칙

1. **V1 = Flagship 완성도 100%**. 깊이 우선.
2. **사용자 시그널 없이 확장 X** (실사용 30일+, 이슈 3건+).
3. **OSS first**. BM 본격 V2부터.
4. **Web SaaS/SI Domain Pack만 V1 완성**, 나머지 skeleton.
5. **Meta-dogfooding**: Pindoc 기획·운영 문서 자체를 Pindoc으로 관리.

## Scope Matrix

| 기능 | V1 | V1.1 | V1.x | V2+ |
|------|----|----|------|-----|
| **Project primitive + Multi-project** | ✅ | | | |
| **GitHub OAuth (self-host)** | ✅ | | | |
| **Zero-friction `pindoc init` CLI** | ✅ | | | |
| **Custom Dashboard Slot** | ✅ | | | |
| **Harness Reversal (PINDOC.md)** | ✅ | | | |
| **Tool-driven Pre-flight Check** | ✅ | | | |
| **Referenced Confirmation** | ✅ | | | |
| Write-Intent Router | ✅ | | | |
| **Auto-publish + Review Queue (opt)** | ✅ | | | |
| Typed Documents Tier A | ✅ | | | |
| Tier B Web SaaS (stable) | ✅ | | | |
| Tier B Game/ML/Mobile (skeleton) | ✅ 껍데기 | 성숙 | | |
| Tier B CS/Library/Embedded | | | skeleton | stable |
| Tier C Custom 타입 | | | | ✅ |
| Git Pinning | ✅ (단순) | ✅ (AST/LLM) | | |
| **Fast Landing (M6)** | ✅ | | | |
| **Resource Freshness M7** | ✅ (명시 트리거) | ✅ (자동) | | |
| Wiki Reader UI + Cmd+K | ✅ | | | |
| Project Switcher | ✅ | | | |
| **Artifact 의미 검색 (pgvector)** | ✅ | | | |
| Continuation Context | ✅ | | | |
| Task 기본 | ✅ | | | |
| TC gate (required_for_close) | ✅ | TC Runner 자동 | | |
| Event Bus / Propagation | 이벤트 + 간단 | 3-tier Dashboard | | |
| Slack/Discord 봇 | | ✅ | | |
| Graph Explorer 시각화 | 간단 인접 | d3 인터랙티브 | | |
| Hosted SaaS | | | | ✅ |
| **Raw 세션 파일 흡수** | ❌ | ❌ | ❌ | 실험 옵션 |

---

## V1 — "Agent-only Wiki가 작동하는 순간"

**목표**: Solo 1명이 세팅 후 30일 매일 쓸 수 있고, 2~3명 팀이 Multi-project로 운용.

**기간**: 설계 확정 후 **3~4개월**.

### V1 Feature Checklist

#### MCP & Harness
- [ ] MCP 서버 (핵심 tools, [10-mcp-tools-spec](10-mcp-tools-spec.md) — 배치 B)
- [ ] **`pindoc init` CLI** — 7단계 zero-friction
- [ ] PINDOC.md 템플릿 ([09-pindoc-md-spec](09-pindoc-md-spec.md) — 배치 B)
- [ ] CLAUDE.md / AGENTS.md / .cursorrules 자동 주입
- [ ] MCP 클라이언트 config 자동 주입 (Claude Code / Cursor / Cline / Codex)
- [ ] Pre-flight Check responder (타입별 체크리스트)
- [ ] Referenced Confirmation 규약
- [ ] Write-Intent Router + auto-publish
- [ ] Review Queue (sensitive_ops=confirm 모드)

#### Multi-project & Auth
- [ ] Project primitive + settings
- [ ] Permission (admin/writer/approver/reader, per-project)
- [ ] Agent token per-project scope + rotation + revoke
- [ ] **GitHub OAuth** (self-host)
- [ ] Local single-user 시나리오 (auto token)

#### Data
- [ ] Tier A 7 타입 (Decision/Analysis/Debug/Flow/Task/TC/Glossary)
- [ ] Tier B Web SaaS 4 타입 stable (Feature/APIEndpoint/Screen/DataModel)
- [ ] Tier B Game/ML/Mobile skeleton 등록
- [ ] Area taxonomy (고정 8 skeleton + depth 1 sub-area) + 신규 sub-area Router ([19-area-taxonomy](19-area-taxonomy.md))
- [ ] Pin (hard) + Related Resource (soft) 분리
- [ ] Event Bus + 기본 이벤트 타입
- [ ] SessionRef 경량 모델 (raw 세션 저장 없음)
- [ ] Continuation Context API

#### Git 연동
- [ ] GitHub App 기반 webhook
- [ ] 단순 stale 판정 (path change)
- [ ] **Git Outbound URL 생성** (artifact → github.com/.../blob)

#### Search & Fast Landing
- [ ] **Artifact 의미 검색** (pgvector, F6 해결 코어)
- [ ] Cmd+K palette
- [ ] `pindoc.context.for_task` (M6)
- [ ] `pindoc.resource.verify` (M7, V1 명시 트리거)

#### Web UI
- [ ] **Wiki Reader** (2축 트리, 사이드 패널, 편집 없음)
- [ ] **Cmd+K Palette**
- [ ] **Project Switcher** (Topbar)
- [ ] Review Queue (sensitive_ops=confirm)
- [ ] 간단 Stale 리스트
- [ ] **Dashboard + Custom Slot 메커니즘**
- [ ] Settings (OAuth, Domain Pack, 멤버, PINDOC.md mode, sensitive_ops, dashboard_slots)

> ❌ **Sessions 전용 화면 없음** — Pindoc이 raw 세션을 저장하지 않으므로 UI 화면도 없음. "최근 Promoted"는 Dashboard 섹션으로 흡수.

#### 배포 & 운영
- [ ] Docker Compose 설치
- [ ] 1분 내 로컬 기동
- [ ] Self-host 운영 가이드

#### 문서 & 마케팅
- [ ] Quickstart (5분 내 `pindoc init` + 첫 promote)
- [ ] MCP 연동 가이드 (Claude Code / Cursor / Cline / Codex)
- [ ] read-only live demo + 대표 screenshot (GIF/영상은 announcement 보조 자산)
- [x] 라이선스 확정: Apache 2.0

### V1 Non-goals

- ❌ 사람 직접 편집 (원칙 1)
- ❌ OSS core에 광고 embed (중립성)
- ❌ 매 artifact 사람 승인 강제
- ❌ **Raw 에이전트 세션 파일 흡수** (각 클라이언트 책임)
- ❌ 메신저 기능 (Slack 봇도 V1.1)
- ❌ 멀티 테넌트
- ❌ SSO, RBAC 세분화
- ❌ TC Runner
- ❌ Propagation Dashboard 3-tier
- ❌ Graph 시각화
- ❌ Hosted SaaS
- ❌ 모바일
- ❌ Tier C Custom

### V1 Launch Criteria

1. **Solo dogfooding 30일**: 저자 Solo 인스턴스 + 최소 2개 Project 30일 운용
2. **Meta-dogfooding**: Pindoc 기획·운영 문서를 Pindoc으로 관리. GitHub repo docs/ 가 Pindoc artifact로 마이그레이션됨
3. **External 2+ 인스턴스**: Solo 2명 or Solo 1 + 팀 1 (Multi-project 포함), 30일+ 사용
4. **pindoc.org 공개 인스턴스 오픈** (아래 섹션)
5. **문서 완비**: README, Quickstart, MCP 연동 가이드 4종, Self-host 가이드
6. **공개 데모**: read-only live demo + 대표 screenshot. GIF/영상은 announcement 보조 자산
7. **깨끗한 VM 10분 내 세팅** 검증
8. **라이선스 확정**

---

## pindoc.org 공개 인스턴스 운영 계획

V1 공개와 **동시** 가동. Pindoc 첫 공개 사례 + meta-dogfooding + 데모 + 운영비 투명 테스트.

### 목적

1. **데모**: 방문자가 Pindoc 실물 체험 (read-only)
2. **Meta-dogfooding**: Pindoc 프로젝트 자체가 이 인스턴스에서 관리됨 (docs/ → artifact)
3. **공개 로그**: 이슈·의사결정이 공개 Graph
4. **운영비 투명**: "이 서버 월 $XX로 돈다" 공개
5. **광고 1슬롯 실험** (EthicalAds)

### 기술 스펙 (잠정)

- 서버: 1 vCPU / 2GB RAM VPS (월 $10~30)
- DB: PostgreSQL + pgvector 동일 박스
- 도메인: **pindoc.org**
- TLS: Let's Encrypt
- 백업: 일 1회 → R2/S3

### Custom Slot 구성

```yaml
dashboard_slots:
  hero:     { type: markdown, source: ./public/hero.md }
  sidebar:  { type: html, source: ./public/sponsors.html }
  ads:      { type: ethicalads, publisher_id: pindoc-public }
  footer:   { type: html, source: ./public/footer.html }
```

### 권한

- 기본 **reader** 공개 (no login)
- OAuth 로그인 자유 Project 생성은 **V1에 안 함** — 공개 인스턴스는 데모+자체 관리 전용
- 사용자 자체 Project는 `pindoc init` 로 self-host 권장
- V2 Hosted SaaS 정식 오픈 검토

### 모니터링

- UptimeRobot (무료 외부)
- monthly hosting cost 자체 공개 (Dashboard sidebar)

---

## BM Roadmap — OSS first, 3 Phase

### Phase 1 — V1 공개 시점

- **EthicalAds 1슬롯** (공개 인스턴스만, ads slot)
- **GitHub Sponsors** 링크 (sidebar slot)
- **운영비 투명 공개** ("$XX/월")
- 목표: 서버비 커버 (월 $30 내외)

### Phase 2 — V1.1 이후 (star 1,000+)

- **Carbon Ads** 승인 시도 (premium advertiser)
- Sponsor wall 섹션
- Open Collective 등록 고려

### Phase 3 — V2 (star 5,000+ 시점 예상)

- **Managed hosted tier — Pindoc Cloud**
  - Solo 무료 (Project 1개)
  - 팀 유료 (Multi-project, member 해제)
  - Sentry / Supabase / n8n 모델
- 광고는 공개 데모에만, 유료 tier 광고 없음

### 원칙

- Core OSS 영원히 무료
- Tier 분할·광고 embed는 Custom Slot 메커니즘으로만
- Apache 2.0으로 MCP 클라이언트 생태계와 enterprise 내부 채택 마찰 최소화

---

## V1.1 — 피드백 반영

예상 (실 피드백 조정):
- Slack/Discord 봇
- Propagation Dashboard 3-tier
- M7 자동 트리거
- TC Runner
- Smart Assign
- Git Pin AST/LLM
- Graph d3/Cytoscape
- Tier B Game/ML pack 성숙

기간: V1 공개 후 2~3개월.

---

## V1.x — 성숙화

이슈 트래커 주도:
- 멀티 에이전트 동시 작업
- Tier B Mobile/CS Desktop 성숙
- 타입별 뷰 최적화
- 검색 품질 (re-ranking)
- (실험) 에이전트 클라이언트별 raw 세션 통합 옵션

---

## V2 — Hosted SaaS + Tier C

- Tier C Custom 타입 (YAML 스키마)
- 플러그인 시스템
- 공식 에이전트 partnerships
- **Managed hosted tier** (BM 본격)

BM 결정은 V2 진입 시점. 선호: Sentry/Supabase 스타일.

---

## 유즈케이스별 V1 가치

| 사용자 | V1 가치 |
|---|---|
| **Solo 개발자** | F6(과거 맥락 재발견) 해결 + 반-자동 아카이브 |
| **신규 프로젝트 부트스트랩** | "설계 정리해줘" → Tier A 스켈레톤 (이 리포가 1호) |
| **레거시 역공학** | 기존 repo 스캔 → Feature/Flow 자동 생성 |
| **2~10인 팀 (Multi-project)** | FE/BE 분리 + 매니지먼트 양쪽 접근 |
| **자율 에이전트 (OpenClaw 등)** | mode=auto, human-out-of-the-loop |

---

## 마일스톤 (잠정)

| 시점 | 마일스톤 |
|------|---------|
| M0 | 설계 확정 (지금) |
| M0–1 | 데이터 모델 + MCP 스켈레톤 + Project primitive + `pindoc init` |
| M1–2 | Harness / Pre-flight / Router / Tier A |
| M2–3 | Wiki Reader + Cmd+K + Fast Landing + Dashboard Slot |
| M3 | Solo dogfooding 시작 (2 Project 운용) |
| M3–4 | External alpha (2+ 인스턴스), pindoc.org staging |
| M4 | **V1 공개 + pindoc.org 오픈** |
| M4–6 | 피드백 → V1.1 준비 |
| M6 | V1.1 공개 |
| M6–12 | V1.x |
| Year 2 | V2 / Hosted 결정 |

---

## 위험 요소

### 기술
- MCP 스펙 변경 — 빠른 적응
- Pre-flight Check 과도 — per-type 세밀 조정
- 벡터 검색 품질 — threshold + 피드백 루프
- Git 연동 복잡도 — V1 작은 repo

### 제품
- Checkpoint 제안 피로 — PINDOC.md mode + 3회 거절 시 off
- "편집 금지 답답" — **원칙 방어**, 타협 없음
- "매 artifact 승인하고 싶다" — `sensitive_ops: confirm` 옵션 안내
- "raw 세션 검색도 되면 좋겠다" — scope 밖임을 명확히 설명
- Typed 경직 — Tier C (V2+)
- "Notion 있는데 왜?" — Agent-only + Fast Landing + Cmd+K + Pin 4개 gif 30초

### 비즈니스
- 경쟁자 (Cursor Rules, Cline Memory, Mem0, Continue.dev) — Agent-only + Harness Reversal + Pin 철학 차별화
- 메인테이너 번아웃 — 조기 커뮤니티, Tier B skeleton 기여 개방
- 광고 수익 미미 — Phase 1은 커버 목표, Phase 3 SaaS가 진짜 BM

---

## 성공/피벗 판단 (6개월)

- GitHub star < 500 → 포지셔닝 재검토
- 실사용 인스턴스 < 5 → 온보딩 대수술
- 이탈 "편집 금지 답답" → **방어**
- 이탈 "Checkpoint 피곤" → PINDOC.md 기본값 재조정
- 이탈 "raw 세션 검색 없음" → **scope 방어**, 대신 Promote 문화 유도 강화
- "Notion/Linear 충분" > 50% → 차별화 전달 재정의 (원칙 유지)

2+ 해당 시 피벗 검토. 단 **Agent-only write / No raw session / Harness Reversal은 피벗 대상 아님**.
