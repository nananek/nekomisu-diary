-- name: GetUserIDByMisskeyAccount :one
SELECT user_id FROM misskey_links
WHERE misskey_instance = $1 AND misskey_user_id = $2;
