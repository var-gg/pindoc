-- +goose Up
-- Reader hot-path indexes for data growth beyond the seed-size dogfood set.

CREATE INDEX IF NOT EXISTS idx_events_warning_subject_created
    ON events(subject_id, created_at DESC, id DESC)
    WHERE kind = 'artifact.warning_raised';

CREATE INDEX IF NOT EXISTS idx_artifact_revisions_created
    ON artifact_revisions(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_artifacts_project_reader_order
    ON artifacts(
        project_id,
        (CASE WHEN type = 'Task' THEN 1 ELSE 0 END),
        updated_at DESC,
        id DESC
    )
    WHERE status <> 'archived';

CREATE INDEX IF NOT EXISTS idx_artifacts_task_project_assignee_priority
    ON artifacts(
        project_id,
        (task_meta->>'assignee'),
        (task_meta->>'priority'),
        updated_at DESC
    )
    WHERE type = 'Task'
      AND status <> 'archived'
      AND status <> 'superseded'
      AND NOT starts_with(slug, '_template_');

CREATE INDEX IF NOT EXISTS idx_artifact_chunks_embedding_hnsw
    ON artifact_chunks
    USING hnsw (embedding vector_cosine_ops)
    WHERE embedding IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_artifact_chunks_embedding_hnsw;
DROP INDEX IF EXISTS idx_artifacts_task_project_assignee_priority;
DROP INDEX IF EXISTS idx_artifacts_project_reader_order;
DROP INDEX IF EXISTS idx_artifact_revisions_created;
DROP INDEX IF EXISTS idx_events_warning_subject_created;
