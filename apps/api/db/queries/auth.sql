-- name: GetIdentity :one
SELECT i.id, i.user_id, i.password_hash, i.password_must_change,
       u.disabled_at, u.role
FROM user_identities i
JOIN users u ON u.id = i.user_id
WHERE i.issuer = $1 AND i.subject = $2;

-- name: CreateProvisionedUser :one
INSERT INTO users (email, name, role)
VALUES (sqlc.narg('email'), $1, $2)
RETURNING id;

-- name: CreateIdentity :one
INSERT INTO user_identities (user_id, provider, issuer, subject, email)
VALUES ($1, $2, $3, $4, sqlc.narg('email'))
RETURNING id;

-- name: TouchUserLogin :exec
UPDATE users SET last_login_at = $2 WHERE id = $1;

-- name: TouchIdentityLogin :exec
UPDATE user_identities
SET last_login_at = $2, email = coalesce(sqlc.narg('email'), email)
WHERE id = $1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, identity_id, provider, csrf_token,
                      created_at, last_seen_at, idle_expires_at, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $8, $9, $10);

-- name: GetSession :one
-- Disabled users and expired sessions resolve to nothing (ADR-055).
SELECT s.user_id, s.provider, s.csrf_token, s.last_seen_at, s.expires_at,
       u.email, u.name, u.role,
       coalesce(i.password_must_change, false)::boolean AS password_must_change
FROM sessions s
JOIN users u ON u.id = s.user_id
LEFT JOIN user_identities i ON i.id = s.identity_id
WHERE s.token_hash = $1
  AND u.disabled_at IS NULL
  AND s.idle_expires_at > sqlc.arg('now')
  AND s.expires_at > sqlc.arg('now');

-- name: TouchSession :exec
UPDATE sessions
SET last_seen_at = sqlc.arg('now'), idle_expires_at = LEAST(sqlc.arg('idle_expires_at'), expires_at)
WHERE token_hash = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteOtherSessions :exec
DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE LEAST(idle_expires_at, expires_at) <= $1;

-- name: GetLocalPassword :one
SELECT id, password_hash FROM user_identities
WHERE user_id = $1 AND issuer = 'urn:archmap:local';

-- name: SetUserRole :exec
UPDATE users SET role = $2 WHERE id = $1;

-- name: SetPassword :exec
UPDATE user_identities SET password_hash = $2, password_must_change = false WHERE id = $1;

-- name: CreateFlow :exec
INSERT INTO auth_flows (id, provider, state, nonce, pkce_verifier, return_to, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: TakeFlow :one
-- A flow is single use: reading it deletes it.
DELETE FROM auth_flows WHERE id = $1
RETURNING provider, state, nonce, pkce_verifier, return_to, expires_at;

-- name: DeleteExpiredFlows :execrows
DELETE FROM auth_flows WHERE expires_at <= $1;
