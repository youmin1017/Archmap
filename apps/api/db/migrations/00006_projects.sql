-- +goose Up

-- Projects an element is part of (an element may serve several). Free text names, like tags;
-- model data, so changes go through changesets.
ALTER TABLE elements ADD COLUMN projects text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE elements DROP COLUMN projects;
