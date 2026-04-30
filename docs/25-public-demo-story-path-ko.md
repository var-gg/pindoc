# 공개 데모 Story Path

<p>
  <a href="./25-public-demo-story-path.md"><img alt="English public demo story path" src="https://img.shields.io/badge/lang-English-6b7280.svg?style=flat-square"></a>
  <a href="./25-public-demo-story-path-ko.md"><img alt="Korean public demo story path" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-2563eb.svg?style=flat-square"></a>
</p>

첫 공개 데모는 Pindoc이 협업형 AI 작업에 왜 필요한지 보여줘야 합니다. 에이전트가
유효한 분석을 발견하고, Pindoc이 그것을 code-pinned team memory로 만들며, 다음
팀원이나 에이전트가 원래 채팅을 읽지 않고도 근거를 따라갈 수 있어야 합니다.

## Launch Route

기본 entry:

```text
/p/pindoc/today
```

권장 visitor flow:

1. Today를 엽니다.
2. public launch, CI, README, policy 작업과 연결된 Change Group을 고릅니다.
3. Task artifact를 엽니다.
4. 관련 Analysis, Debug, Decision artifact를 따라갑니다.
5. commit/file pin 또는 release docs로 이동합니다.
6. Today나 Graph로 돌아와 이 기억이 flat wiki page가 아니라 연결된 project memory임을 확인합니다.

## Candidate Stories

| Story | Start | Follow | Shows |
| --- | --- | --- | --- |
| 다국어 OSS landing | Today 또는 `task-readme-multilingual-landing` | `README.md`, `README-ko.md`, docs hub commits | 에이전트가 남긴 launch docs, acceptance closeout, 다국어 navigation. |
| CI trust gate | `task-public-release-trust-gates` | `.github/workflows/ci.yml`, 성공한 CI run, release checklist | 검증 evidence, release readiness, code/config pin. |
| 협업형 포지셔닝 | `gpt-pro-strategic-review-intake-collaborative-ai-insight-memory` | `task-collaborative-positioning-readme-refresh`, README changes | 외부 리뷰를 제품 판단과 public copy로 걸러내는 흐름. |
| Record-worthy memory policy | `task-record-worthy-artifact-policy` | `docs/24-record-worthy-artifact-policy-ko.md` | Pindoc이 raw chat log가 아니라 curated team knowledge를 남긴다는 원칙. |
| Read-only demo hardening | `task-readonly-demo-public-site` | `docs/22-public-demo-ko.md`, proxy/scrub checklist | Write surface를 막은 실제 dogfood 공개 데모 구조. |

## Public Safety Check

첫 launch path는 `pindoc` project artifact만 사용합니다. Private customer work,
private repository, 미공개 domain, local home path, private deployment detail은 첫
screenshot에 쓰지 않습니다.

Public link를 걸기 전:

- 선택한 story의 artifact body를 샘플링합니다.
- 보이는 pin과 commit summary를 확인합니다.
- 참조 repo가 public이고 명시 승인된 경우가 아니라면 git blob/diff preview가 차단되는지 확인합니다.
- `/mcp`와 mutation route가 차단되는지 확인합니다.
- screenshot에 private path, email, domain, internal hostname이 보이지 않는지 확인합니다.

## README Caption Candidates

Option A:

```text
Real Pindoc artifacts from the Pindoc project itself. Agents turn useful work
into code-pinned team memory; humans review, discuss, and steer it.
```

Option B:

```text
The public demo is real dogfood: launch tasks, strategy analyses, policies, and
CI fixes written through Pindoc and linked back to commits and docs.
```

## Screenshot Priority

1. Public launch 또는 policy Change Group이 보이는 Today view.
2. Acceptance와 related artifacts가 보이는 Task article.
3. 걸러낸 판단이 보이는 관련 Analysis 또는 Decision article.
4. Pin/evidence panel 또는 연결된 docs/commit evidence.
5. Article path보다 관계가 더 잘 보일 때만 Graph view.
