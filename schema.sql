-- ねこのみすきー交換日記 / 独自システム スキーマ (PostgreSQL)
-- 設計方針:
--   * クローズドな少人数SNS的日記。投稿・コメント・添付・ユーザーのみを持つ。
--   * カテゴリ/タグはWP上で実質未使用のため移植しない。
--   * 投稿本文は移行時にGutenbergブロック→HTMLへ正規化したものを body_html に格納し、
--     念のため元のブロック形式を body_source に保存（後で再変換可能にする）。
--   * 移行追跡用に wp_*_id を各テーブルに残す（移行後に DROP COLUMN 可能）。

SET client_encoding = 'UTF8';

CREATE TABLE users (
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

CREATE TYPE post_visibility AS ENUM ('public', 'private', 'draft');

CREATE TABLE posts (
  id            BIGSERIAL PRIMARY KEY,
  author_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  title         TEXT        NOT NULL,
  body_html     TEXT        NOT NULL,
  body_source   TEXT,
  visibility    post_visibility NOT NULL DEFAULT 'public',
  published_at  TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  wp_post_id    BIGINT      UNIQUE
);

CREATE INDEX posts_author_published_idx
  ON posts (author_id, published_at DESC NULLS LAST);

CREATE INDEX posts_public_published_idx
  ON posts (published_at DESC NULLS LAST)
  WHERE visibility = 'public';

CREATE TABLE comments (
  id             BIGSERIAL PRIMARY KEY,
  post_id        BIGINT      NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  author_id      BIGINT      REFERENCES users(id) ON DELETE SET NULL,
  author_name    TEXT,                          -- author_id NULL 時の表示用 (匿名コメント)
  body           TEXT        NOT NULL,
  parent_id      BIGINT      REFERENCES comments(id) ON DELETE CASCADE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  wp_comment_id  BIGINT      UNIQUE,
  CONSTRAINT comments_author_or_name CHECK (
    author_id IS NOT NULL OR author_name IS NOT NULL
  )
);

CREATE INDEX comments_post_idx     ON comments (post_id, created_at);
CREATE INDEX comments_parent_idx   ON comments (parent_id) WHERE parent_id IS NOT NULL;

CREATE TABLE media (
  id                BIGSERIAL PRIMARY KEY,
  uploader_id       BIGINT      REFERENCES users(id) ON DELETE SET NULL,
  filename          TEXT        NOT NULL,
  storage_path      TEXT        NOT NULL UNIQUE,  -- e.g. 2025/06/foo.jpg
  mime_type         TEXT        NOT NULL,
  byte_size         BIGINT,
  width             INT,
  height            INT,
  attached_post_id  BIGINT      REFERENCES posts(id) ON DELETE SET NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  wp_attachment_id  BIGINT      UNIQUE
);

CREATE INDEX media_post_idx ON media (attached_post_id) WHERE attached_post_id IS NOT NULL;

CREATE TABLE sessions (
  id          TEXT        PRIMARY KEY,
  user_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  verified    BOOLEAN     NOT NULL DEFAULT true,
  expires_at  TIMESTAMPTZ NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX sessions_user_idx    ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE totp_secrets (
  user_id     BIGINT      PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  secret      TEXT        NOT NULL,
  verified    BOOLEAN     NOT NULL DEFAULT false,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE webauthn_credentials (
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

CREATE INDEX webauthn_user_idx ON webauthn_credentials (user_id);

-- updated_at 自動更新トリガ
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER posts_updated_at BEFORE UPDATE ON posts
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
