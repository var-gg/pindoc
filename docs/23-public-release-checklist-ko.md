# 공개 릴리스 체크리스트

<p>
  <a href="./23-public-release-checklist.md"><img alt="English public release checklist" src="https://img.shields.io/badge/lang-English-6b7280.svg?style=flat-square"></a>
  <a href="./23-public-release-checklist-ko.md"><img alt="Korean public release checklist" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-2563eb.svg?style=flat-square"></a>
</p>

이 문서는 repository를 공개하거나 README에서 public demo를 연결하기 전의 최소
신뢰 게이트입니다.

## Repository

- `README.md`는 영어 기본 landing page입니다.
- `README-ko.md`는 첫 화면에서 연결됩니다.
- `docs/README.md`와 `docs/README-ko.md`는 언어별 documentation hub입니다.
- `docs/26-system-requirements.md`와 `docs/26-system-requirements-ko.md`는 기본 embedding cache와 권장 host profile을 설명합니다.
- `docs/24-record-worthy-artifact-policy-ko.md`는 무엇을 durable memory로 남기고 무엇을 제외할지 설명합니다.
- `LICENSE`는 Apache 2.0이고 README/license 표현이 일치합니다.
- `SECURITY.md`와 `SECURITY-ko.md`는 loopback trust, external exposure, read-only demo 제약을 설명합니다.
- `CONTRIBUTING.md`와 `CONTRIBUTING-ko.md`는 public contribution path를 설명합니다.
- README 또는 first-run docs가 stale M1 scaffold나 stub-default behavior를 현재 상태처럼 설명하지 않습니다.

## CI

필수 check:

```bash
go test ./...
cd web && pnpm typecheck && pnpm test:unit && pnpm build
docker build -t pindoc-server:local .
git diff --check
```

Windows에서 로컬 C toolchain이 없으면 Docker로 Go test를 실행합니다.

```powershell
docker run --rm -v "${PWD}:/work" -w /work golang:1.25 go test ./...
```

## Docker Quick Start

Fresh clone smoke:

```bash
docker compose up -d --build
curl -fsS http://127.0.0.1:5830/health
curl -fsS http://127.0.0.1:5830/api/config
```

Browser manual check:

```text
http://localhost:5830/
```

기대 결과: Reader 또는 first-project onboarding이 stack trace 없이 열립니다.

기본 경로는 bundled EmbeddingGemma provider로 의미 검색이 켜진 상태여야 합니다.
stub embedding default를 문서화하거나 배포하지 않습니다.

## Public Demo

README에 public demo URL을 추가하기 전:

- DNS와 TLS가 live입니다.
- `/mcp`가 public internet에서 차단됩니다.
- public non-`GET` method가 차단됩니다.
- 모든 참조 repo가 public이고 source browsing이 명시 승인된 경우가 아니라면 git preview route가 차단됩니다.
- Demo data가 [공개 데모 운영안](22-public-demo-ko.md)의 scrub checklist를 통과합니다.
- 첫 visitor path가 [공개 데모 story path](25-public-demo-story-path-ko.md)에 정리되어 있습니다.
- 이후 launch decision이 바꾸지 않는 한 기본 public entry point는 `/p/pindoc/today`입니다.
- curated screenshot 1개가 `docs/assets/` 아래 commit되어 있습니다.

## 남은 사용자 결정

- 최종 public demo domain,
- demo에 노출할 최종 project 목록,
- git blob/diff preview를 public으로 열지 차단할지,
- announcement를 위해 GIF/MP4를 제작할지.
