package db

import (
	"strings"
	"testing"
)

func TestProjectMembersMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0032_project_members.sql")
	if err != nil {
		t.Fatalf("read project_members migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"CREATE TABLE project_members",
		"project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE",
		"user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE",
		"role       TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer'))",
		"invited_by UUID REFERENCES users(id) ON DELETE SET NULL",
		"PRIMARY KEY (project_id, user_id)",
		"CREATE INDEX idx_project_members_user ON project_members(user_id)",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("project_members migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS idx_project_members_user",
		"DROP TABLE IF EXISTS project_members",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("project_members migration Down missing %q", want)
		}
	}
}

func TestUsersEmailCanonicalMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0033_users_email_canonical.sql")
	if err != nil {
		t.Fatalf("read users email canonical migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"ADD COLUMN deleted_at TIMESTAMPTZ NULL",
		"USERS_EMAIL_LOWER_DUPLICATE",
		"GROUP BY lower(email)",
		"DROP INDEX IF EXISTS idx_users_email_unique",
		"CREATE UNIQUE INDEX idx_users_email_unique",
		"AND deleted_at IS NULL",
		"CREATE UNIQUE INDEX users_email_lower_idx",
		"ON users (lower(email))",
		"WHERE deleted_at IS NULL",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("users email canonical migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS users_email_lower_idx",
		"CREATE UNIQUE INDEX idx_users_email_unique",
		"WHERE email IS NOT NULL",
		"ALTER TABLE users DROP COLUMN IF EXISTS deleted_at",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("users email canonical migration Down missing %q", want)
		}
	}
}
