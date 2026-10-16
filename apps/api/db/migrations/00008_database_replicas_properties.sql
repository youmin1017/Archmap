-- +goose Up

-- A datastore is its own element kind (drawn as a cylinder, may be deployed on a node).
ALTER TYPE element_kind ADD VALUE 'database';

-- How many instances / replicas the element stands for (1 = a single one).
ALTER TABLE elements ADD COLUMN replicas integer NOT NULL DEFAULT 1;

-- Free-form facts such as ip, port or namespace (key -> value).
ALTER TABLE elements ADD COLUMN properties jsonb NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE elements DROP COLUMN properties;
ALTER TABLE elements DROP COLUMN replicas;
-- (the 'database' enum value cannot be removed from element_kind; it is harmless)
