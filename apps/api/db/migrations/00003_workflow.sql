-- +goose Up

-- Full snapshot of main after each merged version. History is reconstructed
-- from snapshots (needed as the base of 3-way merges and for /versions/{n}).
CREATE TABLE model_snapshots (
    version integer PRIMARY KEY REFERENCES model_versions (number),
    data    jsonb NOT NULL
);

ALTER TABLE changesets
    ADD COLUMN approved_by uuid REFERENCES users (id),
    ADD COLUMN approved_at timestamptz;

CREATE TYPE resolution_choice AS ENUM ('mine', 'theirs');

-- Explicit conflict resolutions recorded before a merge. field = '*' resolves a
-- whole target (delete-vs-update, dangling reference).
CREATE TABLE merge_resolutions (
    changeset_id uuid NOT NULL REFERENCES changesets (id),
    target_type  change_target NOT NULL,
    target_id    uuid NOT NULL,
    field        text NOT NULL,
    choice       resolution_choice NOT NULL,
    PRIMARY KEY (changeset_id, target_type, target_id, field)
);

-- Backfill: databases created at M1 have a current version without a snapshot.
INSERT INTO model_snapshots (version, data)
SELECT v.number,
       jsonb_build_object(
           'elements',  (SELECT coalesce(jsonb_agg(to_jsonb(e) - 'deleted_at' ORDER BY e.id), '[]')
                         FROM elements e WHERE e.deleted_at IS NULL),
           'relations', (SELECT coalesce(jsonb_agg(to_jsonb(r) - 'deleted_at' ORDER BY r.id), '[]')
                         FROM relations r WHERE r.deleted_at IS NULL))
FROM model_versions v
WHERE v.number = (SELECT max(number) FROM model_versions);

-- +goose Down
DROP TABLE merge_resolutions;
DROP TYPE resolution_choice;
ALTER TABLE changesets DROP COLUMN approved_at, DROP COLUMN approved_by;
DROP TABLE model_snapshots;
