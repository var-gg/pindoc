# Data Integrity Operations

This guide owns Pindoc's operational contract for migration integrity,
artifact-index recovery, build provenance, and disposable test-project
isolation. The MCP tool spec references the shared response types here instead
of duplicating them across every write tool.

## Migration integrity

Every embedded SQL migration has an ID, numeric version, and SHA-256 checksum.
The checksum covers the complete migration file after CRLF normalization,
including both `Up` and `Down` blocks. New migrations must use a unique numeric
version. The two historical `0018` files are the only allow-listed exception.

`schema_migrations.checksum` records the checksum used by the binary. Startup
backfills blank checksums for known legacy rows, rejects a known row whose
stored checksum differs, and applies missing migrations under one Postgres
advisory lock. It never removes or silently accepts a migration that exists
only in the database.

The first checksum-aware upgrade cannot prove which bytes were used for a
legacy row whose checksum was never recorded; it establishes the baseline from
the current binary. Every later edit to an applied migration is detectable.
Treat historical migration files as immutable and add a new migration for any
correction.

Run the read-only doctor before reconciliation and after every rollout:

```bash
pindoc-admin schema doctor
pindoc-admin schema doctor --json
```

The command exits non-zero when it reports any of these issue codes:

- `MIGRATION_LEDGER_MISSING`: the migration ledger does not exist.
- `MIGRATION_CHECKSUM_COLUMN_MISSING`: the checksum-aware migrator has not run.
- `MIGRATION_UNKNOWN_APPLIED`: the database contains an ID absent from this binary.
- `MIGRATION_CHECKSUM_MISSING`: a known applied row has no checksum.
- `MIGRATION_CHECKSUM_MISMATCH`: an applied file differs from its recorded checksum.
- `MIGRATION_PENDING`: the binary contains a migration not yet applied to the database.

An unknown applied migration is evidence, not cleanup permission. Back up the
database, identify the binary or branch that created it, compare the actual
schema and data, and write an explicit forward reconciliation migration. Do
not delete the ledger row merely to make the doctor green.

The same result is exposed as `schema_health` in
`pindoc.runtime.status`:

```typescript
type MigrationIssueCode =
  | "MIGRATION_LEDGER_MISSING"
  | "MIGRATION_CHECKSUM_COLUMN_MISSING"
  | "MIGRATION_UNKNOWN_APPLIED"
  | "MIGRATION_CHECKSUM_MISSING"
  | "MIGRATION_CHECKSUM_MISMATCH"
  | "MIGRATION_PENDING";

interface MigrationHealth {
  healthy: boolean;
  ledger_present: boolean;
  checksum_column_present: boolean;
  manifest_checksum: string;
  known_count: number;
  applied_count: number;
  pending_count: number;
  issues: Array<{
    code: MigrationIssueCode;
    migration_id?: string;
    expected_checksum?: string;
    actual_checksum?: string;
    detail: string;
  }>;
}
```

## Artifact index state

`artifact_index_state` is the durable provenance for the last index attempt.
It records the artifact revision, title/body hashes, provider identity and
dimension, attempt count, timestamps, and the latest error. Existing artifacts
are backfilled as `unknown` because legacy chunks cannot prove which content or
model produced them.

Write paths prepare every title and body embedding before deleting any old
chunk. A provider failure commits the artifact revision with a retryable
`failed` state while preserving the last known-good chunks. A chunk-storage or
index-state database failure rolls back the entire artifact transaction.

The stored states are `unknown`, `indexed`, and `failed`. `stale` is an
effective state derived when current title/body hashes differ from an indexed
row. The re-embed command also treats provider name, model ID, or dimension
drift as stale.

```typescript
interface ArtifactIndexState {
  revision_number: number;
  body_hash: string;
  title_hash: string;
  model_name?: string;
  model_id?: string;
  model_dim?: number;
  status: "unknown" | "indexed" | "failed";
  attempt_count: number;
  last_error?: string;
  retryable?: boolean;
}
```

Preview and repair only rows that need work:

```bash
pindoc-reembed -state needs-refresh -dry-run
pindoc-reembed -state needs-refresh
```

`needs-refresh` includes `unknown`, `failed`, content-stale, and provider-drift
rows. More specific selectors are available through
`-state failed|stale|unknown|indexed|all`. Run the command with the same real
embedding provider intended for normal search; do not repair production data
with the stub provider.

## Build provenance

Release and Docker builds inject both a version and commit:

```bash
docker build \
  --build-arg VERSION=0.0.1 \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  -t pindoc-server:0.0.1 .
```

`pindoc.runtime.status` returns `server_commit` and
`server_commit_source`. The source is `ldflags` for an injected build,
`go_build_info` for a VCS-stamped local binary, or `unavailable`. Use a value
such as `<sha>-dirty` for an image built from an uncommitted working tree; do
not present the base commit as the exact source.

## Fixture-project isolation

Disposable integration projects are marked by `projects.reader_hidden`.
Reader and Task Flow consume that column through the shared project visibility
query. Runtime code must not infer fixture intent from slug prefixes.

Migration `0070_projects_reader_hidden.sql` performs a one-time compatibility
backfill for fixture families created by earlier released tests. New harnesses
must set `CreateProjectInput.ReaderHidden=true` explicitly. The public MCP and
REST project-create inputs intentionally do not expose this internal flag.

Normal Reader queries exclude hidden projects. An explicit operator query may
include them only when the caller resolves as a project owner. Integration
tests must use a disposable database and must never point at a personal or
production Pindoc database.

## Rollout checklist

1. Back up the target database and record the currently running image digest.
2. Build with explicit `VERSION` and `COMMIT` values.
3. Boot the image against a disposable fresh database and require health plus a clean `schema doctor --json` result.
4. Run the target rollout so the migrator applies forward migrations.
5. Require `pindoc.runtime.status.schema_health.healthy=true` and confirm the reported commit.
6. Run `pindoc-reembed -state needs-refresh -dry-run`, then repair with the intended production provider.
7. Re-run the doctor and retain its JSON output with the release evidence.

If step 4 reveals an unknown migration or checksum mismatch, stop automatic
cleanup. Reconcile the lineage explicitly before claiming the rollout healthy.
