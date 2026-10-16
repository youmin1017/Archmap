-- +goose Up

CREATE TYPE view_kind AS ENUM ('landscape', 'context', 'container', 'component', 'deployment', 'custom');

-- Views are saved perspectives over the model. They are NOT part of the
-- changeset workflow: writing them creates no changes and no model versions.
CREATE TABLE views (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    name             text NOT NULL CHECK (btrim(name) <> ''),
    kind             view_kind NOT NULL,
    -- No foreign key: the scope may be an element that only exists in a draft.
    scope_element_id uuid,
    description      text NOT NULL DEFAULT '',
    created_by       uuid NOT NULL REFERENCES users (id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- LAYOUT only (plus custom-view membership). Rows may reference elements that
-- do not exist in the model being shown (e.g. draft-only elements): readers ignore them.
CREATE TABLE view_elements (
    view_id    uuid NOT NULL REFERENCES views (id) ON DELETE CASCADE,
    element_id uuid NOT NULL,
    x          double precision,
    y          double precision,
    w          double precision,
    h          double precision,
    collapsed  boolean NOT NULL DEFAULT true,
    -- Custom views: true marks the element as a member. Ignored for other kinds.
    included   boolean NOT NULL DEFAULT false,
    PRIMARY KEY (view_id, element_id)
);

-- +goose Down
DROP TABLE view_elements;
DROP TABLE views;
DROP TYPE view_kind;
