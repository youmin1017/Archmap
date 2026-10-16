-- +goose Up
CREATE TYPE element_kind AS ENUM ('person', 'system', 'container', 'component', 'node');
CREATE TYPE relation_kind AS ENUM ('uses', 'deployed_on');
CREATE TYPE changeset_status AS ENUM ('draft', 'in_review', 'merged', 'rejected', 'abandoned');
CREATE TYPE change_op AS ENUM ('add', 'update', 'delete');
CREATE TYPE change_target AS ENUM ('element', 'relation');

-- elements / relations hold the CURRENT main (materialized) state.
CREATE TABLE elements (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    parent_id   uuid REFERENCES elements (id),
    kind        element_kind NOT NULL,
    name        text NOT NULL CHECK (btrim(name) <> ''),
    description text NOT NULL DEFAULT '',
    technology  text NOT NULL DEFAULT '',
    tags        text[] NOT NULL DEFAULT '{}',
    protected   boolean NOT NULL DEFAULT false,
    version     integer NOT NULL DEFAULT 1,
    deleted_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX elements_parent_id_idx ON elements (parent_id);

CREATE TABLE relations (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    source_id   uuid NOT NULL REFERENCES elements (id),
    target_id   uuid NOT NULL REFERENCES elements (id),
    kind        relation_kind NOT NULL,
    description text NOT NULL DEFAULT '',
    technology  text NOT NULL DEFAULT '',
    version     integer NOT NULL DEFAULT 1,
    deleted_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX relations_source_id_idx ON relations (source_id);
CREATE INDEX relations_target_id_idx ON relations (target_id);

CREATE TABLE changesets (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    title        text NOT NULL CHECK (btrim(title) <> ''),
    description  text NOT NULL DEFAULT '',
    author_id    uuid NOT NULL REFERENCES users (id),
    base_version integer NOT NULL,
    status       changeset_status NOT NULL DEFAULT 'draft',
    created_at   timestamptz NOT NULL DEFAULT now(),
    merged_at    timestamptz
);
CREATE INDEX changesets_author_id_idx ON changesets (author_id);

-- batch_id groups the changes of one GUI edit (undo / redo unit).
-- undone_at is set by undo and cleared by redo; undone rows are ignored when
-- computing the effective model.
CREATE TABLE changes (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    changeset_id uuid NOT NULL REFERENCES changesets (id),
    seq          integer NOT NULL,
    batch_id     uuid NOT NULL,
    op           change_op NOT NULL,
    target_type  change_target NOT NULL,
    target_id    uuid NOT NULL,
    patch        jsonb NOT NULL DEFAULT '{}',
    before       jsonb,
    undone_at    timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (changeset_id, seq)
);

CREATE TABLE model_versions (
    number       integer PRIMARY KEY,
    changeset_id uuid NOT NULL REFERENCES changesets (id),
    merged_by    uuid NOT NULL REFERENCES users (id),
    merged_at    timestamptz NOT NULL DEFAULT now(),
    summary      text NOT NULL DEFAULT ''
);

-- Reserved for future live-status bindings (Prometheus / OpenEverest). No logic yet.
CREATE TABLE bindings (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    element_id uuid NOT NULL REFERENCES elements (id),
    kind       text NOT NULL,
    config     jsonb NOT NULL DEFAULT '{}'
);

-- +goose Down
DROP TABLE bindings;
DROP TABLE model_versions;
DROP TABLE changes;
DROP TABLE changesets;
DROP TABLE relations;
DROP TABLE elements;
DROP TYPE change_target;
DROP TYPE change_op;
DROP TYPE changeset_status;
DROP TYPE relation_kind;
DROP TYPE element_kind;
