package db

import (
	"strings"
	"testing"
)

func TestEmbeddedMigrationManifestIsValid(t *testing.T) {
	manifest, err := embeddedMigrationManifest()
	if err != nil {
		t.Fatalf("embeddedMigrationManifest() error = %v", err)
	}
	if len(manifest) == 0 {
		t.Fatal("embeddedMigrationManifest() returned no migrations")
	}
	for _, migration := range manifest {
		if len(migration.Checksum) != 64 {
			t.Fatalf("%s checksum len = %d, want 64", migration.ID, len(migration.Checksum))
		}
	}
	if got := migrationManifestChecksum(manifest); len(got) != 64 {
		t.Fatalf("manifest checksum len = %d, want 64", len(got))
	}
}

func TestMigrationChecksumNormalizesLineEndings(t *testing.T) {
	lf := []byte("-- +goose Up\nSELECT 1;\n-- +goose Down\nSELECT 1;\n")
	crlf := []byte(strings.ReplaceAll(string(lf), "\n", "\r\n"))
	if got, want := migrationChecksum(crlf), migrationChecksum(lf); got != want {
		t.Fatalf("CRLF checksum = %s, want LF checksum %s", got, want)
	}
}

func TestValidateMigrationVersionsRejectsNewDuplicate(t *testing.T) {
	manifest := []MigrationDefinition{
		{ID: "0069_first.sql", Version: "0069"},
		{ID: "0069_second.sql", Version: "0069"},
	}
	err := validateMigrationVersions(manifest)
	if err == nil || !strings.Contains(err.Error(), "MIGRATION_DUPLICATE_VERSION") {
		t.Fatalf("validateMigrationVersions() error = %v, want duplicate-version code", err)
	}
}

func TestValidateMigrationVersionsAllowsOnlyHistorical0018Pair(t *testing.T) {
	allowed := []MigrationDefinition{
		{ID: "0018_artifact_scope_edges.sql", Version: "0018"},
		{ID: "0018_backfill_task_assignee.sql", Version: "0018"},
	}
	if err := validateMigrationVersions(allowed); err != nil {
		t.Fatalf("historical 0018 pair rejected: %v", err)
	}
	withThird := append(allowed, MigrationDefinition{ID: "0018_new.sql", Version: "0018"})
	if err := validateMigrationVersions(withThird); err == nil {
		t.Fatal("historical allowlist accepted a third 0018 migration")
	}
}

func TestCompareMigrationStateReportsUnknownMismatchAndPending(t *testing.T) {
	manifest := []MigrationDefinition{
		{ID: "0001_init.sql", Version: "0001", Checksum: "aaa"},
		{ID: "0002_next.sql", Version: "0002", Checksum: "bbb"},
	}
	applied := []AppliedMigration{
		{ID: "0001_init.sql", Checksum: "changed"},
		{ID: "0009_branch_only.sql", Checksum: "ghost"},
	}
	health := compareMigrationState(manifest, applied, true, true)
	if health.Healthy {
		t.Fatal("drifted state reported healthy")
	}
	if health.PendingCount != 1 {
		t.Fatalf("pending_count = %d, want 1", health.PendingCount)
	}
	want := map[string]bool{
		MigrationChecksumMismatchCode: false,
		MigrationUnknownAppliedCode:   false,
		MigrationPendingCode:          false,
	}
	for _, issue := range health.Issues {
		if _, ok := want[issue.Code]; ok {
			want[issue.Code] = true
		}
	}
	for code, seen := range want {
		if !seen {
			t.Fatalf("issues missing %s: %+v", code, health.Issues)
		}
	}
}
