# Decisions & Open Questions

기획 진행 과정에서 내려진 결정 + 아직 답하지 않은 질문. 문서 수정 시마다 여기 기록 업데이트.

---

## ✅ Resolved Decisions

| Date | Decision | Rationale | 관련 문서 |
|------|----------|-----------|----------|
| 2026-04-21 | **Agent-only write surface (원칙 1)** | 사람이 직접 타이핑하는 경로 전부 금지. Pindoc의 구조적 해자. | [00 §원칙1](00-vision.md), [08 Never](08-non-goals.md) |
| 2026-04-21 | **Human Approve 단계 삭제, Auto-publish 기본** | "매 artifact 승인" 강제는 원칙 1과 모순 + 자율 에이전트 환경 충돌. Review Queue는 sensitive ops + confirm 모드만. | [02 §Promote](02-concepts.md), [05 M1.5](05-mechanisms.md) |
| 2026-04-21 | **Primitive 7 → 5** | Session/Checkpoint 를 보조 개념으로 격하. Project/Harness/Promote/Artifact/Graph 5개. | [02](02-concepts.md) |
| 2026-04-21 | **Raw 세션 파일 흡수 V1~V1.x Never** | 각 클라이언트 책임. "너절한 채팅 로그는 해당 앱에서". SessionRef (메타) 만 저장. | [01 F6](01-problem.md), [08 Never](08-non-goals.md) |
| 2026-04-21 | **Publish ≡ Promote 통합** | 외부 관찰자에게 둘은 같은 것. "Publish" 단어 사용 금지, 내부 "Commit" 만 구현 용어로. | [02](02-concepts.md), [05 M1](05-mechanisms.md) |
| 2026-04-21 | **Multi-project 기본 지원** | Solo 사이드 프로젝트, FE/BE 분리, 영세 팀 복수 프로젝트 현실 반영. | [02 §1](02-concepts.md), [03 원칙7](03-architecture.md) |
| 2026-04-21 | **GitHub OAuth V1 self-host 기본** | 개발자 타겟 전원이 GitHub 계정 보유. Git pin 토큰 재사용 가능. | [03 §보안](03-architecture.md) |
| 2026-04-21 | **Custom Dashboard Slot** | 운영 자율성을 fork/branch 분리 없이 흡수. OSS core 중립 유지. | [03 §Slot](03-architecture.md), [06 Flow 7](06-ui-flows.md) |
| 2026-04-21 | **BM Phase 1: EthicalAds + GitHub Sponsors** | AdSense 는 개발자 커뮤니티 fit 낮음. 공개 인스턴스만 광고, core OSS 중립. | [07 BM](07-roadmap.md) |
| 2026-04-21 | **프로젝트명 Varn → Pindoc** | "pin + doc" = 제품 핵심(코드-문서 결합)을 이름이 즉시 전달. var.gg 는 생태계, pindoc 은 제품으로 계층 분리. | [README](../README.md) |
| 2026-04-21 | **pindoc.org 공개 인스턴스 V1 오픈** | Meta-dogfooding + 데모 + 운영비 투명 테스트. | [07 §운영계획](07-roadmap.md) |
| 2026-04-21 | **Tier A/B/C 타입 체계** | Core 강제 + Domain Pack 선택 + Custom V2+. Scope 거버넌스 공백 해결. | [02 §4](02-concepts.md), [04 §Tier B](04-data-model.md) |
| 2026-04-21 | **Area 단수 (Artifact 1 Area)** | Cross-cutting 은 상위 Area 또는 별도 artifact + Graph relates_to로. | [04 §Area](04-data-model.md) |
| 2026-04-21 | **Pin(hard) vs Related Resource(soft) 분리** | 정합 필수 pin과 맥락 navigation resource 의미 구분. | [04 §Pin/ResourceRef](04-data-model.md) |
| 2026-04-21 | **Graph edge = Derived View** | Source of truth 는 Artifact 필드. 이중 저장 아님. | [02 §Graph](02-concepts.md), [04 §Graph](04-data-model.md) |
| 2026-04-21 | **MCP tool 네임스페이스 정리** | `pindoc.session.*` 삭제, `wiki.read` → `artifact.read` 흡수. | [10 MCP Tools](10-mcp-tools-spec.md) |
| 2026-04-21 | **AGENTS.md (복수) 통일** | OpenAI Codex 공식 컨벤션 준수. | [03 §MCP Layer](03-architecture.md) |
| 2026-04-21 | **UI 영감원: Linear + Obsidian + GitHub PR + Cmd+K** | 현대 Wiki UX 표준 반영. | [06 §영감원](06-ui-flows.md) |
| 2026-04-21 | **상태 UI 뱃지 4단계 단순화** | 내부 3축(completeness/status/review_state)은 유지하되 UI는 draft/live/stale/archived 로 축약. | [06 §상태뱃지](06-ui-flows.md), [04 §3축](04-data-model.md) |

---

## 🟡 Open Questions

구현 진입 전·직후 답이 필요한 것들. **P0 = V1 착수 전 결정 필요 / P1 = V1 중간 결정 / P2 = V1 이후**.

### P0 — V1 착수 전

#### Q1. pgvector 임베딩 모델 선택
한국어↔영어 혼용 환경에서 충분히 성능 내는 오픈/무료 모델은? 후보: `bge-m3`, `multilingual-e5-large`, OpenAI `text-embedding-3-small` (유료).
- **영향**: F6 해결의 품질 결정
- **결정 시점**: M1 (데이터 모델 + MCP 스켈레톤) 중
- **담당**: 구현자 + 저자 테스트

#### Q2. `pindoc init` 의 MCP 클라이언트 자동 감지 범위
Claude Code / Cursor / Cline / Codex 4개 외에 V1에 더 포함? (Zed, Windsurf, Aider 등)
- **영향**: Onboarding 마찰, 문서화 범위
- **결정 시점**: M2 (Harness install 구현) 전
- **현 추천**: V1 4개 + "나머지는 manual config copy-paste 가이드"

#### Q3. Conflict Check threshold (0.85 / 0.7) 기본값 적절성
현재 하드코딩. 팀·도메인·언어별 민감도 다를 것.
- **영향**: False positive → 에이전트 루프 증가, False negative → 중복 artifact
- **결정 시점**: Solo dogfooding 후 조정
- **현 추천**: V1 하드코딩으로 시작, V1.1 Settings 에 tunable 추가

### P1 — V1 구현 중

#### Q4. Session 보존 기간 vs SessionRef 만 저장 전환
SessionRef 만 저장한다고 결정했으므로 기존 설계의 "90일 보존"은 **의미 없어짐**. 하지만 SessionRef 의 title_hint 외에 에이전트가 함께 제출한 "핵심 turn 발췌" 같은 optional 필드를 둘 것인가?
- **영향**: 에이전트가 promote 시 추가 컨텍스트 포함 가능성 vs 스토리지·프라이버시
- **결정 시점**: M1-M2
- **현 추천**: V1은 순수 SessionRef 메타만. "핵심 turn 발췌"는 V1.1+ 옵션.

#### Q5. Agent token rotation UX
90일 rotation 기본. rotation 직전·후 에이전트 세션이 끊기지 않으려면?
- 후보: (a) rotation 예정 30일 전 경고 UI + CLI, (b) 두 토큰 동시 유효 기간 (grace period), (c) 자동 rotation + 새 토큰 MCP client 에 자동 갱신
- **영향**: 운영 마찰
- **결정 시점**: M2
- **현 추천**: (b) 14일 grace + (a) 경고

#### Q6. Cross-project edge 권한 모델 세부
FE Feature → BE API reference 선언 시:
- 에이전트가 양쪽 project에 read 권한이 있어야 한다는 원칙은 있음
- 그런데 edge "생성" 자체는 어느 project에 귀속? (source artifact의 project?)
- edge 삭제는 누가 가능?
- **영향**: Multi-project 의 실제 운영 경험
- **결정 시점**: M2
- **현 추천**: edge는 source artifact의 project에 귀속, 양쪽 read 권한으로 생성, source project 의 writer가 삭제

#### Q7. PINDOC.md 휴리스틱 세부 임계값
"한 주제 20~30턴" 같은 숫자는 추정. 실측 필요.
- **영향**: False positive 많으면 사용자 거부감, 적으면 promote 문화 약화
- **결정 시점**: Solo dogfooding 단계
- **현 추천**: V1 초기 값 + 로깅 → 30일 후 조정

#### Q8. 레거시 repo import 정확도 타겟
"기존 repo 스캔 → Feature/Flow 자동 생성" 은 V1 기능. 생성된 artifact 품질 기준?
- **영향**: 유즈케이스 3 (레거시 역공학)
- **결정 시점**: M2-M3
- **현 추천**: V1은 "스켈레톤만, 사용자가 사후 정제" — 완벽 자동화 목표 X

### P2 — V1 이후

#### Q9. V1 → V1.1 스키마 마이그레이션
Tier B pack 스키마가 성숙하면서 필드 변경 가능성.
- **영향**: 기존 사용자 artifact 보존
- **결정 시점**: V1.1 준비 시
- **현 추천**: Additive-only (필드 추가만, 제거·rename 금지) V1.x까지 유지

#### Q10. TC Runner 구현 구체 (V1.1)
- AI-가능 TC 실행 환경: 사용자 CI / Pindoc 서버 측 / 에이전트 sandbox?
- 플레임워크: jest / pytest / playwright 각각의 runner 통합 방식
- **결정 시점**: V1.1 스펙 작성 시

#### Q11. Hosted SaaS BM 구체 (V2)
Sentry/Supabase/n8n 중 어느 쪽에 더 가깝게?
- 가격 모델 (per-user / per-project / flat)
- 무료 tier 범위
- **결정 시점**: V2 진입 시점

#### Q12. 에이전트 클라이언트별 raw 세션 통합 (V2+)
V1~V1.x Never이지만 V2+ 에서 옵션 검토 시:
- Claude Code hooks 시스템 활용?
- 각 벤더 파트너십 필요 여부?
- 프라이버시 모델 (opt-in)
- **결정 시점**: V2 초기

#### Q13. Graph `relates_to` 선언 시 Pre-flight Check
약한 관련성 edge도 Pre-flight 를 거쳐야 하나? 아니면 자유 선언?
- **현 추천**: 자유 선언 (low friction). 대신 검색 weight 낮게.

#### Q14. Tier B Community 기여 리크루팅 전략
Game/ML/Mobile pack 성숙은 각 도메인 기여자 등장에 의존. 어떻게 유치?
- **현 추천**: V1 공개 후 pindoc.org 에 "Domain Pack Wanted" 공지. GitHub Discussion 활용.

---

## 🔵 Deferred (나중에)

확실히 V2+ 이후로 미룬 것들. 지금 논의 불필요.

- Tier C Custom 타입 시스템
- Hosted SaaS 세부 (Q11과 별개, BM 이외 기술)
- 모바일 read-only 뷰어
- AST/LLM 기반 stale 판정
- 다국어 UI
- 데스크톱 CS wrapper
- Plugin 시스템
- SSO (GitHub 외 IdP)
- 실시간 협업 (구조적 해당 없음)

---

## 📋 How to update this file

문서 수정·의사결정이 발생하면 이 파일에 반영:

1. **새 결정**: `Resolved Decisions` 테이블에 date + decision + rationale + 관련 문서 링크 추가
2. **새 질문**: `Open Questions` 에 P0/P1/P2 구분해서 추가
3. **질문 → 결정 전환**: Open Questions 에서 제거 + Resolved 에 추가
4. **Commit 메시지**: `docs(decisions): add Q{N} / resolve Q{N}` 규칙

V1 공개 후 이 파일도 Pindoc Artifact 로 마이그레이션 — Analysis 타입으로 저장하고 `references` edge 로 관련 결정 artifact 들과 연결할 것.
