-- +goose Up
-- evidence edge — artifact-to-artifact support proof. Concrete repository,
-- file, line, or URL proof remains artifact_pins; this relation is for the
-- case where the supporting source is itself another Pindoc artifact.

ALTER TABLE artifact_edges DROP CONSTRAINT IF EXISTS artifact_edges_relation_check;
ALTER TABLE artifact_edges ADD CONSTRAINT artifact_edges_relation_check
    CHECK (relation IN (
        'implements',
        'references',
        'blocks',
        'relates_to',
        'translation_of',
        'evidence'
    ));

-- +goose Down
ALTER TABLE artifact_edges DROP CONSTRAINT IF EXISTS artifact_edges_relation_check;
-- Reverting requires evidence rows to be deleted first:
--   DELETE FROM artifact_edges WHERE relation = 'evidence';
ALTER TABLE artifact_edges ADD CONSTRAINT artifact_edges_relation_check
    CHECK (relation IN (
        'implements',
        'references',
        'blocks',
        'relates_to',
        'translation_of'
    ));
