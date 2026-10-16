-- name: ListLayerRules :many
SELECT * FROM layer_rules ORDER BY element_class;

-- name: DeleteLayerRules :exec
DELETE FROM layer_rules;

-- name: InsertLayerRule :exec
INSERT INTO layer_rules (element_class, layers) VALUES ($1, $2);
