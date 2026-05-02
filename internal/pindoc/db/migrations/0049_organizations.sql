-- +goose Up
-- Organizations: SaaS-ready multi-tenant primitive that lives in OSS too.
--
-- Pindoc dogfoods itself: the same OSS instance serves the owner's wiki
-- publicly (visibility='public') alongside owner-only strategic content
-- (visibility='private'). The Organization layer is the billing-and-
-- ownership boundary that Slack/Notion/Linear/GitHub all share. Putting
-- it in OSS Day-1 — not as SaaS-only — keeps schema unified across
-- self-host and cloud, and means migration self-host -> cloud is data
-- export/import rather than a model fork.
--
-- Layout:
--   organizations (id, slug, name, kind, owner_user_id, created_at, ...)
--   organization_members (org_id, user_id, role, joined_at)
--   users.username  — handle for personal-Org slug; shares namespace with
--                     organizations.slug so /{slug} routing is unambiguous.
--                     V1 leaves null; V1.5+/SaaS signup populates it.
--   projects.organization_id — FK to organizations(id). Backfilled to the
--                              auto-created 'default' Org so existing self-
--                              host data keeps working without app changes.
--
-- Slug rules: the application enforces the same regex + reserved set used
-- by projects/create.go (lowercase kebab, 2-40 chars, blocks /admin /api
-- etc). DB layer uses CHECK + UNIQUE; the human-friendly validation
-- happens in Go so the error surface stays consistent.
--
-- owner_id retention: projects.owner_id (TEXT, default 'default') stays
-- for now. It's exposed on the MCP tool + REST input surface and removing
-- it would be a breaking API change. organization_id is added alongside,
-- and a follow-up cleanup migration will drop owner_id once the higher
-- layers route through organization_id end-to-end.

-- ---------- organizations ----------------------------------------------
CREATE TABLE organizations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL DEFAULT 'team'
                          CHECK (kind IN ('personal', 'team')),
    owner_user_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    -- description is free-form for future profile pages; nullable now.
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ NULL
);

-- A user has at most one personal Org. Enforced via partial unique index
-- on (kind='personal', owner_user_id) for active rows.
CREATE UNIQUE INDEX idx_organizations_personal_one_per_user
    ON organizations (owner_user_id)
    WHERE kind = 'personal' AND deleted_at IS NULL;

CREATE INDEX idx_organizations_owner_user ON organizations(owner_user_id);

-- ---------- organization_members ---------------------------------------
-- A user can belong to many Orgs with different roles. Personal Org always
-- has its owner as the sole 'owner' member; team Orgs can have many.
CREATE TABLE organization_members (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    invited_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);

CREATE INDEX idx_organization_members_user ON organization_members(user_id);

-- ---------- users.username ---------------------------------------------
-- Username is the personal-Org slug. Shares namespace with organizations.
-- slug so /pindoc.org/{slug} routes unambiguously to either a user's
-- personal Org or a team Org. Nullable for self-host bootstrap; SaaS
-- signup makes it required at the application layer.
ALTER TABLE users
    ADD COLUMN username TEXT;

CREATE UNIQUE INDEX idx_users_username_unique
    ON users (username)
    WHERE username IS NOT NULL AND deleted_at IS NULL;

-- ---------- projects.organization_id -----------------------------------
-- Add nullable, backfill to the auto-created 'default' Org, then mark
-- NOT NULL. The legacy owner_id text column stays for now (see header).
ALTER TABLE projects
    ADD COLUMN organization_id UUID REFERENCES organizations(id) ON DELETE RESTRICT;

-- Insert the default Org. Self-host bootstrap historically used owner_id
-- = 'default'; the default Org carries the same slug so URL resolution
-- stays consistent: /pindoc.org/default/p/{slug} -> the same row that
-- /p/{slug} resolved to before.
INSERT INTO organizations (slug, name, kind, description)
VALUES ('default', 'Default', 'team',
        'Auto-created bootstrap organization. Rename when a real owner identity is set.');

-- Backfill every project to the default Org. Future migrations or app
-- logic can re-home individual projects to user-specific personal Orgs.
UPDATE projects
SET organization_id = (SELECT id FROM organizations WHERE slug = 'default')
WHERE organization_id IS NULL;

ALTER TABLE projects
    ALTER COLUMN organization_id SET NOT NULL;

-- New ownership-scoped uniqueness: (organization_id, slug). The legacy
-- (owner_id, slug) UNIQUE stays so downgrade is safe; both must hold
-- during the transition window.
CREATE UNIQUE INDEX idx_projects_org_slug
    ON projects (organization_id, slug);

CREATE INDEX idx_projects_org ON projects(organization_id);


-- +goose Down
DROP INDEX IF EXISTS idx_projects_org;
DROP INDEX IF EXISTS idx_projects_org_slug;
ALTER TABLE projects DROP COLUMN IF EXISTS organization_id;

DROP INDEX IF EXISTS idx_users_username_unique;
ALTER TABLE users DROP COLUMN IF EXISTS username;

DROP INDEX IF EXISTS idx_organization_members_user;
DROP TABLE IF EXISTS organization_members;

DROP INDEX IF EXISTS idx_organizations_owner_user;
DROP INDEX IF EXISTS idx_organizations_personal_one_per_user;
DROP TABLE IF EXISTS organizations;
