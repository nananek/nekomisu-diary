-- name: GetMisskeyLink :one
SELECT user_id, last_miauth_token FROM misskey_links
WHERE misskey_instance = $1 AND misskey_user_id = $2;

-- name: SetMisskeyLinkToken :exec
UPDATE misskey_links
SET last_miauth_token = $3
WHERE misskey_instance = $1 AND misskey_user_id = $2;
