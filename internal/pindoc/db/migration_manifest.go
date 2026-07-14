package db

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// MigrationDefinition is the immutable identity of one embedded migration.
// The checksum covers the complete, line-ending-normalized SQL file so both
// Up and Down blocks remain part of the reviewed migration contract.
type MigrationDefinition struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Checksum string `json:"checksum"`
}

// 0018 predates the unique-version guard and shipped as two independent
// migrations. Keep that exact pair readable without allowing any new file to
// reuse the version. New migrations must have one numeric prefix each.
var legacyDuplicateMigrationVersions = map[string]map[string]struct{}{
	"0018": {
		"0018_artifact_scope_edges.sql":   {},
		"0018_backfill_task_assignee.sql": {},
	},
}

func embeddedMigrationManifest() ([]MigrationDefinition, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}

	manifest := make([]MigrationDefinition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		raw, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		version, err := migrationVersion(entry.Name())
		if err != nil {
			return nil, err
		}
		if extractUp(string(raw)) == "" {
			return nil, fmt.Errorf("%s: no +goose Up block found", entry.Name())
		}
		manifest = append(manifest, MigrationDefinition{
			ID:       entry.Name(),
			Version:  version,
			Checksum: migrationChecksum(raw),
		})
	}
	sort.Slice(manifest, func(i, j int) bool { return manifest[i].ID < manifest[j].ID })
	if err := validateMigrationVersions(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func migrationVersion(name string) (string, error) {
	underscore := strings.IndexByte(name, '_')
	if underscore <= 0 {
		return "", fmt.Errorf("migration %q must start with a numeric version followed by underscore", name)
	}
	version := name[:underscore]
	for _, r := range version {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("migration %q has non-numeric version %q", name, version)
		}
	}
	return version, nil
}

func migrationChecksum(raw []byte) string {
	canonical := bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	canonical = bytes.TrimPrefix(canonical, []byte{0xef, 0xbb, 0xbf})
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func migrationManifestChecksum(manifest []MigrationDefinition) string {
	hash := sha256.New()
	for _, migration := range manifest {
		_, _ = fmt.Fprintf(hash, "%s:%s\n", migration.ID, migration.Checksum)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validateMigrationVersions(manifest []MigrationDefinition) error {
	byVersion := make(map[string][]string, len(manifest))
	for _, migration := range manifest {
		byVersion[migration.Version] = append(byVersion[migration.Version], migration.ID)
	}
	for version, ids := range byVersion {
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		allowed := legacyDuplicateMigrationVersions[version]
		if len(allowed) == len(ids) {
			allAllowed := true
			for _, id := range ids {
				if _, ok := allowed[id]; !ok {
					allAllowed = false
					break
				}
			}
			if allAllowed {
				continue
			}
		}
		return fmt.Errorf("MIGRATION_DUPLICATE_VERSION: version %s is used by %s", version, strings.Join(ids, ", "))
	}
	return nil
}
