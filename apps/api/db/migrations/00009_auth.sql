-- +goose Up

-- users is the internal account only (ADR-053). Email is a contact attribute:
-- nullable (an external identity may not provide one) and not unique (two
-- accounts may share it; accounts are never linked by email).
ALTER TABLE users
    ADD COLUMN disabled_at timestamptz,
    ADD COLUMN last_login_at timestamptz,
    ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users DROP CONSTRAINT users_email_key;

-- Ways to sign in. (issuer, subject) is the only lookup key. Local identities
-- use issuer 'urn:archmap:local' and the normalized login email as subject.
CREATE TABLE user_identities (
    id                   uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id              uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider             text NOT NULL,
    issuer               text NOT NULL,
    subject              text NOT NULL,
    email                text,
    password_hash        text,
    password_must_change boolean NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now(),
    last_login_at        timestamptz,
    UNIQUE (issuer, subject),
    CHECK ((issuer = 'urn:archmap:local') = (password_hash IS NOT NULL))
);
CREATE INDEX user_identities_user_id_idx ON user_identities (user_id);

-- Server-side sessions (ADR-055). Only sha256(token) is stored.
CREATE TABLE sessions (
    token_hash      bytea PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    identity_id     uuid REFERENCES user_identities (id) ON DELETE CASCADE,
    provider        text NOT NULL,
    csrf_token      text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    idle_expires_at timestamptz NOT NULL,
    expires_at      timestamptz NOT NULL,
    user_agent      text NOT NULL DEFAULT '',
    ip              text NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (LEAST(idle_expires_at, expires_at));

-- In-flight redirect logins (OIDC later; the fake provider in tests).
CREATE TABLE auth_flows (
    id            text PRIMARY KEY,
    provider      text NOT NULL,
    state         text NOT NULL,
    nonce         text NOT NULL DEFAULT '',
    pkce_verifier text NOT NULL DEFAULT '',
    return_to     text NOT NULL,
    expires_at    timestamptz NOT NULL
);

-- Every existing account keeps its password as a local identity.
INSERT INTO user_identities (user_id, provider, issuer, subject, email, password_hash)
SELECT id, 'local', 'urn:archmap:local', lower(trim(email)), email, password_hash FROM users;

ALTER TABLE users DROP COLUMN password_hash;

-- +goose Down
ALTER TABLE users ADD COLUMN password_hash text;
UPDATE users u SET password_hash = i.password_hash
FROM user_identities i
WHERE i.user_id = u.id AND i.issuer = 'urn:archmap:local';
-- Accounts without a local password cannot exist in the old schema.
UPDATE users SET password_hash = '!' WHERE password_hash IS NULL;
UPDATE users SET email = id::text || '@unknown.invalid' WHERE email IS NULL;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
ALTER TABLE users ALTER COLUMN email SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email); -- fails if two accounts share an email
DROP TABLE auth_flows;
DROP TABLE sessions;
DROP TABLE user_identities;
ALTER TABLE users DROP COLUMN last_login_at, DROP COLUMN disabled_at;
