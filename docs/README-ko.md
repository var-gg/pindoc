# Pindoc 문서

<p>
  <a href="./README.md"><img alt="English documentation" src="https://img.shields.io/badge/lang-English-6b7280.svg?style=flat-square"></a>
  <a href="./README-ko.md"><img alt="Korean documentation" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-2563eb.svg?style=flat-square"></a>
</p>

이 허브는 repo 문서를 공개 진입점, 운영 문서, 설계 원본, 유지보수 히스토리로
나눕니다. 공개 문서는 먼저 협업 흐름을 설명해야 합니다. 에이전트는 유용한
분석과 작업 결과를 code-pinned project memory로 남기고, 사람은 읽고 토론하고
무엇을 팀 지식으로 남길지 결정합니다. 깊은 설계 문서는 작성 당시의 원본 언어를
유지합니다.

## 먼저 볼 문서

- [README](../README-ko.md) - 제품 개요, 빠른 시작, MCP 클라이언트 연결.
- [공개 데모 운영안](22-public-demo-ko.md) - read-only demo 구조, proxy 정책, scrub 체크리스트, screenshot 규칙.
- [공개 데모 story path](25-public-demo-story-path-ko.md) - 실제 dogfood artifact로 협업형 AI 통찰이 팀 지식이 되는 경로를 보여주는 큐레이션.
- [Record-worthy artifact 정책](24-record-worthy-artifact-policy-ko.md) - 무엇을 기록하고, 무엇을 기록하지 않으며, 언제 새 문서보다 update/supersede를 우선할지 정합니다.
- [공개 릴리스 체크리스트](23-public-release-checklist-ko.md) - repo 공개나 demo 링크 추가 전 최소 신뢰 게이트.
- [보안 정책](../SECURITY-ko.md) - 로컬 신뢰 모델, 외부 노출, 취약점 제보.
- [기여 안내](../CONTRIBUTING-ko.md) - issue, PR, smoke loop, CLA.

## 에이전트 워크플로우와 MCP

- [MCP Tools Spec](10-mcp-tools-spec.md) - MCP tool 계약과 구현 상태.
- [PINDOC.md Harness Spec](09-pindoc-md-spec.md) - workspace harness, task protocol, agent startup 규칙.
- [Revision Shapes Spec](18-revision-shapes-spec.md) - typed artifact revision 경로.
- [Locale Contribution Guide](CONTRIBUTING_LOCALE.md) - title-quality locale 추가 방법.

## 설계 원본 노트

- [Vision](00-vision.md)
- [Problem](01-problem.md)
- [Concepts](02-concepts.md)
- [Architecture](03-architecture.md)
- [Data Model](04-data-model.md)
- [Mechanisms](05-mechanisms.md)
- [UI Flows](06-ui-flows.md)
- [Roadmap](07-roadmap.md)
- [Non-goals](08-non-goals.md)

## 유지보수 히스토리

- [Decisions](decisions.md)
- [Glossary](glossary.md)
- [M1 Implementation Plan](12-m1-implementation-plan.md)
- [Peer Review Response](14-peer-review-response.md)
- [Area Taxonomy](19-area-taxonomy.md)
- [Sub-area Promotion Policy](20-sub-area-promotion-policy.md)
- [Cross-cutting Admission Rule](21-cross-cutting-admission-rule.md)
- [Session Handoff 2026-04-22](13-session-handoff-2026-04-22.md)
- [Dogfood Session Handoff](15-session-handoff-dogfood.md)
- [Tier 2 Preflight](16-tier2-preflight.md)
- [Post-Tier-2 Handoff](17-session-handoff-post-tier2.md)

## 자산

- [README와 런치 자산](assets/README.md)
