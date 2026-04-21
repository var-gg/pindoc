-- +goose Up
-- M1 Phase 2 schema. Embedding columns land in 0003 alongside Phase 3.

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE projects (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug             TEXT UNIQUE NOT NULL,
    name             TEXT NOT NULL,
    description      TEXT,
    color            TEXT,
    primary_language TEXT NOT NULL DEFAULT 'en',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_projects_slug ON projects(slug);

CREATE TABLE areas (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_id        UUID REFERENCES areas(id) ON DELETE CASCADE,
    slug             TEXT NOT NULL,
    name             TEXT NOT NULL,
    description      TEXT,
    is_cross_cutting BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug)
);
CREATE INDEX idx_areas_project ON areas(project_id);
CREATE INDEX idx_areas_parent  ON areas(parent_id);

CREATE TABLE artifacts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id         UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    area_id            UUID NOT NULL REFERENCES areas(id)     ON DELETE RESTRICT,
    slug               TEXT NOT NULL,
    type               TEXT NOT NULL,
    title              TEXT NOT NULL,
    body_markdown      TEXT NOT NULL DEFAULT '',
    body_json          JSONB,
    tags               TEXT[] NOT NULL DEFAULT '{}',

    completeness       TEXT NOT NULL DEFAULT 'draft'
                       CHECK (completeness IN ('draft', 'partial', 'settled')),
    status             TEXT NOT NULL DEFAULT 'published'
                       CHECK (status IN ('published', 'stale', 'superseded', 'archived')),
    review_state       TEXT NOT NULL DEFAULT 'auto_published'
                       CHECK (review_state IN ('auto_published', 'pending_review', 'approved', 'rejected')),

    author_kind        TEXT NOT NULL DEFAULT 'agent'
                       CHECK (author_kind IN ('agent', 'system')),
    author_id          TEXT NOT NULL,
    author_version    TEXT,
    source_session_ref JSONB,

    superseded_by      UUID REFERENCES artifacts(id),

    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at       TIMESTAMPTZ,

    UNIQUE (project_id, slug)
);
CREATE INDEX idx_artifacts_project       ON artifacts(project_id);
CREATE INDEX idx_artifacts_area          ON artifacts(area_id);
CREATE INDEX idx_artifacts_type          ON artifacts(type);
CREATE INDEX idx_artifacts_status        ON artifacts(status);
CREATE INDEX idx_artifacts_review_state  ON artifacts(review_state);
CREATE INDEX idx_artifacts_tags          ON artifacts USING GIN (tags);
CREATE INDEX idx_artifacts_tsv ON artifacts
    USING GIN (to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(body_markdown, '')));

CREATE TABLE events (
    id           BIGSERIAL PRIMARY KEY,
    project_id   UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    subject_id   UUID,
    payload      JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_project_time ON events(project_id, created_at DESC);
CREATE INDEX idx_events_kind         ON events(kind);

-- +goose Down
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS artifacts;
DROP TABLE IF EXISTS areas;
DROP TABLE IF EXISTS projects;
