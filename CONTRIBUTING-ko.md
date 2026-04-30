# Pindoc 기여 안내

<p>
  <a href="./CONTRIBUTING.md"><img alt="English contributing guide" src="https://img.shields.io/badge/lang-English-6b7280.svg?style=flat-square"></a>
  <a href="./CONTRIBUTING-ko.md"><img alt="Korean contributing guide" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-2563eb.svg?style=flat-square"></a>
</p>

Pindoc 개선에 참여할 때 필요한 issue 작성, pull request 절차, CLA 서명,
검증 루프를 정리합니다.

## 시작 전

- **버그 리포트**: 재현 절차, 재현한 commit, 기대한 동작을 포함해 issue를 열어 주세요.
- **기능 제안**: 먼저 issue로 방향을 논의해 주세요. Pindoc은 "agent-only write surface", "code-pinned artifacts", "promote not store" 같은 명확한 철학을 갖고 있습니다.
- **문서 수정**: 사전 논의 없이 PR로 보내도 좋습니다. CLA는 코드뿐 아니라 모든 기여에 적용됩니다.

## 개발 환경

도구 요구사항은 [README 빠른 시작](README-ko.md#빠른-시작)을 참고하세요. 자주 쓰는 개발 명령은 `make help`에서 볼 수 있습니다.

PR 전 smoke loop:

```bash
go test ./...
cd web && pnpm install --frozen-lockfile && pnpm typecheck && pnpm test:unit
docker build -t pindoc-server:local .
```

Windows에서 로컬 C toolchain이 없으면 Docker로 Go test를 실행합니다.

```powershell
docker run --rm -v "${PWD}:/work" -w /work golang:1.25 go test ./...
```

## Contributor License Agreement

Pindoc은 Apache License 2.0으로 배포됩니다. 법적 일관성을 유지하고 향후
라이선스 질문에 대응하기 위해, 첫 merge 전에 Contributor License Agreement
(CLA) 서명을 요청합니다.

CLA는 다음 방식입니다.

- **1회 서명**: 이후 기여 전체에 적용됩니다.
- **자동화**: 첫 PR에서 bot이 서명 링크를 댓글로 남깁니다.
- **개인/법인 선택**: 개인 기여인지 회사 소속 기여인지에 맞는 양식을 선택합니다.

계약 전문은 `CLA.md`에 있습니다. 기여자의 저작권은 유지되고, 프로젝트는
Apache 2.0 조건에 맞춰 해당 기여를 사용할 수 있는 영구 라이선스를 받습니다.
CLA check를 통과하지 못한 PR은 merge하지 않습니다.

## Pull Request 절차

1. repo를 fork하고 `main`에서 feature branch를 만듭니다.
2. 변경은 작고 독립적인 commit으로 나눕니다. commit message는 무엇을 바꿨는지뿐 아니라 왜 바꿨는지를 설명합니다.
3. smoke loop를 실행하고 Go build, `go vet`, TypeScript typecheck가 통과하는지 확인합니다.
4. `main` 대상으로 PR을 열고 사용자에게 보이는 변화와 관련 issue를 적습니다.
5. 첫 PR이면 CLA bot이 올린 링크에서 서명합니다.
6. maintainer review를 받습니다. 이름, scope, Pindoc의 write-regulator 철학과 맞는지에 대한 코멘트를 기대하세요.
7. 승인과 CLA 서명이 끝나면 maintainer가 merge합니다.

## 스타일 메모

- Go: `gofmt`와 `go vet`이 통과해야 합니다. 에러는 `fmt.Errorf("context: %w", err)` 형태로 감쌉니다.
- TypeScript: strict mode를 유지하고 `pnpm exec tsc --noEmit`이 통과해야 합니다.
- Commit: 기존 Pindoc 스타일처럼 현재형의 짧은 첫 줄을 쓰고, 본문에는 이유를 적습니다.
- Docs: 사용자에게 보이는 flag, tool, HTTP endpoint를 추가하면 관련 `docs/*.md`도 같은 PR에서 갱신합니다.

## 범위 밖

Pindoc은 의도적으로 다음을 제공하지 않습니다.

- 사람이 직접 편집하는 wiki UI. 자세한 배경은 [Non-goals](docs/08-non-goals.md)를 참고하세요.
- raw session 또는 chat log 수집.
- sprint, burndown, time tracking까지 포함하는 완전한 task tracker.

이 방향을 되돌리는 PR은 설명과 함께 닫힐 수 있습니다. 좋은 아이디어라도 이
프로젝트에 맞는 아이디어가 아닐 수 있습니다.

## 행동 기준

기술적 의견 충돌은 자유롭게 해도 됩니다. 사람을 공격하지 마세요. Maintainer는
선을 넘은 thread를 닫거나 반복 위반자를 차단할 수 있습니다.
