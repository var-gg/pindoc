-- +goose Up
-- translation_of edge — cross-locale pairing (Task
-- `task-phase-18-project-locale-implementation`). When the same conceptual
-- artifact exists in multiple locales (e.g. `pindoc/ko` + `pindoc/en`) the
-- graph carries a `translation_of` edge between them so Reader can surface
-- a "Switch language" affordance and retrieval can de-duplicate across
-- locales.
--
-- CHECK constraints cannot be altered in place, so drop-and-recreate.

ALTER TABLE artifact_edges DROP CONSTRAINT IF EXISTS artifact_edges_relation_check;
ALTER TABLE artifact_edges ADD CONSTRAINT artifact_edges_relation_check
    CHECK (relation IN (
        'implements',
        'references',
        'blocks',
        'relates_to',
        'translation_of'
    ));

-- +goose Down
ALTER TABLE artifact_edges DROP CONSTRAINT IF EXISTS artifact_edges_relation_check;
-- Reverting requires any translation_of rows to be deleted first, otherwise
-- the revived CHECK fails. Caller is expected to `DELETE FROM
-- artifact_edges WHERE relation = 'translation_of'` before running Down.
ALTER TABLE artifact_edges ADD CONSTRAINT artifact_edges_relation_check
    CHECK (relation IN (
        'implements',
        'references',
        'blocks',
        'relates_to'
    ));
