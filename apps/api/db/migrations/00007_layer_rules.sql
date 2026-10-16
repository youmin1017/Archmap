-- +goose Up

-- Which layers an element class may be placed on. element_class is "database" (a datastore
-- container / component) or an element kind. An element on a layer outside its class's list is
-- rejected when a change sets its layer; classes without a row are unrestricted.
CREATE TABLE layer_rules (
    element_class text PRIMARY KEY,
    layers        text[] NOT NULL
);

-- +goose Down
DROP TABLE layer_rules;
