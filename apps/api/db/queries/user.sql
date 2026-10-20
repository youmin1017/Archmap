-- name: ListUsers :many
SELECT u.id, u.email, u.name, u.role, u.created_at, u.disabled_at,
       u.last_login_at, count(i.id)::integer AS identity_count
FROM users u
LEFT JOIN user_identities i ON i.user_id = u.id
GROUP BY u.id
ORDER BY lower(u.name), u.id;

-- name: GetUser :one
SELECT u.id, u.email, u.name, u.role, u.created_at, u.disabled_at,
       u.last_login_at, count(i.id)::integer AS identity_count
FROM users u
LEFT JOIN user_identities i ON i.user_id = u.id
WHERE u.id = $1
GROUP BY u.id;

-- name: LockMaintainers :many
-- Every operation that can remove maintainer power locks these rows first, so
-- the break-glass invariant (ADR-059) is checked against a stable set.
SELECT u.id, (u.disabled_at IS NULL)::boolean AS enabled,
       EXISTS (SELECT 1 FROM user_identities i
               WHERE i.user_id = u.id AND i.issuer = 'urn:archmap:local')::boolean AS has_local
FROM users u
WHERE u.role = 'maintainer'
ORDER BY u.id
FOR UPDATE OF u;

-- name: GetUserForUpdate :one
SELECT id, email, role, disabled_at FROM users WHERE id = $1 FOR UPDATE;

-- name: SetUserRole :exec
UPDATE users SET role = $2 WHERE id = $1;

-- name: SetUserName :exec
UPDATE users SET name = $2 WHERE id = $1;

-- name: SetUserDisabled :exec
UPDATE users SET disabled_at = sqlc.narg('disabled_at') WHERE id = $1;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: CreateUser :one
INSERT INTO users (email, name, role)
VALUES (sqlc.narg('email'), $1, $2)
RETURNING id;

-- name: CreateLocalIdentity :one
INSERT INTO user_identities (user_id, provider, issuer, subject, email, password_hash, password_must_change)
VALUES ($1, 'local', 'urn:archmap:local', $2, $3, $4, $5)
RETURNING id;

-- name: CreateExternalIdentity :one
INSERT INTO user_identities (user_id, provider, issuer, subject)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, provider, issuer, subject, email, created_at, last_login_at;

-- name: GetLocalIdentityBySubject :one
SELECT id, user_id FROM user_identities
WHERE issuer = 'urn:archmap:local' AND subject = $1;

-- name: GetLocalIdentity :one
SELECT id FROM user_identities
WHERE user_id = $1 AND issuer = 'urn:archmap:local';

-- name: SetLocalPassword :exec
UPDATE user_identities SET password_hash = $2, password_must_change = $3 WHERE id = $1;

-- name: ListIdentities :many
SELECT id, user_id, provider, issuer, subject, email, created_at, last_login_at
FROM user_identities
WHERE user_id = $1
ORDER BY created_at, id;

-- name: GetIdentityForUpdate :one
SELECT id, user_id, issuer FROM user_identities WHERE id = $1 AND user_id = $2 FOR UPDATE;

-- name: DeleteIdentity :exec
DELETE FROM user_identities WHERE id = $1;

-- name: ListGroups :many
SELECT g.id, g.name, g.description, g.created_at, count(m.user_id)::integer AS member_count
FROM groups g
LEFT JOIN group_members m ON m.group_id = g.id
WHERE g.deleted_at IS NULL
GROUP BY g.id
ORDER BY lower(g.name), g.id;

-- name: ListGroupMembers :many
SELECT m.group_id, m.user_id
FROM group_members m
JOIN groups g ON g.id = m.group_id
WHERE g.deleted_at IS NULL
ORDER BY m.group_id, m.user_id;

-- name: CreateGroup :one
INSERT INTO groups (name, description) VALUES ($1, $2) RETURNING id;

-- name: GetGroupForUpdate :one
SELECT id, name, description FROM groups WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: UpdateGroup :exec
UPDATE groups SET name = $2, description = $3 WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteGroup :exec
UPDATE groups SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;

-- name: ClearGroupMembers :exec
DELETE FROM group_members WHERE group_id = $1;

-- name: AddGroupMember :exec
INSERT INTO group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListPrincipals :many
-- Every user and group (deleted groups included) for owner links (ADR-069).
SELECT id, 'user'::text AS kind, name, (disabled_at IS NOT NULL)::boolean AS inactive FROM users
UNION ALL
SELECT id, 'group'::text AS kind, name, (deleted_at IS NOT NULL)::boolean AS inactive FROM groups;

-- name: ListDirectoryUsers :many
SELECT id, name, email, (disabled_at IS NOT NULL)::boolean AS disabled FROM users ORDER BY lower(name), id;
