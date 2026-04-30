# Record-worthy Artifact 정책

<p>
  <a href="./24-record-worthy-artifact-policy.md"><img alt="English record-worthy artifact policy" src="https://img.shields.io/badge/lang-English-6b7280.svg?style=flat-square"></a>
  <a href="./24-record-worthy-artifact-policy-ko.md"><img alt="Korean record-worthy artifact policy" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-2563eb.svg?style=flat-square"></a>
</p>

Pindoc은 raw chat archive가 아닙니다. 미래 팀원과 미래 에이전트가 재사용할 수
있는 AI-assisted work의 핵심만 기록합니다.

Artifact를 만들거나 갱신하기 전 기준은 다음입니다.

```text
나중에 누군가가 프로젝트를 이해하고, 토론하고, 검증하고, 이어서 작업하는 데
도움이 될 때만 기록한다.
```

## 기록할 것

현재 대화를 넘어 프로젝트 가치가 있을 때 다음을 기록합니다.

- rationale, alternative, consequence, ownership impact가 있는 결정,
- 비자명한 발견, tradeoff, 사용자 요구, 시스템 동작을 설명하는 분석,
- 실패한 시도, root cause, fix direction, reproduction detail이 있는 디버그 경로,
- acceptance, verification, commit/file evidence가 있는 task closeout,
- 미래 작업이 신뢰하거나 재실행해야 하는 test/QA evidence,
- 바로 수정할 수는 없지만 팀 공론화가 필요한 책임소재/운영 관행 관련 이슈,
- 중복 문서, 반복 조사, 충돌하는 결정을 줄이는 데 필요한 문맥.

## 기록하지 않을 것

다음만으로는 artifact를 만들지 않습니다.

- raw chat transcript,
- 임시 사고흐름,
- 단순 진행상황 나열,
- 미래 문맥 가치가 없는 기계적 typo/formatting 수정,
- 팀 메모리가 되어서는 안 되는 private conversation,
- 기존 artifact의 상태, 증거, 해석을 바꾸지 않는 반복 요약,
- code comment, test, commit message만으로 충분한 구현 세부사항.

## Type별 예시

| Type | Record-worthy example | Non-example |
| --- | --- | --- |
| Task | acceptance check, verification note, commit pin이 있는 다단계 변경. | commit만으로 설명되는 한 줄 typo fix. |
| Decision | rationale과 consequence가 있는 제품/아키텍처 선택. | 지속적 팀 영향이 없는 개인 선호. |
| Analysis | 사용자 문제, launch 전략, data model, workflow를 새롭게 해석한 발견. | 새 판단이 없는 일반 session summary. |
| Debug | reproduction, 실패한 시도, root cause, 검증된 fix direction. | 조사 없는 error message 복사. |
| TC | 미래 팀원/에이전트가 신뢰하거나 재실행할 test/QA 결과. | 해석 없는 command log. |
| Glossary | 반복되는 용어 혼선을 줄이는 경계 정의. | 모두가 이미 같은 뜻으로 쓰는 일반 사전 항목. |

## 새로 만들기보다 갱신할 때

다음이면 새 artifact보다 update, supersede, relate-to를 우선합니다.

- 같은 결정이나 분석이 이미 있습니다.
- 새 정보가 기존 Task 상태를 바꿉니다.
- 새 evidence가 이전 claim을 확인하거나 무효화합니다.
- 작업이 기존 story path나 launch track에 속합니다.
- 새 가치가 commit, test result, wording clarification뿐입니다.

다음이면 새 artifact를 만듭니다.

- 주제가 실질적으로 새롭습니다.
- 별도 lifecycle 또는 review state가 필요합니다.
- 기존 artifact에 넣으면 혼란스러워집니다.
- acceptance criteria를 담을 새 Task가 필요합니다.

## Public Summary

Pindoc은 모든 메시지를 저장하지 않고 curated project memory를 남깁니다. 에이전트는
미래 팀원이나 미래 에이전트가 다시 쓸 가치가 있는 결정, 분석, 디버그 경로,
task closeout, 검증 근거를 기록합니다. 임시 생각, raw chat log, 기존 memory의
반복 요약은 새 artifact가 되면 안 됩니다.

## Dogfood Sample Check

Harness에 반영하기 전 다음 샘플에 이 정책을 적용합니다.

- `task-readme-multilingual-landing` - public docs와 launch acceptance를 담은 record-worthy Task.
- `gpt-pro-strategic-review-intake-collaborative-ai-insight-memory` - 외부 리뷰를 채택/기각 판단으로 걸러낸 record-worthy Analysis.
- `task-public-release-trust-gates` - CI/security evidence를 담은 release Task.
- `decision-artifact-format-leak-mitigation` - harness behavior와 사용자 응답 품질에 영향을 주는 Decision.
- README badge 색상만 바꾼 한 줄 수정 - 보통 새 artifact 대상이 아니며 기존 README Task나 commit history로 충분합니다.
