-- +goose Up

-- Workspace-wide settings (one row). review_policy decides whether main can be edited
-- directly (each save is an auto-merged changeset) or only through reviewed drafts (ADR-072).
CREATE TABLE settings (
    id            boolean PRIMARY KEY DEFAULT true CHECK (id),
    review_policy text NOT NULL DEFAULT 'required' CHECK (review_policy IN ('required', 'direct')),
    updated_by    uuid REFERENCES users (id),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
INSERT INTO settings DEFAULT VALUES;

-- kind 'direct': a direct edit of main, merged automatically (hidden from the changeset list).
-- reverts: the direct edit this changeset undoes.
ALTER TABLE changesets
    ADD COLUMN kind text NOT NULL DEFAULT 'draft' CHECK (kind IN ('draft', 'direct')),
    ADD COLUMN reverts uuid REFERENCES changesets (id);
CREATE UNIQUE INDEX changesets_reverts ON changesets (reverts) WHERE reverts IS NOT NULL;

-- +goose Down
DROP INDEX changesets_reverts;
ALTER TABLE changesets DROP COLUMN reverts, DROP COLUMN kind;
DROP TABLE settings;
