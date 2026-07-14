# 데이터 무결성 운영

이 문서는 Pindoc의 migration 무결성, artifact index 복구, build provenance,
일회성 테스트 프로젝트 격리 계약을 설명합니다. MCP tool spec은 write tool마다
같은 응답 구조를 반복하지 않고 이 문서의 공통 타입을 참조합니다.

## Migration 무결성

binary에 포함된 SQL migration은 ID, 숫자 version, SHA-256 checksum을 갖습니다.
Checksum은 CRLF를 정규화한 전체 파일을 대상으로 하므로 `Up`과 `Down` block이
모두 검토 계약에 포함됩니다. 새 migration의 숫자 version은 유일해야 하며,
과거에 배포된 `0018` 두 파일만 명시적으로 허용한 예외입니다.

`schema_migrations.checksum`은 해당 migration에 사용한 checksum을 기록합니다.
서버 시작 시 알려진 legacy row의 빈 checksum을 채우고, 저장값과 현재 binary가
다르면 migration을 거부하며, Postgres advisory lock 안에서 누락 migration을
적용합니다. DB에만 존재하는 migration을 삭제하거나 정상으로 간주하지 않습니다.

Checksum을 처음 도입하는 upgrade는 과거에 checksum이 없던 row가 실제로 어떤
bytes로 적용됐는지 역으로 증명할 수 없습니다. 현재 binary를 기준선으로 삼은
뒤부터는 변경을 검출합니다. 적용된 migration 파일은 수정하지 말고, 보정은 새
migration으로 작성해야 합니다.

조정 전과 배포 후에 read-only doctor를 실행합니다.

```bash
pindoc-admin schema doctor
pindoc-admin schema doctor --json
```

다음 issue가 하나라도 있으면 command는 non-zero로 종료합니다.

- `MIGRATION_LEDGER_MISSING`: migration ledger가 없습니다.
- `MIGRATION_CHECKSUM_COLUMN_MISSING`: checksum-aware migrator가 아직 실행되지 않았습니다.
- `MIGRATION_UNKNOWN_APPLIED`: DB에는 있지만 현재 binary에는 없는 ID입니다.
- `MIGRATION_CHECKSUM_MISSING`: 알려진 applied row의 checksum이 비어 있습니다.
- `MIGRATION_CHECKSUM_MISMATCH`: 적용 파일과 기록한 checksum이 다릅니다.
- `MIGRATION_PENDING`: binary의 migration이 아직 DB에 적용되지 않았습니다.

Unknown migration은 자동 삭제 허가가 아니라 조사 증거입니다. DB를 백업하고,
그 row를 만든 binary나 branch를 찾고, 실제 schema와 데이터를 비교한 다음,
명시적인 forward reconciliation migration을 작성합니다. Doctor를 green으로
만들기 위해 ledger row만 지우면 안 됩니다.

같은 결과는 `pindoc.runtime.status.schema_health`로도 노출됩니다. 공통
`MigrationHealth` 타입은 [영문 운영 계약](27-data-integrity-operations.md#migration-integrity)에
정의되어 있습니다.

## Artifact index 상태

`artifact_index_state`는 마지막 index 시도의 durable provenance입니다. Artifact
revision, title/body hash, provider identity와 dimension, attempt count, timestamp,
최근 오류를 기록합니다. 기존 artifact는 과거 chunk가 어떤 content와 model에서
나왔는지 증명할 수 없으므로 `unknown`으로 backfill합니다.

Write path는 기존 chunk를 삭제하기 전에 title과 body embedding을 모두
준비합니다. Provider 실패 시 artifact revision과 retryable `failed` 상태는
commit하지만 마지막 정상 chunk는 유지합니다. Chunk 저장이나 index-state DB
write가 실패하면 artifact transaction 전체를 rollback합니다.

저장 상태는 `unknown`, `indexed`, `failed`입니다. `stale`은 현재 title/body hash와
indexed row가 다를 때 계산되는 effective 상태입니다. Re-embed command는 provider
name, model ID, dimension drift도 stale로 취급합니다. 공통
`ArtifactIndexState` 타입은 [영문 운영 계약](27-data-integrity-operations.md#artifact-index-state)에
정의되어 있습니다.

복구 대상을 먼저 확인한 뒤 필요한 row만 처리합니다.

```bash
pindoc-reembed -state needs-refresh -dry-run
pindoc-reembed -state needs-refresh
```

`needs-refresh`는 `unknown`, `failed`, content-stale, provider-drift row를 포함합니다.
`-state failed|stale|unknown|indexed|all` selector도 사용할 수 있습니다. 정상 검색에
사용할 실제 provider로 실행해야 하며 production data를 stub provider로 복구하면
안 됩니다.

## Build provenance

Release와 Docker build에는 version과 commit을 명시적으로 주입합니다.

```bash
docker build \
  --build-arg VERSION=0.0.1 \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  -t pindoc-server:0.0.1 .
```

`pindoc.runtime.status`는 `server_commit`과 `server_commit_source`를 반환합니다.
주입한 build는 `ldflags`, VCS stamp가 있는 local binary는 `go_build_info`, 둘 다
없으면 `unavailable`입니다. Commit하지 않은 working tree로 만든 이미지는
`<sha>-dirty`처럼 표시해 base commit을 정확한 source인 것처럼 보이지 않게 합니다.

## Fixture 프로젝트 격리

일회성 integration project는 `projects.reader_hidden`으로 표시합니다. Reader와
Task Flow는 공통 project visibility query를 통해 이 column을 사용하며, runtime
code는 slug prefix로 fixture 여부를 추론하지 않습니다.

Migration `0070_projects_reader_hidden.sql`은 과거 배포 테스트가 만든 fixture family를
한 번만 backfill합니다. 새 harness는 `CreateProjectInput.ReaderHidden=true`를
명시해야 합니다. Public MCP와 REST project-create input에는 이 internal flag를
노출하지 않습니다.

일반 Reader query는 hidden project를 제외합니다. 명시적인 operator query도 해당
project owner로 확인된 caller만 hidden row를 볼 수 있습니다. Integration test는
반드시 일회성 DB에서 실행하고 개인 DB나 production Pindoc DB를 향하면 안 됩니다.

## Rollout 체크리스트

1. 대상 DB를 백업하고 현재 실행 image digest를 기록합니다.
2. 명시적인 `VERSION`, `COMMIT` 값으로 build합니다.
3. 일회성 fresh DB에서 image를 띄우고 health와 clean `schema doctor --json`을 확인합니다.
4. 대상 rollout으로 forward migration을 적용합니다.
5. `pindoc.runtime.status.schema_health.healthy=true`와 보고된 commit을 확인합니다.
6. `pindoc-reembed -state needs-refresh -dry-run` 후 production provider로 복구합니다.
7. Doctor를 다시 실행하고 JSON 결과를 release evidence로 보관합니다.

4단계에서 unknown migration이나 checksum mismatch가 보이면 자동 cleanup을
중단합니다. Lineage를 명시적으로 조정하기 전에는 rollout이 건강하다고 선언하지
않습니다.
