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

func TestOAuthDCRLifecycleMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0060_oauth_dcr_lifecycle.sql")
	if err != nil {
		t.Fatalf("read oauth dcr lifecycle migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ NULL",
		"ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL",
		"SET expires_at = created_at + interval '90 days'",
		"WHERE created_via = 'dcr'",
		"CREATE INDEX IF NOT EXISTS idx_oauth_clients_dcr_expires_at",
		"CREATE INDEX IF NOT EXISTS idx_oauth_clients_dcr_last_used_at",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("oauth dcr lifecycle migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS idx_oauth_clients_dcr_last_used_at",
		"DROP INDEX IF EXISTS idx_oauth_clients_dcr_expires_at",
		"DROP COLUMN IF EXISTS expires_at",
		"DROP COLUMN IF EXISTS last_used_at",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("oauth dcr lifecycle migration Down missing %q", want)
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

func TestPinRepoIDAndKindVocabularyMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0043_pin_repo_id_and_kind_vocabulary.sql")
	if err != nil {
		t.Fatalf("read pin repo_id migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS local_paths TEXT[]",
		"ADD COLUMN IF NOT EXISTS urls",
		"ADD COLUMN IF NOT EXISTS repo_id UUID NULL REFERENCES project_repos(id) ON DELETE SET NULL",
		"CREATE INDEX IF NOT EXISTS idx_artifact_pins_repo_id",
		"CHECK (kind IN ('code', 'doc', 'config', 'asset', 'resource', 'url'))",
		"UPDATE artifact_pins p",
		"SET repo_id = pr.id",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("pin repo_id migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS idx_artifact_pins_repo_id",
		"DROP COLUMN IF EXISTS repo_id",
		"DROP COLUMN IF EXISTS urls",
		"DROP COLUMN IF EXISTS local_paths",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("pin repo_id migration Down missing %q", want)
		}
	}
}

func TestOrganizationsMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0049_organizations.sql")
	if err != nil {
		t.Fatalf("read organizations migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"CREATE TABLE organizations",
		"id              UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"slug            TEXT NOT NULL UNIQUE",
		"name            TEXT NOT NULL",
		"kind            TEXT NOT NULL DEFAULT 'team'",
		"CHECK (kind IN ('personal', 'team'))",
		"owner_user_id   UUID REFERENCES users(id) ON DELETE SET NULL",
		"deleted_at      TIMESTAMPTZ NULL",
		"CREATE UNIQUE INDEX idx_organizations_personal_one_per_user",
		"WHERE kind = 'personal' AND deleted_at IS NULL",
		"CREATE TABLE organization_members",
		"organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE",
		"user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE",
		"role            TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member'))",
		"PRIMARY KEY (organization_id, user_id)",
		"ALTER TABLE users",
		"ADD COLUMN username TEXT",
		"CREATE UNIQUE INDEX idx_users_username_unique",
		"WHERE username IS NOT NULL AND deleted_at IS NULL",
		"ALTER TABLE projects",
		"ADD COLUMN organization_id UUID REFERENCES organizations(id) ON DELETE RESTRICT",
		"INSERT INTO organizations (slug, name, kind, description)",
		"'default'",
		"UPDATE projects",
		"SET organization_id = (SELECT id FROM organizations WHERE slug = 'default')",
		"ALTER COLUMN organization_id SET NOT NULL",
		"CREATE UNIQUE INDEX idx_projects_org_slug",
		"ON projects (organization_id, slug)",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("organizations migration Up missing %q:\n---\n%s\n---", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS idx_projects_org",
		"DROP INDEX IF EXISTS idx_projects_org_slug",
		"DROP COLUMN IF EXISTS organization_id",
		"DROP INDEX IF EXISTS idx_users_username_unique",
		"DROP COLUMN IF EXISTS username",
		"DROP TABLE IF EXISTS organization_members",
		"DROP TABLE IF EXISTS organizations",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("organizations migration Down missing %q", want)
		}
	}
}

func TestProjectDefaultVisibilityMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0051_project_default_visibility.sql")
	if err != nil {
		t.Fatalf("read project default visibility migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"ALTER TABLE projects",
		"ADD COLUMN default_artifact_visibility TEXT NOT NULL DEFAULT 'org'",
		"CHECK (default_artifact_visibility IN ('public', 'org', 'private'))",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("project default visibility migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP COLUMN IF EXISTS default_artifact_visibility",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("project default visibility migration Down missing %q", want)
		}
	}
}

func TestArtifactSlugAliasesMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0052_artifact_slug_aliases.sql")
	if err != nil {
		t.Fatalf("read artifact slug aliases migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"CREATE TABLE artifact_slug_aliases",
		"project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE",
		"artifact_id UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE",
		"old_slug    TEXT NOT NULL",
		"UNIQUE (project_id, old_slug)",
		"CREATE INDEX idx_artifact_slug_aliases_artifact",
		"ON artifact_slug_aliases(artifact_id)",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("artifact slug aliases migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS idx_artifact_slug_aliases_artifact",
		"DROP TABLE IF EXISTS artifact_slug_aliases",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("artifact slug aliases migration Down missing %q", want)
		}
	}
}

func TestArtifactRevisionAuthorUserMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0053_artifact_revision_author_user.sql")
	if err != nil {
		t.Fatalf("read artifact revision author user migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"ALTER TABLE artifact_revisions",
		"ADD COLUMN IF NOT EXISTS author_user_id UUID",
		"artifact_revisions_author_user_id_fkey",
		"FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE SET NULL",
		"CREATE INDEX IF NOT EXISTS idx_artifact_revisions_author_user",
		"UPDATE artifact_revisions r",
		"SET author_user_id = a.author_user_id",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("artifact revision author user migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS idx_artifact_revisions_author_user",
		"DROP CONSTRAINT IF EXISTS artifact_revisions_author_user_id_fkey",
		"DROP COLUMN IF EXISTS author_user_id",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("artifact revision author user migration Down missing %q", want)
		}
	}
}

func TestAssetsMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0054_assets.sql")
	if err != nil {
		t.Fatalf("read assets migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"CREATE TABLE assets",
		"project_id        UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE",
		"sha256            TEXT NOT NULL",
		"storage_driver    TEXT NOT NULL",
		"storage_key       TEXT NOT NULL",
		"UNIQUE (project_id, sha256)",
		"CREATE TABLE artifact_assets",
		"artifact_revision_id UUID NOT NULL REFERENCES artifact_revisions(id) ON DELETE CASCADE",
		"role                 TEXT NOT NULL CHECK (role IN ('inline_image', 'attachment', 'evidence', 'generated_output'))",
		"UNIQUE (artifact_revision_id, asset_id, role)",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("assets migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP TABLE IF EXISTS artifact_assets",
		"DROP TABLE IF EXISTS assets",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("assets migration Down missing %q", want)
		}
	}
}

func TestDropProjectsOwnerIDMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0055_drop_projects_owner_id.sql")
	if err != nil {
		t.Fatalf("read drop projects owner_id migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"DROP CONSTRAINT IF EXISTS projects_owner_slug_locale_unique",
		"DROP CONSTRAINT IF EXISTS projects_owner_slug_key",
		"DROP INDEX IF EXISTS idx_projects_owner",
		"DROP COLUMN IF EXISTS owner_id",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("drop projects owner_id migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"ADD COLUMN IF NOT EXISTS owner_id TEXT NOT NULL DEFAULT 'default'",
		"CREATE INDEX IF NOT EXISTS idx_projects_owner ON projects(owner_id)",
		"ADD CONSTRAINT projects_owner_slug_key UNIQUE (owner_id, slug)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("drop projects owner_id migration Down missing %q", want)
		}
	}
}

func TestNormalizeTaskAssigneeUserUUIDMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0056_normalize_task_assignee_user_uuid.sql")
	if err != nil {
		t.Fatalf("read normalize task assignee migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"JOIN users u",
		"a.type = 'Task'",
		"u.deleted_at IS NULL",
		"a.task_meta->>'assignee' ~* '^user:[0-9a-f]{8}-",
		"jsonb_set(",
		"to_jsonb(r.normalized_assignee)",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("normalize task assignee migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"Irreversible data cleanup",
		"SELECT 1",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("normalize task assignee migration Down missing %q", want)
		}
	}
}

func TestVisibilityMigrationContract(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0050_visibility.sql")
	if err != nil {
		t.Fatalf("read visibility migration: %v", err)
	}
	sql := string(raw)
	up := extractUp(sql)
	for _, want := range []string{
		"ALTER TABLE projects",
		"ADD COLUMN visibility TEXT NOT NULL DEFAULT 'org'",
		"CHECK (visibility IN ('public', 'org', 'private'))",
		"ALTER TABLE artifacts",
		"CREATE INDEX idx_projects_visibility ON projects(visibility)",
		"CREATE INDEX idx_artifacts_visibility ON artifacts(visibility)",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("visibility migration Up missing %q:\n%s", want, up)
		}
	}
	for _, want := range []string{
		"-- +goose Down",
		"DROP INDEX IF EXISTS idx_artifacts_visibility",
		"DROP INDEX IF EXISTS idx_projects_visibility",
		"DROP COLUMN IF EXISTS visibility",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("visibility migration Down missing %q", want)
		}
	}
}
