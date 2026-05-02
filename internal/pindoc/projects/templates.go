package projects

// TemplateSeed describes one "best practice structure" artifact that every
// project gets on creation. Seeded for the pindoc project by migration
// 0006_template_artifacts.sql; seeded for new projects by CreateProject
// via seedTemplates below.
//
// Bodies below mirror revision 1 of each _template_* artifact on the
// pindoc-self instance (Task task-templates-prose-first-redesign). Prose-
// first design: the narrative register here teaches agents how to write
// artifacts without leaking "Alt A — pros/cons/rejected" shorthand back
// into their user-facing chat register.
type TemplateSeed struct {
	Slug  string
	Type  string
	Title string
	Body  string
}

// TemplateSeeds is the V1 default set. Tier A core types plus the
// SessionHandoff convention template (stored as Flow because artifact types
// remain fixed; "SessionHandoff" is a convention, not a new type).
var TemplateSeeds = []TemplateSeed{
	{Slug: "_template_debug", Type: "Debug", Title: "Template — Debug artifact", Body: templateDebugBody},
	{Slug: "_template_decision", Type: "Decision", Title: "Template — Decision (ADR-style) artifact", Body: templateDecisionBody},
	{Slug: "_template_analysis", Type: "Analysis", Title: "Template — Analysis artifact", Body: templateAnalysisBody},
	{Slug: "_template_task", Type: "Task", Title: "Template — Task artifact", Body: templateTaskBody},
	{Slug: "_template_session_handoff", Type: "Flow", Title: "Template — Session handoff artifact", Body: templateSessionHandoffBody},
}

const templateDebugBody = `<!-- validator: required_h2=증상,재현,가설,원인,해결,검증; required_keywords=reproduction,repro,재현,증상,symptom,resolution,root cause,원인,해결 -->
> **This artifact is a template.** Read it before proposing a new
> ` + "`Debug`" + ` artifact so the resulting document matches the house structure.
> The template itself is an ordinary artifact — improvements land as
> revisions (` + "`update_of`" + `) on this slug. Do **not** copy-paste this body
> verbatim into a new Debug; rewrite each section with your specific case.

> **Register guide.** Debug는 증상에서 해결까지의 흐름을 기록하는 artifact이므로 각 H2 섹션은 narrative 중심이다. 재현 단계와 검증 체크는 본질적으로 checklist이므로 ` + "`- [ ]`" + ` 포맷이 자연스럽지만, 가설(Hypotheses tried)은 "가설 1 — 한 줄 축약 — 결과" 같은 bullet 대신 각 가설을 한 문단 narrative로 푼다. 어떤 가설을 왜 시도했고 어떤 증거로 맞거나 틀렸는지를 문장으로 써 놔야 나중에 같은 종류의 버그를 재발견할 때 맥락이 살아난다. H2 slot은 Pre-flight와 sticky TOC가 의존하므로 이름을 바꾸지 말고 순서를 참고한다.

## 증상 (Symptom)

관측된 잘못된 동작을 한두 문장으로 기술한다. 재현 빈도와 영향 범위를 함께 밝힌다. 예: "결제 API 호출이 피크 트래픽 시간대에 약 15% 확률로 504 Gateway Timeout을 반환한다. QA 환경에서는 재현되지 않았고 프로덕션에서만 관찰됐다."

## 재현 (Reproduction)

상단에 한 문단으로 재현 시나리오를 서술한 뒤, 실제 실행 단계는 checklist로 나열한다. 이 섹션은 다른 엔지니어가 같은 상태를 만들 수 있게 하는 게 목적이므로 체크박스 포맷이 자연스럽다.

- [ ] 재현 단계 1 (명령어 / 요청 / 입력)
- [ ] 재현 단계 2
- [ ] 관측되는 에러 / 로그

재현률은 ` + "`N / N회`" + `로 수치화한다. 재현이 간헐적이라면 어느 조건에서 안 나는지도 함께 기록한다.

## 가설 (Hypotheses tried)

시도한 가설을 각각 한 문단 narrative로 기술한다. 예를 들어 "첫 번째 가설은 PG-A 측 일시 장애를 의심했다. ` + "`payments.retry_log`" + `에서 동 시간대 4xx/5xx 분포를 뽑아 보니 PG-A 쪽은 정상 응답 비율이 평소와 같았다. 단순한 upstream 장애는 아니라고 판단해 내부 worker pool 경합으로 시선을 옮겼다"처럼. "가설 1 — 한 줄 — 맞음/틀림" 식 bullet은 쓰지 않는다. 각 가설을 문장으로 풀면 나중에 같은 패턴의 버그가 다시 올라왔을 때 어디까지 검증된 상태였는지 즉시 파악할 수 있다.

## 원인 (Root cause)

실제 원인을 산문으로 기술한다. 어떤 조건에서 왜 이 증상이 나오는지 코드 라인·로그·측정치로 증명한다. 원인을 한 줄 요약으로 끝내지 말고 "왜 이 조건에서 특히 나타나는가"까지 풀어 적는다.

## 해결 (Resolution)

실제로 수정한 내용을 narrative로 기술한다. 어떤 파일의 어떤 부분을 어떻게 고쳤고 왜 그 방식이 원인을 해소하는지를 함께 쓴다. 관련 PR·commit 링크는 본 artifact의 ` + "`pins[]`" + `로 연결한다.

## 검증 (Verification)

- [ ] 재현 단계를 다시 돌려 증상이 사라지는지 확인
- [ ] regression test 추가 여부

검증 항목은 본질적으로 체크리스트이므로 bullet 포맷을 유지한다. 수동 확인이 필요한 환경 조건(트래픽·데이터·시점)이 있으면 checklist 아래 한 문단으로 덧붙인다.

## Open questions / 남은 미스터리

아직 풀리지 않은 부수적 질문을 한두 질문으로 제기한다. 예를 들어 "동일 worker pool 경합이 다른 API 경로에서도 잠재적으로 일어날 여지가 있다. 전수 조사는 본 Debug 범위를 넘어서므로 follow-up Analysis에 맡긴다" 같은 식으로 범위를 닫아 놓는다.

## 연관

이 fix가 이전 Debug artifact의 원인을 교정한다면 ` + "`supersede_of`" + ` edge로 lineage를 남기고, 관련 Decision·Feature는 ` + "`relates_to`" + `로 연결한다.
`

const templateDecisionBody = `<!-- validator: required_h2=TL,Context,Decision,Rationale,Alternatives considered,Consequences; required_keywords=decision,context -->
> **This artifact is a template.** Read before proposing a new
> ` + "`Decision`" + ` artifact. Template evolves via ` + "`update_of`" + `.

> **Register guide.** 이 템플릿을 참조해 Decision artifact를 작성할 때 각 H2 섹션 내부는 1-2 문단 narrative가 기본이다. 표와 bullet은 독립 항목이 셋 이상으로 나열되거나 본질적으로 checklist일 때만 보조 도구로 쓴다. ` + "`Alt A — 장점 / 단점 / 기각`" + ` 같은 축약 포맷은 피한다 — 축약은 artifact에서 대화 register로 역유입되어 추론 흐름을 훼손한다. 섹션 제목(H2)은 Pre-flight ` + "`checkRequiredH2`" + `와 Reader sticky TOC가 의존하므로 바꾸지 말고, 아래 slot 순서를 유지한다.

## TL;DR

결정의 요지를 최대 두 줄로 압축한다. 결론과 핵심 이유가 이것만 읽어도 전달되어야 하며, 세부 근거는 아래 섹션에서 풀어 적는다.

## Context

결정이 필요한 배경을 산문으로 기술한다. 현재 어떤 상황에서 어떤 제약이 걸려 있고 어떤 이해관계자가 있으며 "왜 지금 결정해야 하는가"가 분명해야 한다. 상황 서술은 한두 문단이면 충분하고, 여기서 나중 섹션(Rationale, Alternatives)에 들어갈 판단까지 선취하지 않는다. Context는 관찰과 제약만 담는 섹션이다.

## Decision

실제 결정 사항을 한두 문장으로 단호하게 선언한다. 여러 부수 조건이 붙으면 본문 여백이 아니라 Consequences 섹션에서 풀어 적는다. 예시 한 문장으로 쓰자면 "결제 실패 재시도는 최대 3회, 1초 / 3초 / 9초 exponential backoff로 한다"와 같은 형태다.

## Rationale

왜 이 결정이 채택됐는가를 narrative로 설명한다. 바로 아래 Alternatives considered 섹션에서 기각된 선택지를 하나씩 다루므로 여기서는 "선택된 경로가 어떤 판단 기준을 만족하는가"에 집중한다. 판단 기준이 셋 이상이면 각 기준을 한 문단으로 나누어 서술해도 되고, 기준 목록을 bullet으로 먼저 나열한 뒤 각 항목을 문단으로 받는 구조도 허용된다.

## Alternatives considered

기각된 선택지를 최소 둘 이상 기술한다. 각 alternative는 **별도 한 문단 narrative**로 작성한다. 예를 들어 "Alt A는 재시도 없이 사용자에게 즉시 실패를 표시하는 단순 경로다. 구현 비용은 거의 0이지만 결제사 일시 장애가 곧바로 사용자 이탈로 번져 ~%의 결제 취소율이 관측됐다. 현 트래픽 규모에서 허용 가능한 이탈률을 넘기에 기각한다"처럼. ` + "`Alt A(...) · 장점 · 단점 · 기각 이유`" + ` 같은 괄호 축약은 쓰지 않는다. 한 가지 alternative는 언제나 "아무것도 하지 않고 현상을 유지한다"가 유효하므로 명시적으로 포함해도 좋다.

## Consequences

이 결정의 단기·중기·장기 영향을 산문으로 기술한다. 기간 구분을 꼭 세 개로 쪼개지 않아도 된다. 예컨대 "단기적으로는 결제 worker의 재시도 큐 길이가 평균 3배로 늘어나고, 중장기적으로는 이 재시도 정책이 신규 PG 연동을 할 때마다 재현되는 기본 기대가 된다"처럼 한 문단에서 자연스럽게 시간대를 흐를 수 있다. 긍정과 부정 영향 모두 포함한다.

## Open questions

아직 결정하지 못한 세부 사항이나 이후 별도 Decision으로 분해할 지점이 있으면 여기에 짧게 둔다. 질문이 많다면 bullet 대신 한두 질문을 골라 narrative로 제기하고 "나머지는 follow-up Decision에서 다룬다"고 범위를 제한한다.

## 연관

영향을 받거나 이 결정을 근거로 움직이는 Feature / Task는 ` + "`relates_to`" + ` edge로 연결한다. 기존 Decision을 뒤집는다면 ` + "`supersede_of`" + ` 필드를 사용해 lineage를 남긴다. 관련 artifact 수가 많아지면 이 섹션에서 한 문단으로 그룹을 요약해도 된다.
`

const templateAnalysisBody = `<!-- validator: required_h2=TL;DR -->
> **This artifact is a template.** Read before proposing a new
> ` + "`Analysis`" + `. Template evolves via ` + "`update_of`" + `.

> **Register guide.** Analysis는 관찰과 측정, 그리고 그에 대한 해석을 남기는 artifact이므로 각 섹션은 1-2 문단 narrative로 쓰는 것이 기본이다. 표·bullet은 독립 항목이 셋 이상 나열될 때나 수치 집계 같은 구조화된 데이터를 담을 때에만 쓴다. "발견 1: ~" 식의 단일 라인 축약은 피하고, 각 발견을 한 문단 이상으로 풀어 적는다. H2 slot은 Pre-flight ` + "`checkRequiredH2`" + `와 Reader sticky TOC가 의존하므로 이름을 바꾸지 말고 순서만 참고한다.

## TL;DR

한 문장 결론이다. 바쁜 독자가 이것만 읽어도 Analysis의 요지가 전달되어야 한다. 판단까지 포함하려면 두 문장을 넘기지 않는다.

## 목적 / Scope

이 분석이 답하려는 질문을 한 문단으로 서술한다. 이어서 건드리는 영역과 건드리지 않는 영역을 산문으로 구분한다. 예를 들어 "본 분석은 지난 30일간 결제 worker의 retry 큐 길이 패턴을 다룬다. 원인 해석 수준까지는 들어가지만 결제사 선택 자체에 대한 의사결정은 후속 Decision에 맡긴다"처럼. 범위 구분을 bullet로 열거하기보다 문장으로 감싸는 편이 drift를 막는다.

## 조사 시점 (Investigation timestamp)

분석을 돌린 날짜와 사용한 도구·쿼리·코드 snapshot을 한 문단으로 적는다. "2026-04-23 오후에 Cloud SQL proxy로 프로덕션 DB에 붙어 ` + "`payments.retry_log`" + ` 테이블 최근 30일을 조회했다"처럼 재현 가능한 수준의 컨텍스트를 담는다.

## 발견 (Findings)

핵심 발견을 H3 하위 섹션으로 나눈다. 발견이 셋 이상이면 H3로 번호를 매기되 각 발견 내부는 narrative로 푼다. 한 발견에 여러 수치가 얽히는 경우에만 작은 표를 쓰고, 그 표 바로 아래에 표가 말하는 바를 산문으로 해석한다.

### 발견 1

### 발견 2

### 발견 3

## 평가 (Interpretation)

발견에 대한 저자의 판단을 산문으로 기술한다. 강점·약점·위험·기회를 네 bullet로 나누지 말고, 판단의 흐름 그대로 한 문단에 녹인다. 예를 들어 "재시도 큐 길이의 급증은 PG-A 측 일시 장애와 상관이 뚜렷한데, 현재는 backoff가 고정값이라 피크 시간대에 ~% 이탈이 발생한다"처럼 판단의 근거와 함의를 함께 제시한다.

## 재조회 방법 (Re-verification)

같은 조사를 나중에 반복하려면 어떤 명령 / 쿼리를 돌리면 되는지 한 문단으로 기술한다. 관련 script나 쿼리 파일이 있으면 ` + "`pins[]`" + `에 연결해 artifact에서 바로 열 수 있도록 한다.

## Open questions / 남은 미스터리

답을 내지 못한 부수적 질문이나 후속 분석이 필요한 지점을 질문 하나 혹은 두 개로 제기한다. 이 섹션이 길어지면 해당 질문들을 별도 Analysis로 분해하는 편이 낫다.

## 연관

이 분석에서 파생된 Decision / Task는 ` + "`relates_to`" + ` edge로 연결하고, 이 섹션에서는 "어느 축의 follow-up이 나왔는지" 한두 문장으로 요약한다.
`

const templateTaskBody = `<!-- validator: required_h2=목적,범위,코드 좌표,TODO,TC / DoD; required_keywords=acceptance -->
> **This artifact is a template.** Read before proposing a new
> ` + "`Task`" + `. Template evolves via ` + "`update_of`" + `.

> **Register guide.** Task는 "무엇을 왜 하는가"와 "어떤 상태가 되면 끝났는가"를 나누어 기록하는 artifact다. 목적·범위·분석 요약·코드 좌표 섹션은 narrative로 작성하고, TODO acceptance criteria와 TC/DoD의 자동 TC / 수동 QA는 본질적으로 체크리스트이므로 bullet 포맷을 유지한다. Bullet을 쓸 때도 각 항목이 독립적으로 "완료 판정이 가능한 조건"이어야 한다. H2 slot은 Pre-flight와 sticky TOC가 의존하므로 이름은 그대로 두고 순서만 참고한다.

## 목적 / Purpose

이 Task가 해결하는 문제를 한두 문단 narrative로 서술한다. 단순히 "X를 구현한다"가 아니라 "왜 지금 이 시점에 X가 필요하고 하지 않으면 어떤 비용이 쌓이는가"를 함께 적는다. 관련 Decision·Analysis가 있으면 "이 Task는 [` + "`decision-...`" + `](pindoc://decision-...)의 구현 축이다" 같은 문장으로 연결한다.

## 범위 / Scope

건드리는 영역과 건드리지 않는 영역을 각각 한 문단씩 산문으로 구분한다. 건드릴 영역에는 수정할 파일 / 모듈 / 에이전트 경로를 narrative 안에 녹여 언급하고, 건드리지 않을 영역에는 "이번 Task에서는 다루지 않고 이유는 이러하다"를 함께 적는다. 범위를 bullet로 나열하면 각 항목의 맥락이 사라지고 축약 register가 역유입되기 쉽다.

## 코드 좌표 (Code coordinates)

작업에 걸리는 파일·모듈·package 경로를 한두 문단으로 서술한다. 예: ` + "`internal/pindoc/mcp/tools/artifact_propose.go`" + ` 또는 ` + "`package internal/pindoc/mcp/tools`" + `. 정확한 path는 ` + "`pins[]`" + `로도 연결해 artifact에서 바로 열 수 있게 한다. 정책·vision Task처럼 코드 좌표가 본질적으로 없으면 ` + "`task_meta.code_coordinate_exempt=true`" + ` 또는 ` + "`artifact_meta.code_coordinate_exempt=true`" + `를 명시한다.

## 분석 요약 (Analysis summary)

작업 전에 조사한 결과를 한 문단으로 요약한다. 이미 관련 Analysis·Decision artifact가 있으면 ` + "`relates_to`" + ` edge로 연결하고, 여기서는 "어느 artifact에서 어느 결론을 빌려왔는지" 그리고 "남은 의문 중 이번 Task에서 풀 것과 미루는 것"을 언급한다.

## TODO — Acceptance criteria

- [ ] acceptance criterion 1 — "어떤 상태가 되면 이 항목이 true인가"가 문장 안에 분명히 드러나야 한다
- [ ] acceptance criterion 2
- [ ] …

각 항목은 완료 판정이 독립적으로 가능한 단위여야 한다. 애매한 "X가 좋아짐" 같은 표현은 다른 항목으로 쪼개거나 Analysis에 맡긴다.

## TC / DoD

### 자동 TC

- 추가·수정할 자동 테스트 이름
- 기존 통과 범위 유지

### 수동 QA

- 시나리오별 확인 단계

### 완료 기준 (DoD)

TODO가 모두 체크됐고, 자동 TC가 통과했고, 추가로 확인해야 할 비기능 요구(성능·로그·모니터링)가 있으면 한 문장으로 명시한다. DoD는 "Task가 끝났다고 선언할 수 있는 상태"를 narrative로 봉합하는 자리다.

## Open issues / 남은 질문

결정이 필요한 사소한 지점을 한두 질문으로 제기한다. 별도 Decision으로 올릴 만한 규모인지, 이 Task 안에서 해결할 만한지 판단을 함께 적는다.

## 연관

관련 Task·Decision·Debug는 ` + "`relates_to`" + `·` + "`blocks`" + `·` + "`implements`" + ` edge로 연결하고, 이 섹션에서는 "어느 축과 어떻게 얽혀 있는지"를 한두 문장으로 요약한다.
`

const templateSessionHandoffBody = `<!-- validator: required_h2=Current task,Completed work,Pending checks,Evidence,Next MCP calls; required_keywords=handoff,task,next -->
> **This artifact is a template.** Read before creating a session handoff.
> SessionHandoff is a convention, not a new artifact type; create it as a
> Flow artifact and link it to the active Task with ` + "`relates_to`" + `.

## Current task

Name the active Task slug, assignee, and why the session is being handed off.
Keep this to one short paragraph so a continuation agent can identify the
work without reading chat history.

## Completed work

Summarize what changed or what artifact decisions were published. Include
commit SHA, artifact slugs, or branch names when available, but keep durable
coordinates in ` + "`pins[]`" + ` or ` + "`relates_to`" + ` rather than only prose.

## Pending checks

List the checks that still need to run or the blockers that prevented
completion. Use checklist items only when each item can be independently
closed by the next agent.

- [ ] pending check or blocker

## Evidence

Name evidence artifacts, verification receipts, relevant pins, or manual QA
notes that the next agent should inspect before proceeding.

## Next MCP calls

State the exact next Pindoc MCP calls expected, such as
` + "`pindoc.context.for_task`" + `, ` + "`pindoc.artifact.read(view=\"continuation\")`" + `,
` + "`pindoc.task.queue`" + `, or ` + "`pindoc.task.done_check`" + `. This section is
the durable replacement for implicit chat-memory handoff.
`
