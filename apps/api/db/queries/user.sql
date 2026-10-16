-- name: UpsertUser :one
INSERT INTO users (email, name, role, password_hash)
VALUES ($1, $2, $3, $4)
ON CONFLICT (email) DO UPDATE
SET name = EXCLUDED.name, role = EXCLUDED.role, password_hash = EXCLUDED.password_hash
RETURNING id, email, name, role, created_at;

-- name: ListUsers :many
SELECT id, email, name, role, created_at FROM users ORDER BY email;

-- name: GetUserByEmail :one
SELECT id, email, name, role, created_at FROM users WHERE email = $1;

-- name: LockMaintainers :many
SELECT id FROM users WHERE role = 'maintainer' ORDER BY id FOR UPDATE;

-- name: GetUserForUpdate :one
SELECT id, email, name, role, created_at FROM users WHERE id = $1 FOR UPDATE;

-- name: SetUserRole :one
UPDATE users SET role = $2 WHERE id = $1
RETURNING id, email, name, role, created_at;
