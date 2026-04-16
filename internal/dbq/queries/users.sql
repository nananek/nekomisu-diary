-- name: GetUserByLogin :one
SELECT id, login, email, display_name, password_hash, avatar_path
FROM users WHERE login = $1;

-- name: GetUserByID :one
SELECT id, login, email, display_name, avatar_path, created_at
FROM users WHERE id = $1;

-- name: GetUserPasswordHash :one
SELECT password_hash FROM users WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (login, email, display_name, password_hash)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $1 WHERE id = $2;

-- name: UpdateUserDisplayName :exec
UPDATE users SET display_name = $1 WHERE id = $2;

-- name: UpdateUserEmail :exec
UPDATE users SET email = $1 WHERE id = $2;

-- name: UpdateUserAvatar :exec
UPDATE users SET avatar_path = $1 WHERE id = $2;

-- name: ListMembers :many
SELECT u.id, u.login, u.display_name, u.avatar_path, u.created_at,
       (SELECT COUNT(*) FROM posts WHERE author_id = u.id AND visibility != 'draft')::int AS post_count,
       (SELECT COUNT(*) FROM comments WHERE author_id = u.id)::int AS comment_count
FROM users u
ORDER BY u.created_at;

-- name: GetMember :one
SELECT u.id, u.login, u.display_name, u.avatar_path, u.created_at,
       (SELECT COUNT(*) FROM posts WHERE author_id = u.id AND visibility != 'draft')::int AS post_count,
       (SELECT COUNT(*) FROM comments WHERE author_id = u.id)::int AS comment_count
FROM users u WHERE u.id = $1;

-- name: UserHas2FA :one
SELECT
  EXISTS(SELECT 1 FROM totp_secrets ts WHERE ts.user_id = $1 AND ts.verified = true) AS has_totp,
  EXISTS(SELECT 1 FROM webauthn_credentials wc WHERE wc.user_id = $1) AS has_webauthn;
