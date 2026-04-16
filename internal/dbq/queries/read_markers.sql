-- name: GetReadMarker :one
SELECT posts_seen_at FROM read_markers WHERE user_id = $1;

-- name: UpsertReadMarker :exec
INSERT INTO read_markers (user_id, posts_seen_at)
VALUES ($1, NOW())
ON CONFLICT (user_id) DO UPDATE SET posts_seen_at = NOW();
