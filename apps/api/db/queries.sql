-- name: UpsertUser :one
INSERT INTO users (email, name, role, password_hash)
VALUES ($1, $2, $3, $4)
ON CONFLICT (email) DO UPDATE
SET name = EXCLUDED.name, role = EXCLUDED.role, password_hash = EXCLUDED.password_hash
RETURNING id, email, name, role, created_at;

-- name: ListUsers :many
SELECT id, email, name, role, created_at FROM users ORDER BY email;
