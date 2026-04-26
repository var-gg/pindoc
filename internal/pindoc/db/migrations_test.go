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
