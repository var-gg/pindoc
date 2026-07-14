package db

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MigrationLedgerMissingCode         = "MIGRATION_LEDGER_MISSING"
	MigrationChecksumColumnMissingCode = "MIGRATION_CHECKSUM_COLUMN_MISSING"
	MigrationUnknownAppliedCode        = "MIGRATION_UNKNOWN_APPLIED"
	MigrationChecksumMissingCode       = "MIGRATION_CHECKSUM_MISSING"
	MigrationChecksumMismatchCode      = "MIGRATION_CHECKSUM_MISMATCH"
	MigrationPendingCode               = "MIGRATION_PENDING"
)

type AppliedMigration struct {
	ID       string
	Checksum string
}

type MigrationIssue struct {
	Code             string `json:"code"`
	MigrationID      string `json:"migration_id,omitempty"`
	ExpectedChecksum string `json:"expected_checksum,omitempty"`
	ActualChecksum   string `json:"actual_checksum,omitempty"`
	Detail           string `json:"detail"`
}

// MigrationHealth is a read-only comparison between the binary's embedded
// manifest and the database ledger. It never drops, renames, or accepts an
// unknown migration on the operator's behalf.
type MigrationHealth struct {
	Healthy               bool             `json:"healthy"`
	LedgerPresent         bool             `json:"ledger_present"`
	ChecksumColumnPresent bool             `json:"checksum_column_present"`
	ManifestChecksum      string           `json:"manifest_checksum"`
	KnownCount            int              `json:"known_count"`
	AppliedCount          int              `json:"applied_count"`
	PendingCount          int              `json:"pending_count"`
	Issues                []MigrationIssue `json:"issues"`
}

func (h MigrationHealth) IssueCodes() []string {
	seen := make(map[string]struct{}, len(h.Issues))
	out := make([]string, 0, len(h.Issues))
	for _, issue := range h.Issues {
		if _, ok := seen[issue.Code]; ok {
			continue
		}
		seen[issue.Code] = struct{}{}
		out = append(out, issue.Code)
	}
	sort.Strings(out)
	return out
}

func InspectMigrationHealth(ctx context.Context, pool *pgxpool.Pool) (MigrationHealth, error) {
	manifest, err := embeddedMigrationManifest()
	if err != nil {
		return MigrationHealth{}, err
	}

	var ledgerPresent bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&ledgerPresent); err != nil {
		return MigrationHealth{}, fmt.Errorf("inspect migration ledger: %w", err)
	}
	if !ledgerPresent {
		return compareMigrationState(manifest, nil, false, false), nil
	}

	var checksumColumnPresent bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = 'schema_migrations'
			  AND column_name = 'checksum'
		)
	`).Scan(&checksumColumnPresent); err != nil {
		return MigrationHealth{}, fmt.Errorf("inspect migration checksum column: %w", err)
	}

	query := `SELECT id, '' FROM schema_migrations ORDER BY id`
	if checksumColumnPresent {
		query = `SELECT id, COALESCE(checksum, '') FROM schema_migrations ORDER BY id`
	}
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return MigrationHealth{}, fmt.Errorf("read migration ledger: %w", err)
	}
	defer rows.Close()

	applied := make([]AppliedMigration, 0, len(manifest))
	for rows.Next() {
		var migration AppliedMigration
		if err := rows.Scan(&migration.ID, &migration.Checksum); err != nil {
			return MigrationHealth{}, fmt.Errorf("scan migration ledger: %w", err)
		}
		applied = append(applied, migration)
	}
	if err := rows.Err(); err != nil {
		return MigrationHealth{}, fmt.Errorf("iterate migration ledger: %w", err)
	}
	return compareMigrationState(manifest, applied, true, checksumColumnPresent), nil
}

func compareMigrationState(manifest []MigrationDefinition, applied []AppliedMigration, ledgerPresent, checksumColumnPresent bool) MigrationHealth {
	health := MigrationHealth{
		LedgerPresent:         ledgerPresent,
		ChecksumColumnPresent: checksumColumnPresent,
		ManifestChecksum:      migrationManifestChecksum(manifest),
		KnownCount:            len(manifest),
		AppliedCount:          len(applied),
		Issues:                []MigrationIssue{},
	}
	if !ledgerPresent {
		health.PendingCount = len(manifest)
		health.Issues = append(health.Issues, MigrationIssue{
			Code:   MigrationLedgerMissingCode,
			Detail: "schema_migrations table does not exist",
		})
		return health
	}
	if !checksumColumnPresent {
		health.Issues = append(health.Issues, MigrationIssue{
			Code:   MigrationChecksumColumnMissingCode,
			Detail: "schema_migrations.checksum does not exist; run the current server migrator before doctor",
		})
	}

	known := make(map[string]MigrationDefinition, len(manifest))
	for _, migration := range manifest {
		known[migration.ID] = migration
	}
	appliedByID := make(map[string]AppliedMigration, len(applied))
	for _, migration := range applied {
		appliedByID[migration.ID] = migration
		expected, ok := known[migration.ID]
		if !ok {
			health.Issues = append(health.Issues, MigrationIssue{
				Code:        MigrationUnknownAppliedCode,
				MigrationID: migration.ID,
				Detail:      "database contains a migration that is not embedded in this binary",
			})
			continue
		}
		if !checksumColumnPresent {
			continue
		}
		actual := strings.TrimSpace(migration.Checksum)
		switch {
		case actual == "":
			health.Issues = append(health.Issues, MigrationIssue{
				Code:             MigrationChecksumMissingCode,
				MigrationID:      migration.ID,
				ExpectedChecksum: expected.Checksum,
				Detail:           "known applied migration has no recorded checksum",
			})
		case actual != expected.Checksum:
			health.Issues = append(health.Issues, MigrationIssue{
				Code:             MigrationChecksumMismatchCode,
				MigrationID:      migration.ID,
				ExpectedChecksum: expected.Checksum,
				ActualChecksum:   actual,
				Detail:           "applied migration checksum differs from the embedded SQL",
			})
		}
	}

	for _, migration := range manifest {
		if _, ok := appliedByID[migration.ID]; ok {
			continue
		}
		health.PendingCount++
		health.Issues = append(health.Issues, MigrationIssue{
			Code:             MigrationPendingCode,
			MigrationID:      migration.ID,
			ExpectedChecksum: migration.Checksum,
			Detail:           "embedded migration has not been applied",
		})
	}
	sort.Slice(health.Issues, func(i, j int) bool {
		if health.Issues[i].Code == health.Issues[j].Code {
			return health.Issues[i].MigrationID < health.Issues[j].MigrationID
		}
		return health.Issues[i].Code < health.Issues[j].Code
	})
	health.Healthy = len(health.Issues) == 0
	return health
}
