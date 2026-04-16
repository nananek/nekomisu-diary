-- name: ListMediaByUploader :many
SELECT id, filename, storage_path, thumbnail_path, mime_type, byte_size, width, height, created_at
FROM media
WHERE uploader_id = $1
ORDER BY created_at DESC
LIMIT 200;

-- name: GetMediaForDelete :one
SELECT storage_path, thumbnail_path
FROM media WHERE id = $1 AND uploader_id = $2;

-- name: DeleteMedia :exec
DELETE FROM media WHERE id = $1;

-- name: CreateMedia :one
INSERT INTO media (uploader_id, filename, storage_path, thumbnail_path, mime_type, byte_size, width, height, attached_post_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;
