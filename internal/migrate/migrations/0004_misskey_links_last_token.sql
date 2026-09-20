-- +goose Up
-- Tracks the Misskey access token issued by the most recent successful
-- MiAuth login for this link, so the next login can self-revoke it on the
-- Misskey side before storing the new one. Without this, every "Misskeyで
-- ログイン" approval leaves its token alive in Misskey's access_token
-- table forever, since MiAuth mints a fresh token per approval and Misskey
-- itself never expires or dedupes them.
ALTER TABLE misskey_links ADD COLUMN IF NOT EXISTS last_miauth_token TEXT;

-- +goose Down
ALTER TABLE misskey_links DROP COLUMN IF EXISTS last_miauth_token;
