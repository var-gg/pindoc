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

func TestInviteTokensMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0028_invite_tokens.sql")
	if err != nil {
		t.Fatalf("read invite_tokens migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"CREATE TABLE invite_tokens",
		"token_hash  TEXT PRIMARY KEY",
		"project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE",
		"role        TEXT NOT NULL CHECK (role IN ('editor', 'viewer'))",
		"issued_by   UUID REFERENCES users(id) ON DELETE SET NULL",
		"consumed_by UUID REFERENCES users(id) ON DELETE SET NULL",
		"CREATE INDEX idx_invite_tokens_project_consumed",
		"ON invite_tokens(project_id, consumed_at)",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("invite_tokens migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS idx_invite_tokens_expires_at",
		"DROP INDEX IF EXISTS idx_invite_tokens_project_consumed",
		"DROP TABLE IF EXISTS invite_tokens",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("invite_tokens migration Down missing %q", want)
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

func TestOAuthStorageMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0037_oauth_storage.sql")
	if err != nil {
		t.Fatalf("read oauth storage migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"CREATE TABLE oauth_clients",
		"client_id      TEXT PRIMARY KEY",
		"secret_hash    BYTEA NULL",
		"redirect_uris  TEXT[] NOT NULL",
		"CREATE TABLE oauth_authorize_codes",
		"code_hash        TEXT PRIMARY KEY",
		"form_data        JSONB NOT NULL DEFAULT '{}'::jsonb",
		"session          JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CREATE TABLE oauth_access_tokens",
		"CREATE TABLE oauth_refresh_tokens",
		"rotated_from      TEXT NULL REFERENCES oauth_refresh_tokens(token_hash) ON DELETE SET NULL",
		"CREATE TABLE oauth_pkce_requests",
		"code_challenge_method TEXT NOT NULL",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("oauth storage migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP TABLE IF EXISTS oauth_pkce_requests",
		"DROP TABLE IF EXISTS oauth_refresh_tokens",
		"DROP TABLE IF EXISTS oauth_access_tokens",
		"DROP TABLE IF EXISTS oauth_authorize_codes",
		"DROP TABLE IF EXISTS oauth_clients",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("oauth storage migration Down missing %q", want)
		}
	}
}

func TestUsersOAuthProviderMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0038_users_oauth_provider.sql")
	if err != nil {
		t.Fatalf("read users oauth provider migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS provider TEXT NULL",
		"ADD COLUMN IF NOT EXISTS provider_uid TEXT NULL",
		"ADD CONSTRAINT users_provider_check",
		"CHECK (provider IS NULL OR provider IN ('github'))",
		"CREATE UNIQUE INDEX IF NOT EXISTS users_provider_uid_unique",
		"ON users (provider, provider_uid)",
		"AND deleted_at IS NULL",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("users oauth provider migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS users_provider_uid_unique",
		"DROP CONSTRAINT IF EXISTS users_provider_check",
		"DROP COLUMN IF EXISTS provider_uid",
		"DROP COLUMN IF EXISTS provider",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("users oauth provider migration Down missing %q", want)
		}
	}
}

func TestProjectReposMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0039_project_repos.sql")
	if err != nil {
		t.Fatalf("read project_repos migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"CREATE TABLE project_repos",
		"id                      UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"project_id              UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE",
		"git_remote_url          TEXT NOT NULL",
		"git_remote_url_original TEXT NOT NULL DEFAULT ''",
		"default_branch          TEXT NOT NULL DEFAULT 'main'",
		"UNIQUE (project_id, git_remote_url)",
		"CREATE INDEX idx_project_repos_git_remote_url",
		"ON project_repos(git_remote_url)",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("project_repos migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS idx_project_repos_git_remote_url",
		"DROP TABLE IF EXISTS project_repos",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("project_repos migration Down missing %q", want)
		}
	}
}
