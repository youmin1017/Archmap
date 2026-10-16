-- name: InsertView :one
INSERT INTO views (name, kind, scope_element_id, description, created_by, layers)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetView :one
SELECT * FROM views WHERE id = $1;

-- name: ListViews :many
SELECT * FROM views ORDER BY created_at, id;

-- name: UpdateView :one
UPDATE views
SET name = $2, kind = $3, scope_element_id = $4, description = $5, layers = $6, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteView :execrows
DELETE FROM views WHERE id = $1;

-- name: ListViewElements :many
SELECT * FROM view_elements WHERE view_id = $1 ORDER BY element_id;

-- name: ResetViewLayout :exec
UPDATE view_elements
SET x = NULL, y = NULL, w = NULL, h = NULL, collapsed = true
WHERE view_id = $1;

-- name: UpsertViewLayout :exec
INSERT INTO view_elements (view_id, element_id, x, y, w, h, collapsed)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (view_id, element_id) DO UPDATE
SET x = EXCLUDED.x, y = EXCLUDED.y, w = EXCLUDED.w, h = EXCLUDED.h, collapsed = EXCLUDED.collapsed;

-- name: ClearViewMembers :exec
UPDATE view_elements SET included = false WHERE view_id = $1;

-- name: UpsertViewMember :exec
INSERT INTO view_elements (view_id, element_id, included)
VALUES ($1, $2, true)
ON CONFLICT (view_id, element_id) DO UPDATE SET included = true;

-- name: PruneViewElements :exec
DELETE FROM view_elements
WHERE view_id = $1 AND NOT included AND x IS NULL AND y IS NULL AND w IS NULL AND h IS NULL AND collapsed;
