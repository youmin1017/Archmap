-- name: GetSettings :one
SELECT review_policy, updated_by, updated_at FROM settings WHERE id;

-- name: UpdateReviewPolicy :one
UPDATE settings SET review_policy = $1, updated_by = $2, updated_at = now() WHERE id
RETURNING review_policy, updated_by, updated_at;
