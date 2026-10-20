-- +goose Up

-- Type facet and operational fields on elements (ADR-062, ADR-065).
ALTER TABLE elements
    ADD COLUMN type text NOT NULL DEFAULT ''
        CHECK (type IN ('', 'database-server', 'database', 'schema', 'cache', 'queue', 'proxy')),
    ADD COLUMN environment text NOT NULL DEFAULT '' CHECK (environment IN ('', 'prod', 'staging', 'dev')),
    ADD COLUMN lifecycle text NOT NULL DEFAULT 'active' CHECK (lifecycle IN ('active', 'deprecated', 'planned')),
    ADD COLUMN criticality text NOT NULL DEFAULT '' CHECK (criticality IN ('', 'low', 'medium', 'high', 'critical')),
    ADD COLUMN runbook_url text NOT NULL DEFAULT '',
    ADD COLUMN docs_url text NOT NULL DEFAULT '',
    ADD COLUMN last_reviewed_at date;

-- Owners are model data, not login users (ADR-063). external_ref is reserved for a
-- later identity-provider group mapping and not exposed by the API yet.
CREATE TYPE owner_kind AS ENUM ('team', 'person');
CREATE TABLE owners (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    kind         owner_kind NOT NULL,
    name         text NOT NULL CHECK (btrim(name) <> ''),
    contact      text NOT NULL DEFAULT '',
    oncall_url   text NOT NULL DEFAULT '',
    external_ref text,
    version      integer NOT NULL DEFAULT 1,
    deleted_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- One owner per (element, role); the id is derived from both in Go (model.OwnershipID).
CREATE TYPE owner_role AS ENUM ('owner', 'maintainer', 'oncall', 'escalation');
CREATE TABLE element_owners (
    id         uuid PRIMARY KEY,
    element_id uuid NOT NULL REFERENCES elements (id),
    owner_id   uuid NOT NULL REFERENCES owners (id),
    role       owner_role NOT NULL,
    version    integer NOT NULL DEFAULT 1,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX element_owners_live_idx ON element_owners (element_id, role) WHERE deleted_at IS NULL;
CREATE INDEX element_owners_owner_idx ON element_owners (owner_id);

ALTER TYPE change_target ADD VALUE IF NOT EXISTS 'owner';
ALTER TYPE change_target ADD VALUE IF NOT EXISTS 'ownership';
ALTER TYPE view_kind ADD VALUE IF NOT EXISTS 'unowned'; -- IF NOT EXISTS: down cannot remove enum values

-- Retire the `database` kind (ADR-062, supersedes ADR-047's kind): a container-level
-- datastore becomes a container of type `database`, keeping its id. Historical
-- snapshots and open drafts are normalized when read (model.Element.Normalized).
UPDATE elements SET kind = 'container', type = 'database' WHERE kind = 'database';

-- +goose Down
UPDATE elements SET kind = 'database' WHERE kind = 'container' AND type = 'database';
DROP TABLE element_owners;
DROP TYPE owner_role;
DROP TABLE owners;
DROP TYPE owner_kind;
ALTER TABLE elements
    DROP COLUMN last_reviewed_at, DROP COLUMN docs_url, DROP COLUMN runbook_url,
    DROP COLUMN criticality, DROP COLUMN lifecycle, DROP COLUMN environment, DROP COLUMN type;
-- (the 'owner' / 'ownership' / 'unowned' enum values cannot be removed; they are harmless)
