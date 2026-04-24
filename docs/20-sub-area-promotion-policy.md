# 20. Sub-area Promotion Policy

이 문서는 Area taxonomy의 depth 1 sub-area를 언제 만들고, 이름을 바꾸고, 합치고, 제거하는지 정한다.
Top-level Area는 D0가 고정한 8 concern skeleton으로 유지하고, 프로젝트별 다양성은 Tag에서 시작해 충분히
반복될 때만 sub-area로 승격한다.

## 1. Promotion Criteria

Tag나 반복 표현을 sub-area로 승격하려면 다음 조건을 모두 만족해야 한다.

- **반복성**: 최근 30일 안에 같은 top-level Area 아래 3개 이상의 비템플릿 artifact에서 쓰인다.
- **안정성**: 한 번의 phase, sprint, launch-day fix처럼 일회성 이름이 아니다.
- **명사성**: 다음 artifact에도 재사용 가능한 domain noun이다. 상태(`done`, `blocked`), Type(`Decision`,
  `Task`), 사람/팀/agent 이름은 승격하지 않는다.
- **단일 부모**: 하나의 top-level Area에 자연스럽게 귀속된다. 둘 이상에 걸치는 reusable concern이면
  [cross-cutting admission rule](21-cross-cutting-admission-rule.md)을 따른다.
- **운영 가능성**: owner, description, review rule을 써도 어색하지 않다.

권장 threshold는 "30일 / 3개 artifact"다. 프로젝트 규모가 큰 경우 threshold를 올릴 수 있지만, 낮추려면
Decision artifact로 이유를 남긴다.

## 2. Promotion Procedure

1. 후보 Tag와 대표 artifact 3개 이상을 모은다.
2. 기존 sub-area와 이름이 겹치는지 확인한다.
3. Decision artifact를 작성한다. 본문에는 parent Area, slug, name, description, 예시 artifact, 대안을 포함한다.
4. 승인되면 `pindoc.area.create`로 depth 1 sub-area를 만든다.
5. 기존 artifact는 batch relabel 경로(`pindoc-admin relabel-artifacts` 또는 후속 MCP tool)로 이동한다.
6. 기존 Tag는 필요할 때만 유지한다. sub-area와 같은 의미의 Tag는 새 artifact에 중복 부여하지 않는다.

Sub-area 생성은 항상 agent를 통해 기록한다. raw SQL은 재난 복구나 migration 검증 외에는 쓰지 않는다.

## 3. Rename, Merge, Remove

### Rename

Rename은 의미가 동일하고 이름만 나쁠 때 사용한다. slug는 URL과 agent reference에 남으므로 가능하면 새
sub-area를 만들고 기존 artifact를 relabel한 뒤, 옛 slug를 문서에서 deprecated로 표시한다. 실제 slug rename이
필요하면 migration에 redirect/alias 방침을 같이 적는다.

### Merge

두 sub-area가 같은 질문에 답한다고 확인되면 더 넓고 안정적인 이름 하나로 합친다. Merge Decision에는
유지할 slug, 제거할 slug, 이동 대상 artifact 목록, 남길 Tag 여부를 포함한다.

### Remove

Remove는 artifact가 없거나 모두 다른 Area로 이동된 뒤에만 한다. FK가 남아 있으면 migration은 실패해야 한다.
삭제보다 `misc` 이동이 쉬워 보이더라도, 분류 의미가 이미 확인된 artifact는 적절한 subject area로 옮긴다.

## 4. Non-promotion Cases

다음은 sub-area로 만들지 않는다.

- 문서 형식: `decision`, `analysis`, `task`, `debug`, `screen`
- workflow 상태: `todo`, `in-progress`, `review`, `done`, `blocked`
- 사람, 팀, agent: `codex`, `claude`, `backend-team`
- 일회성 initiative: `phase-5`, `april-patch`, `launch-day-fix`
- 너무 깊은 경로: `system/mcp/tools`, `experience/ui/mobile`

깊이가 더 필요하면 sub-area를 늘리지 말고 Tag와 문서 내부 heading을 사용한다.

## 5. SLA

- `misc`에 들어간 artifact는 30일 안에 재분류하거나, 왜 아직 미분류인지 Task/Analysis에 남긴다.
- Taxonomy owner는 분기마다 `misc_ratio`, `cross_cutting_ratio`, `rehome_rate_30d`,
  `agent_suggestion_accuracy`를 확인한다.
- `misc_ratio >= 0.10`이 2주 지속되면 sub-area 후보를 조사한다.
- rename/merge/remove는 한 분기에 한 번 이하로 묶어 처리한다. 잦은 변경은 agent 기억과 URL 안정성을 해친다.

## 6. Examples

### Promote: `system/mcp`

MCP tool shape, harness install, context retrieval이 반복적으로 등장하고 모두 내부 구현 surface를 다룬다.
`mcp`는 stable recurring noun이므로 `system/mcp`로 승격한다.

### Reject: `phase-5`

Phase 이름은 작업 묶음이지 subject concern이 아니다. Task의 parent/Tag로는 쓸 수 있지만 sub-area로 만들지 않는다.

### Merge: `docs` → `content`

문서 copy, guide, reader body 구조가 모두 `experience/content` 질문에 답한다면 `docs`와 `content`를 나눌
필요가 없다. `content`를 유지하고 기존 `docs` artifact를 이동한다.
