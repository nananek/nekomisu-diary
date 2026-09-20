-- +goose Up
-- MiAuth login: each diary account can be linked to at most one Misskey
-- account, and each Misskey account can be linked to at most one diary
-- account. Linking is admin-only (see cmd/miauthlink), never self-service,
-- so there is no "pending"/"unverified" state here unlike totp_secrets.

CREATE TABLE IF NOT EXISTS misskey_links (
  id                BIGSERIAL   PRIMARY KEY,
  user_id           BIGINT      NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  misskey_instance  TEXT        NOT NULL,
  misskey_user_id   TEXT        NOT NULL,
  misskey_username  TEXT        NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (misskey_instance, misskey_user_id)
);

-- +goose Down
DROP TABLE IF EXISTS misskey_links;
