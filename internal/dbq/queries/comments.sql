-- name: ListComments :many
SELECT c.id, c.post_id, c.author_id,
       COALESCE(u.display_name, c.author_name, 'Anonymous') AS author_name,
       u.avatar_path AS author_avatar,
       c.body, c.parent_id, c.created_at
FROM comments c
LEFT JOIN users u ON u.id = c.author_id
WHERE c.post_id = $1
ORDER BY c.created_at ASC;

-- name: CreateComment :one
INSERT INTO comments (post_id, author_id, body, parent_id)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: DeleteComment :execrows
DELETE FROM comments WHERE id = $1 AND author_id = $2;

-- name: GetPostTitle :one
SELECT title FROM posts WHERE id = $1;
