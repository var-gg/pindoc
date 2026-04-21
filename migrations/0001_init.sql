-- Phase 2 will populate this. Reserved now so the migration runner (goose
-- or similar) has a stable starting point and docker-compose can mount
-- the directory without a chicken-and-egg bootstrap.
--
-- Planned schema for Phase 2: projects, areas, artifacts, events.
-- Phase 3 adds artifact_chunks with pgvector columns.

CREATE EXTENSION IF NOT EXISTS vector;
