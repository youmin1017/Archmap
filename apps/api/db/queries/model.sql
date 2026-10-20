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
INSERT INTO elements (id, parent_id, kind, name, description, technology, tags, protected, layer, projects, replicas, properties,
                      type, environment, lifecycle, criticality, runbook_url, docs_url, last_reviewed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19);

-- name: UpdateElement :exec
UPDATE elements
SET parent_id = $2, kind = $3, name = $4, description = $5, technology = $6,
    tags = $7, protected = $8, layer = $9, projects = $10, replicas = $11, properties = $12,
    type = $13, environment = $14, lifecycle = $15, criticality = $16, runbook_url = $17, docs_url = $18, last_reviewed_at = $19,
    version = version + 1, updated_at = now()
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

-- name: ListOwners :many
SELECT id, kind, name, contact, oncall_url, principal_id, version, created_at, updated_at
FROM owners WHERE deleted_at IS NULL ORDER BY lower(name), id;

-- name: InsertOwner :exec
INSERT INTO owners (id, kind, name, contact, oncall_url, principal_id) VALUES ($1, $2, $3, $4, $5, sqlc.narg('principal_id'));

-- name: UpdateOwner :exec
UPDATE owners
SET kind = $2, name = $3, contact = $4, oncall_url = $5, principal_id = sqlc.narg('principal_id'), version = version + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteOwner :exec
UPDATE owners SET deleted_at = now(), version = version + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListOwnerships :many
SELECT id, element_id, owner_id, role, version, created_at, updated_at
FROM element_owners WHERE deleted_at IS NULL ORDER BY element_id, role;

-- name: InsertOwnership :exec
-- A (element, role) assignment removed earlier is revived under the same id.
INSERT INTO element_owners (id, element_id, owner_id, role) VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE
SET element_id = EXCLUDED.element_id, owner_id = EXCLUDED.owner_id, role = EXCLUDED.role,
    deleted_at = NULL, version = element_owners.version + 1, updated_at = now();

-- name: UpdateOwnership :exec
UPDATE element_owners
SET element_id = $2, owner_id = $3, role = $4, version = version + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteOwnership :exec
UPDATE element_owners SET deleted_at = now(), version = version + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetOwner :one
SELECT id, kind, name, contact, oncall_url, principal_id, version, created_at, updated_at
FROM owners WHERE id = $1 AND deleted_at IS NULL;

-- name: GetOwnership :one
SELECT id, element_id, owner_id, role, version, created_at, updated_at
FROM element_owners WHERE id = $1 AND deleted_at IS NULL;
