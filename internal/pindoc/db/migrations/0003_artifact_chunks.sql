-- +goose Up
-- Phase 3: chunked body embeddings.
--
-- Dimension is provider-specific (384 for MiniLM-L12-v2, 768 for
-- EmbeddingGemma, 1024 for BGE-M3, 3072 for OpenAI text-embedding-3-large).
-- We size the column for the largest we plan to support in V1 (768 /
-- EmbeddingGemma) and pad shorter vectors to 768 on insert. If we later
-- need 1024/3072 we add a second column rather than resizing in place —
-- pgvector doesn't support ALTER TYPE vector(N).
--
-- Actually, pgvector DOES let us keep per-row dimensions variable now
-- (since 0.5+), but keeping a fixed column width makes HNSW indexes
-- possible. V1 fixes at 768. Provider.Info().Dimension tells us to pad.

CREATE TABLE artifact_chunks (
    id              BIGSERIAL PRIMARY KEY,
    artifact_id     UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('title', 'body')),
    chunk_index     INT  NOT NULL,
    heading         TEXT,
    span_start      INT  NOT NULL DEFAULT 0,
    span_end        INT  NOT NULL DEFAULT 0,
    text            TEXT NOT NULL,
    embedding       vector(768),       -- padded on insert for <768-dim providers
    model_name      TEXT NOT NULL,
    model_dim       INT  NOT NULL,     -- actual vector length before padding
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (artifact_id, kind, chunk_index)
);

CREATE INDEX idx_artifact_chunks_artifact ON artifact_chunks(artifact_id);

-- HNSW index for cosine similarity. Build once at ~100s of artifacts; for
-- tiny V1 datasets a seqscan is fine, so we skip the index creation until
-- the project crosses N>500 artifacts. Saving the index creation for a
-- future migration keeps bootstrap fast.
-- (Planned: CREATE INDEX ... USING hnsw (embedding vector_cosine_ops);)

-- +goose Down
DROP TABLE IF EXISTS artifact_chunks;
