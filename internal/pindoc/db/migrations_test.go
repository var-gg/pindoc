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

func TestReadEventsMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0034_read_events.sql")
	if err != nil {
		t.Fatalf("read read_events migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"CREATE TABLE read_events",
		"artifact_id    UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE",
		"user_id        UUID REFERENCES users(id) ON DELETE SET NULL",
		"active_seconds DOUBLE PRECISION NOT NULL DEFAULT 0",
		"scroll_max_pct DOUBLE PRECISION NOT NULL DEFAULT 0",
		"CHECK (scroll_max_pct >= 0 AND scroll_max_pct <= 1)",
		"CHECK (active_seconds <= EXTRACT(EPOCH FROM (ended_at - started_at)))",
		"CREATE INDEX idx_read_events_artifact_started",
		"ON read_events(artifact_id, started_at)",
		"CREATE INDEX idx_read_events_user_started",
		"ON read_events(user_id, started_at)",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("read_events migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS idx_read_events_user_started",
		"DROP INDEX IF EXISTS idx_read_events_artifact_started",
		"DROP TABLE IF EXISTS read_events",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("read_events migration Down missing %q", want)
		}
	}
}

func TestProjectsSensitiveOpsMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0035_projects_sensitive_ops.sql")
	if err != nil {
		t.Fatalf("read projects sensitive ops migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS sensitive_ops TEXT NOT NULL DEFAULT 'auto'",
		"DROP CONSTRAINT IF EXISTS projects_sensitive_ops_check",
		"ADD CONSTRAINT projects_sensitive_ops_check",
		"CHECK (sensitive_ops IN ('auto', 'confirm'))",
		"SET sensitive_ops = 'auto'",
		"ALTER COLUMN sensitive_ops SET DEFAULT 'auto'",
		"ALTER COLUMN sensitive_ops SET NOT NULL",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("projects sensitive_ops migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP CONSTRAINT IF EXISTS projects_sensitive_ops_check",
		"DROP COLUMN IF EXISTS sensitive_ops",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("projects sensitive_ops migration Down missing %q", want)
		}
	}
}

func TestAuditUserFKsSetNullMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0036_audit_user_fks_set_null.sql")
	if err != nil {
		t.Fatalf("read audit user fks migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"artifacts_author_user_id_fkey",
		"artifact_scope_edges_created_by_user_id_fkey",
		"mcp_tool_calls_user_id_fkey",
		"project_members_invited_by_fkey",
		"read_events_user_id_fkey",
		"ON DELETE SET NULL",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("audit user fks migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP CONSTRAINT IF EXISTS read_events_user_id_fkey",
		"DROP CONSTRAINT IF EXISTS project_members_invited_by_fkey",
		"DROP CONSTRAINT IF EXISTS mcp_tool_calls_user_id_fkey",
		"DROP CONSTRAINT IF EXISTS artifact_scope_edges_created_by_user_id_fkey",
		"DROP CONSTRAINT IF EXISTS artifacts_author_user_id_fkey",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("audit user fks migration Down missing %q", want)
		}
	}
}
