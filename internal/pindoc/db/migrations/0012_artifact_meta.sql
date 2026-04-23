-- +goose Up
-- Contamination hardening: artifact_meta JSONB carries the epistemic axes
-- that answer "can I trust this artifact / why is it here / does it flow
-- into next-session context". Decision:
--   conversation-vs-canonical-memory-경계-강화-외부-리뷰-흡수-artifact-met
-- Task:
--   artifact-meta-jsonb-스키마-추가-6축-epistemic-metadata-도입
--
-- Shape (all keys optional — unset = "not classified"):
--   {
--     "source_type":          "code" | "artifact" | "user_chat" | "external" | "mixed",
--     "consent_state":        "not_needed" | "requested" | "granted" | "denied",
--     "confidence":           "low" | "medium" | "high",
--     "audience":             "owner_only" | "approvers" | "project_readers",
--     "next_context_policy":  "default" | "opt_in" | "excluded",
--     "verification_state":   "verified" | "partially_verified" | "unverified"
--   }
--
-- JSONB (not individual columns) so the axis set can evolve without a
-- migration per field. Individual fields graduate to columns after
-- dogfood shows a stable query pattern. task_meta (0010) is the
-- precedent.
--
-- NOT NULL DEFAULT '{}' so existing rows read back cleanly — resolvers
-- treat empty object as "unclassified, apply defaults".

ALTER TABLE artifacts
    ADD COLUMN artifact_meta JSONB NOT NULL DEFAULT '{}'::jsonb;

-- GIN index on the whole JSONB with jsonb_path_ops for the keys we
-- actually filter on (next_context_policy, audience, verification_state).
-- Partial index: most existing rows will have '{}' until the next propose
-- backfills them, so skip empty meta to keep the index small.
CREATE INDEX idx_artifacts_artifact_meta
    ON artifacts USING GIN (artifact_meta jsonb_path_ops)
    WHERE artifact_meta <> '{}'::jsonb;

-- +goose Down
DROP INDEX IF EXISTS idx_artifacts_artifact_meta;
ALTER TABLE artifacts DROP COLUMN IF EXISTS artifact_meta;
