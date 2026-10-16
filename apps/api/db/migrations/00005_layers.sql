-- +goose Up

-- Layer an element belongs to in layered (3D) views, e.g. "User", "Services", "Kubernetes", "Physical".
-- Free text; an empty string means "unassigned". It is model data (changes go through changesets).
ALTER TABLE elements ADD COLUMN layer text NOT NULL DEFAULT '';

-- Layered views: ordered layer names, top to bottom.
ALTER TYPE view_kind ADD VALUE 'layers';
ALTER TABLE views ADD COLUMN layers text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE views DROP COLUMN layers;
ALTER TABLE elements DROP COLUMN layer;
-- (the 'layers' enum value cannot be removed from view_kind; it is harmless)
