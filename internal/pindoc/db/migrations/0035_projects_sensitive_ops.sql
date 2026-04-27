-- +goose Up
-- Project-level switch for M1.5 Review Queue. The default remains auto so
-- existing self-hosted projects keep the current publish behavior until an
-- operator explicitly opts into confirmation for sensitive operations.

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS sensitive_ops TEXT NOT NULL DEFAULT 'auto';

ALTER TABLE projects
    DROP CONSTRAINT IF EXISTS projects_sensitive_ops_check;

UPDATE projects
   SET sensitive_ops = 'auto'
 WHERE sensitive_ops IS NULL OR sensitive_ops = '';

ALTER TABLE projects
    ALTER COLUMN sensitive_ops SET DEFAULT 'auto';

ALTER TABLE projects
    ALTER COLUMN sensitive_ops SET NOT NULL;

ALTER TABLE projects
    ADD CONSTRAINT projects_sensitive_ops_check
    CHECK (sensitive_ops IN ('auto', 'confirm'));

-- +goose Down
ALTER TABLE projects
    DROP CONSTRAINT IF EXISTS projects_sensitive_ops_check;

ALTER TABLE projects
    DROP COLUMN IF EXISTS sensitive_ops;
