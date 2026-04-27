# 09. PINDOC.md — Harness 스펙

`PINDOC.md`는 에이전트가 **매 세션 시작 시 로드하는 행동 규약**입니다. `pindoc init` 또는 `pindoc.harness.install`이 프로젝트 루트에 자동 생성하고, `CLAUDE.md` / `AGENTS.md` / `.cursorrules`가 이를 참조합니다.

현재 harness 출력은 파일 상단 YAML frontmatter(`project_slug`, `project_id`, `locale`, `schema_version`)를 포함한다. 이 frontmatter는 workspace detection의 Priority 1 explicit override이고, `locale`은 canonical identity가 아니라 사용자의 보기 언어 선호다. Section X는 Applicable Rules Mechanism을 설명해 `context.for_task`의 `applicable_rules[]`를 worker가 자동으로 읽게 하고, Section 12는 chip/parallel work가 Pindoc Task의 open → claimed_done → verified 흐름과 acceptance checkbox 갱신을 놓치지 않도록 pre-spawn, during, after-merge, interrupted, retroactive policy를 명시한다.

이 문서는 두 부분으로 구성:
1. **템플릿 원문** — `pindoc init`이 생성하는 실제 파일 내용
2. **해설 & 구현 주의사항** — 각 섹션의 의도와 버전별 변화

---

## Part 1. 템플릿 원문

아래는 Web SaaS pack + `mode=auto` + `sensitive_ops=auto` 기본 구성의 PINDOC.md입니다. `pindoc init` 시 Project 설정에 따라 플레이스홀더가 채워집니다.

```markdown
# PINDOC.md — Agent Protocol for "{project_slug}"

> 이 파일은 Pindoc MCP가 생성·관리합니다. **에이전트는 매 세션 시작 시 이 파일을 읽고 규약을 따라야 합니다.**
> 사람 직접 편집은 Pindoc의 원칙 1(Human-writable surface is zero)에 따라 권장되지 않습니다.
> 변경이 필요하면 `pindoc config` CLI 또는 Settings UI를 통해서만 수정하세요.

## 0. 프로젝트 메타

```yaml
project_id:      proj_{xxx}
project_slug:    {shop-fe}
project_name:    {Shop Frontend}
server_url:      {https://pindoc.mycompany.dev | http://localhost:5733}
mcp_scope:       single-project  # V1: 이 MCP subprocess는 위 project_slug에 고정. 다른 프로젝트를 쓰려면 새 MCP 연결.
active_packs:    [web-saas]   # Tier A는 항상 활성, 여기는 Tier B만
mode:            auto         # auto | manual | off
sensitive_ops:   auto         # auto | confirm
generated_at:    2026-04-21T10:00:00Z
pindoc_version:  1.0.0
```

## 1. Checkpoint 규약

### 1.1 Mode

- `auto` (기본): 아래 휴리스틱에 맞으면 에이전트가 사용자에게 "정리할까요?" 역제안.
- `manual`: 사용자가 명시("정리해줘", "위키에", "체크포인트")한 경우에만 제안.
- `off`: 에이전트는 자율 제안하지 않음. 사용자가 명시해도 tool은 동작.

### 1.2 자율 제안 휴리스틱 (mode=auto 시)

다음 중 하나 이상 해당 시 사용자에게 "정리할까요?" 제안:

- **결론 도달 신호**: 한 주제 약 20~30턴 지속 후 "그럼 이렇게 가자", "결정", "확정" 등 표현
- **디버깅 resolution**: "해결됨", "root cause는 X였음" 등 명시
- **새 artifact 유발 이벤트**: 새 파일/모듈/스키마/ADR을 만들어냄
- **설계 문답 종료**: Q/A 왕복 후 단일 결론으로 수렴
- **사용자 피로 신호**: 같은 내용 반복 설명 징후 → 정리 후 URL로 대체 제안

### 1.3 거절 반복 자동 off

같은 세션 내 3회 거절 시 이 세션 동안 자율 제안 off. 다음 세션에서 다시 활성.

### 1.4 Checkpoint 제안 형식 (Referenced Confirmation 준수 필수)

```
이 작업 정리할 단위가 됐습니다.

정리 대상:
  - Type: {Debug | Feature | ADR | ...}
  - Area: {system/api | experience/ui | governance/taxonomy-policy | ...}
  - Scope: {1줄 요약}

관련 기존 문서:
  - [Title](URL) ({relation})
  ...

영향 코드 (pin 후보):
  - [path L10-50](repo URL)
  ...

Preview: {draft URL}

어떻게 할까요?
  [a] 이대로 publish (partial)     ← 기본
  [b] completeness=settled 로
  [c] 편집 지시 (자유 텍스트)
  [d] 아직 저장하지 마
```

## 2. Write 프로토콜

모든 artifact 생성·수정은 반드시 다음 순서:

```
1. pindoc.artifact.search 로 관련 선행 artifact 확인
2. pindoc.artifact.propose (intent + 본문)
3. 응답이 NOT_READY면 checklist 대응 후 재제출
4. READY → 자동 publish (sensitive op이면 Review Queue 대기)
```

**절대 금지**:
- `pindoc.artifact.search` 없이 바로 propose
- conflict 반려 시 force 대신 "별개" 주장
- 필수 필드 누락 상태로 propose 재시도

### 2.1 본문과 Graph edge 분리

Artifact 본문은 narrative 전용입니다. Purpose, Scope, Background,
Decision, Evidence 같은 설명 섹션을 쓰고, artifact 간 관계는 본문 섹션이
아니라 `relates_to` 입력 필드로 제출합니다.

**준수**:
```json
{
  "relates_to": [
    {"target_id": "task-reader-ia-refactor", "relation": "implements"}
  ]
}
```

**안티패턴**:
```markdown
## 연관

- implements -> task-reader-ia-refactor
```

`## 연관`, `## 역참조`, `## Dependencies / 선후`, `## 리소스 경로` 같은
H2가 graph metadata를 본문에 중복하면 서버는 publish를 막지 않고
`SECTION_DUPLICATES_EDGES` warning을 반환할 수 있습니다.

### 2.2 Applicable Rules Mechanism

정책 wiki는 `artifact_meta.rule_severity`(`binding` / `guidance` /
`reference`)를 설정하면 task worker에게 자동 적용됩니다. Task author가
references를 직접 붙이지 않아도 worker는 작업 시작 시
`pindoc.context.for_task`를 호출하고 응답의 `applicable_rules[]` excerpt를
스캔합니다. 더 깊은 맥락이 필요할 때만 해당 rule의 `agent_ref`로
`pindoc.artifact.read`를 호출합니다.

`applies_to_areas`를 생략하면 rule artifact의 own area + sub-area에
적용되고, cross-cutting child area(security/privacy/accessibility 등)의
rule은 모든 task에 자동 적용됩니다. `binding`은 의도적으로 위반하려면
사용자 확인이 필요한 강도이고, `guidance`는 기본 경로, `reference`는
관련 시 참고하는 배경입니다.

## 3. Referenced Confirmation 프로토콜

사용자에게 확인·승인 요청할 때 **반드시 다음 포함**:

1. **1줄 요약**
2. **관련 artifact URL(들)** — `{server_url}/a/{slug}` 또는 full URL
3. **코드 경로가 있으면** repo URL + line range (`https://github.com/.../blob/COMMIT/PATH#L10-L30`)
4. **여러 대안이면** 각각 URL. 3개 이상이면 비교 Analysis artifact로 묶어 단일 URL

**안티패턴** (금지):
```
❌ "이 부분 이렇게 고칠까요?"
❌ "PG 타임아웃 issue 봤어요. 수정할까요?"
```

**준수**:
```
✓ "결제 retry에 exponential backoff 도입 제안.
   - ADR-042: {url}
   - retry.ts L10-55: {github url}
   - 대안 비교: {url}
  진행할까요?"
```

## 4. Sensitive Ops 정책

현재 모드: `{sensitive_ops}`

### 4.1 Sensitive 작업 목록

- 삭제 / archive
- `settled` 완결 승격
- `supersede` (기존 artifact 대체)
- 신규 Area 생성
- `--force` (conflict HARD BLOCK 뚫기)
- 대규모 supersede (5개 이상 일괄)

### 4.2 모드별 동작

- `auto`: Sensitive 작업도 그대로 publish.
- `confirm`: Sensitive 작업은 Review Queue에 올라가고 approver role 사용자의 OK 대기.

일반 publish / modification / partial 기록은 어느 모드든 **auto**. Review Queue 안 탐.

## 5. Area 규율

- 모든 artifact는 **하나의 Area**에 속함 (단수).
- top-level Area는 고정 8개: `strategy`, `context`, `experience`, `system`, `operations`, `governance`, `cross-cutting`, `misc`.
- sub-area는 depth 1 only: `system/mcp`는 가능, `system/mcp/tools`는 불가.
- 미분류는 `/misc`로 넣되 temporary overflow로 취급하고, 가능한 한 subject area로 rehome.
- 새 sub-area가 진짜 필요하면 stable recurring noun인지 확인한 뒤 `pindoc.area.create` 호출 → Write-Intent Router 통과 → (sensitive_ops=confirm 이면 Review Queue).
- **여러 area에 걸친 관심사**는:
  - reusable named concern이면 `cross-cutting/<concern>`에 둔다.
  - 특정 subject area의 단일 instance면 subject area + Tag로 표현한다.
  - 여러 subject를 독립적으로 다루면 별도 artifact 여러 개 + Graph `relates_to`로 분리한다.
- `Decision`은 Artifact type이다. Decision artifact도 subject area에 배치하고 `decisions` Area는 사용하지 않는다.

## 6. URL 처리

사용자가 `{server_url}/a/...` 또는 외부 pindoc URL을 대화에 던지면:

```
1. pindoc.artifact.read(url) 호출
2. ContinuationContext 수령 (artifact + neighbors + related_resources + source_session + ...)
3. 번들 기반으로 대화 재개
```

**금지**: URL을 받고 무시하거나 재요약 없이 "알겠다"만 하기.

## 7. Cross-project 규약

- Agent token은 **하나의 project에만 writer** 권한을 가짐.
- Cross-project edge (예: FE Feature → BE API reference) 선언 시, 양쪽 project에 **read 권한**이 있어야 함.
- `pindoc.project.list` 로 접근 가능 project 확인, `pindoc.project.switch` 로 활성 project 전환.
- 본 프로젝트는 `{project_slug}` 임을 잊지 말 것.

## 8. Tool Quick Reference

| Tool | 언제 쓰나 |
|---|---|
| `pindoc.artifact.search` | propose 전 항상 |
| `pindoc.artifact.propose` | promote 시 |
| `pindoc.artifact.read` | URL/ID fetch |
| `pindoc.context.for_task` | 키워드로 관련 artifact+리소스 묶음 |
| `pindoc.graph.neighbors` | 특정 artifact 주변 |
| `pindoc.area.propose` | 신규 Area 신청 |
| `pindoc.resource.verify` | related_resources 재검증 |
| `pindoc.project.list` / `.switch` | Multi-project 전환 |
| `pindoc.tc.register` / `.run_result` | TC 관리 |

## 9. Pre-flight Check 타입별 체크리스트

Propose 시 Pindoc이 NOT_READY 응답으로 되돌릴 수 있는 체크. 통과하려면 아래 항목을 모두 충족해야 함.

### Decision (ADR)
- [ ] `alternatives` 최소 2개 선언
- [ ] 관련 선행 ADR을 search 로 확인 (겹치는 주제 없음)
- [ ] 1개 이상 Pin 또는 Related Resource

### Debug
- [ ] `hypotheses_tried` ≥ 1, 각 evidence 포함
- [ ] `reproduction` 단계 기술
- [ ] `symptom` 구체 (에러 메시지·로그 포함 권장)
- [ ] status=resolved면 `root_cause` + `resolution` 필수

### Feature
- [ ] `acceptance_criteria` ≥ 1
- [ ] `scope` 문단 (포함/제외 명시)
- [ ] dependencies 식별 (없으면 빈 배열 명시)

### Flow
- [ ] Mermaid 다이어그램 ≥ 1
- [ ] `actors` ≥ 1

### APIEndpoint (Web SaaS pack)
- [ ] method + path
- [ ] description
- [ ] request/response schema (권장, 필수 아님)

### Glossary
- [ ] `term` 명시
- [ ] `definition` 최소 1 문단

### 공통
- [ ] `source_session` 메타 포함 (당신의 세션 ID)
- [ ] `target_area` 선언
- [ ] Intent `reason` 자연어 문장

## 10. 사람의 역할

사람은 **방향 제시자**. 다음을 하지 **않습니다**:

- Wiki를 직접 타이핑
- 매 artifact 승인 (일반 publish는 auto)
- Pindoc 내부 config 수동 편집 (이 파일 포함 — `pindoc config` CLI 사용)

사람이 하는 것:
- 대화 중 방향 제시
- 체크포인트 제안에 OK/NO/편집지시
- Review Queue에 올라온 sensitive op 처리 (confirm 모드)
- "이거 지워/고쳐" 피드백

## 11. 버전 & 업데이트

이 파일은 `pindoc_version: {X.Y.Z}` 기준.
- Pindoc 서버가 업데이트되면 `pindoc harness update` CLI로 최신화.
- 수동 편집 후 `pindoc harness update` 는 사용자 변경을 덮어쓰니 주의 (운영자 config로 이동할 것).
```

---

## Part 2. 해설 & 구현 주의사항

### Section 0 (프로젝트 메타) 구현 주의

- `pindoc_version`은 **semver**로 관리. 에이전트 클라이언트가 이 값을 보고 호환성 판단 가능.
- `server_url`에 대한 접근 가능성은 에이전트 세션 시작 시 헬스체크 권장 (서버 다운 시 fallback 안내).
- `generated_at`은 **PINDOC.md 수정 시마다 갱신**. Git blame과 별개 추적.

### Section 1 (Checkpoint 규약) 구현 주의

- 휴리스틱 판정은 **에이전트 client 측 LLM**이 수행. Pindoc 서버가 개입하지 않음.
- `mode=auto` 의 false positive 를 줄이기 위해 **"정리할 구간"에 대한 요약을 먼저 사용자에게 보여준 뒤** 제안하는 것이 권장.
- 3회 거절 off는 **세션 단위** state — 세션 종료 시 리셋.

### Section 2 (Write 프로토콜) 구현 주의

- `pindoc.artifact.search` 없이 propose하면 보통 Pre-flight Check가 "선행 search 없음" 으로 NOT_READY 반환한다. 예외는 `project.create`가 동봉한 one-use `bootstrap_receipt`를 쓰는 첫 propose, 또는 서버가 empty/same-author area의 첫 N건으로 `receipt_exempted`를 반환하는 bootstrap create다.
- NOT_READY 응답 포맷은 [10-mcp-tools-spec](10-mcp-tools-spec.md)의 `pindoc.artifact.propose` 섹션 참조.
- `SECTION_DUPLICATES_EDGES`는 warn-only입니다. Artifact는 publish되지만,
  에이전트는 다음 update에서 관계를 `relates_to` 필드로 옮기도록 유도받습니다.

### Section 3 (Referenced Confirmation) 구현 주의

- **URL 생성**은 Pindoc 서버가 `propose` 응답의 `draft_url` 필드로 제공. 에이전트는 이를 그대로 인용.
- 코드 경로 URL은 git pinner 가 `{github_base}/blob/{commit}/{path}#L{start}-L{end}` 포맷으로 자동 생성 가능. 에이전트가 직접 조합하지 말고 `pindoc.resource.github_url(path, line_range)` 헬퍼 tool 이용 권장 (V1.1에서 추가).

### Section 4 (Sensitive Ops) 구현 주의

- `sensitive_ops` 정책은 Project 수준. 사용자 개인이 세션 내에서 일시 override는 불가 (규율 보호).
- 다만 특정 작업에 대해 `--explicit-confirm` 플래그로 에이전트가 사용자에게 한 번 더 확인 구할 수 있음 (권장이지만 강제 아님).

### Section 5 (Area 규율) 구현 주의

- `/misc` 는 install 시 자동 생성. 사용하지 말라고 유도하되 막지는 않음.
- Area slug는 lowercase kebab-case 표준. UI display name은 locale에서 별도 관리한다.
- 새 top-level Area 생성은 금지한다. Project-specific 확장은 고정 top-level 아래 depth 1 sub-area로만 둔다.
- sub-area로 문서 형식, workflow 상태, 사람/팀/agent 이름, one-off initiative를 만들지 않는다.
- `decisions` Area 입력은 거절하고 `Type=Decision` + subject area 조합을 안내한다.

### Section 6 (URL 처리) 구현 주의

- 에이전트가 외부 도메인 pindoc URL (다른 인스턴스) 을 만나면 **해당 인스턴스의 read-only API** 로 폴백 시도. 불가 시 사용자에게 "외부 pindoc 인스턴스입니다. 해당 인스턴스 계정이 필요" 안내.

### Section 7 (Cross-project) 구현 주의

- 에이전트가 writer 권한이 없는 project 의 artifact 를 read 하려면 `pindoc.artifact.read` 의 read 권한 확인 로직이 403 반환 가능. 이때 사용자에게 "이 project 접근 권한이 없습니다" 메시지.

### Section 9 (Pre-flight Checklists) 구현 주의

- 체크리스트는 **Pindoc 서버의 권위**. PINDOC.md 에 적힌 것은 참고용 사본. 실제 판정은 서버가.
- V1은 위 기본 체크리스트. Tier B pack별 / 팀별 커스터마이징은 V1.x+.

### 버전 마이그레이션 전략 (Section 11)

- PINDOC.md 스키마가 바뀌면 `pindoc harness update`가 **diff 기반 merge** 시도.
- 사용자가 직접 편집한 영역(특히 `mode`, `sensitive_ops` 같은 설정)은 **preserve**. 구조/섹션은 갱신.
- Breaking change 시 migration guide 문서 제공.

---

## 관련 문서

- 개념 정의: [02 Concepts](02-concepts.md)
- 메커니즘: [05 Mechanisms](05-mechanisms.md)
- MCP Tool 상세: [10 MCP Tools Spec](10-mcp-tools-spec.md)
- 용어집: [Glossary](glossary.md)
