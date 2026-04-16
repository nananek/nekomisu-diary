-- +goose Up
-- Initial schema. Idempotent so existing databases (dev/staging that
-- predate this migration framework) can safely be brought under management
-- without failing.

SET client_encoding = 'UTF8';

CREATE TABLE IF NOT EXISTS users (
  id            BIGSERIAL PRIMARY KEY,
  login         TEXT        NOT NULL UNIQUE,
  email         TEXT        NOT NULL UNIQUE,
  display_name  TEXT        NOT NULL,
  password_hash TEXT        NOT NULL,
  avatar_path   TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  wp_user_id    BIGINT      UNIQUE
);

-- +goose StatementBegin
DO $$ BEGIN
  CREATE TYPE post_visibility AS ENUM ('public', 'private', 'draft');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $func$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$func$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS posts (
  id            BIGSERIAL PRIMARY KEY,
  author_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  title         TEXT        NOT NULL,
  body_html     TEXT        NOT NULL,
  body_md       TEXT,
  body_source   TEXT,
  visibility    post_visibility NOT NULL DEFAULT 'public',
  published_at  TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  wp_post_id    BIGINT      UNIQUE
);
-- Catch older DBs that predate body_md
ALTER TABLE posts ADD COLUMN IF NOT EXISTS body_md TEXT;
CREATE INDEX IF NOT EXISTS posts_author_published_idx
  ON posts (author_id, published_at DESC NULLS LAST);
CREATE INDEX IF NOT EXISTS posts_public_published_idx
  ON posts (published_at DESC NULLS LAST)
  WHERE visibility = 'public';

CREATE TABLE IF NOT EXISTS comments (
  id             BIGSERIAL PRIMARY KEY,
  post_id        BIGINT      NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  author_id      BIGINT      REFERENCES users(id) ON DELETE SET NULL,
  author_name    TEXT,
  body           TEXT        NOT NULL,
  parent_id      BIGINT      REFERENCES comments(id) ON DELETE CASCADE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  wp_comment_id  BIGINT      UNIQUE,
  CONSTRAINT comments_author_or_name CHECK (
    author_id IS NOT NULL OR author_name IS NOT NULL
  )
);
CREATE INDEX IF NOT EXISTS comments_post_idx   ON comments (post_id, created_at);
CREATE INDEX IF NOT EXISTS comments_parent_idx ON comments (parent_id) WHERE parent_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS media (
  id                BIGSERIAL PRIMARY KEY,
  uploader_id       BIGINT      REFERENCES users(id) ON DELETE SET NULL,
  filename          TEXT        NOT NULL,
  storage_path      TEXT        NOT NULL UNIQUE,
  thumbnail_path    TEXT,
  mime_type         TEXT        NOT NULL,
  byte_size         BIGINT,
  width             INT,
  height            INT,
  attached_post_id  BIGINT      REFERENCES posts(id) ON DELETE SET NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  wp_attachment_id  BIGINT      UNIQUE
);
ALTER TABLE media ADD COLUMN IF NOT EXISTS thumbnail_path TEXT;
CREATE INDEX IF NOT EXISTS media_post_idx ON media (attached_post_id) WHERE attached_post_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS sessions (
  id          TEXT        PRIMARY KEY,
  user_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  verified    BOOLEAN     NOT NULL DEFAULT true,
  expires_at  TIMESTAMPTZ NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS verified BOOLEAN NOT NULL DEFAULT true;
CREATE INDEX IF NOT EXISTS sessions_user_idx    ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions (expires_at);

CREATE TABLE IF NOT EXISTS read_markers (
  user_id       BIGINT      PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  posts_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS totp_secrets (
  user_id     BIGINT      PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  secret      TEXT        NOT NULL,
  verified    BOOLEAN     NOT NULL DEFAULT false,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS webauthn_credentials (
  id              TEXT        PRIMARY KEY,
  user_id         BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name            TEXT        NOT NULL DEFAULT '',
  public_key      BYTEA       NOT NULL,
  attestation_type TEXT,
  aaguid          BYTEA,
  sign_count      BIGINT      NOT NULL DEFAULT 0,
  transports      TEXT[],
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS webauthn_user_idx ON webauthn_credentials (user_id);

DROP TRIGGER IF EXISTS users_updated_at ON users;
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS posts_updated_at ON posts;
CREATE TRIGGER posts_updated_at BEFORE UPDATE ON posts
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS webauthn_credentials;
DROP TABLE IF EXISTS totp_secrets;
DROP TABLE IF EXISTS read_markers;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS media;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS users;
DROP TYPE IF EXISTS post_visibility;
DROP FUNCTION IF EXISTS set_updated_at();
