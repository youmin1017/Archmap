-- +goose Up

-- App-managed user groups (ADR-070): account administration, not model data.
CREATE TABLE groups (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    name        text NOT NULL CHECK (btrim(name) <> ''),
    description text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
CREATE UNIQUE INDEX groups_name_live_idx ON groups (lower(name)) WHERE deleted_at IS NULL;

CREATE TABLE group_members (
    group_id uuid NOT NULL REFERENCES groups (id),
    user_id  uuid NOT NULL REFERENCES users (id),
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_members_user_idx ON group_members (user_id);

-- Owners may point at a user or a group (ADR-069). No foreign key: owners are
-- versioned model data and groups can be deleted.
ALTER TYPE owner_kind ADD VALUE IF NOT EXISTS 'user';
ALTER TYPE owner_kind ADD VALUE IF NOT EXISTS 'group';
ALTER TABLE owners ADD COLUMN principal_id uuid;
CREATE UNIQUE INDEX owners_principal_live_idx ON owners (principal_id) WHERE deleted_at IS NULL AND principal_id IS NOT NULL;

-- +goose Down
DROP INDEX owners_principal_live_idx;
ALTER TABLE owners DROP COLUMN principal_id;
DROP TABLE group_members;
DROP TABLE groups;
-- (the 'user' / 'group' owner_kind values cannot be removed; they are harmless)
