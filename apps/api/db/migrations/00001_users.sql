-- +goose Up
CREATE TYPE user_role AS ENUM ('viewer', 'editor', 'maintainer');

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    email         text NOT NULL UNIQUE,
    name          text NOT NULL,
    role          user_role NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;
DROP TYPE user_role;
