-- name: ListElements :many
SELECT * FROM elements
WHERE deleted_at IS NULL
  AND (sqlc.narg('parent_id')::uuid IS NULL OR parent_id = sqlc.narg('parent_id')::uuid)
  AND (sqlc.narg('q')::text IS NULL OR name ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY name, id;

-- name: GetElement :one
SELECT * FROM elements WHERE id = $1 AND deleted_at IS NULL;

-- name: ListRelations :many
SELECT * FROM relations
WHERE deleted_at IS NULL
  AND (sqlc.narg('source_id')::uuid IS NULL OR source_id = sqlc.narg('source_id')::uuid)
  AND (sqlc.narg('target_id')::uuid IS NULL OR target_id = sqlc.narg('target_id')::uuid)
ORDER BY created_at, id;

-- name: GetRelation :one
SELECT * FROM relations WHERE id = $1 AND deleted_at IS NULL;

-- name: InsertElement :exec
INSERT INTO elements (id, parent_id, kind, name, description, technology, tags, protected, layer, projects, replicas, properties)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: UpdateElement :exec
UPDATE elements
SET parent_id = $2, kind = $3, name = $4, description = $5, technology = $6,
    tags = $7, protected = $8, layer = $9, projects = $10, replicas = $11, properties = $12, version = version + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteElement :exec
UPDATE elements
SET deleted_at = now(), version = version + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: InsertRelation :exec
INSERT INTO relations (id, source_id, target_id, kind, description, technology)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: UpdateRelation :exec
UPDATE relations
SET source_id = $2, target_id = $3, kind = $4, description = $5, technology = $6,
    version = version + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteRelation :exec
UPDATE relations
SET deleted_at = now(), version = version + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;
