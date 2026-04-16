-- name: GetPost :one
SELECT p.id, p.author_id, u.display_name AS author_name, u.avatar_path AS author_avatar,
       p.title, p.body_html, p.body_md, p.visibility,
       p.published_at, p.created_at,
       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)::int AS comment_count
FROM posts p
JOIN users u ON u.id = p.author_id
WHERE p.id = $1
  AND (p.visibility = 'public' OR (p.visibility = 'private' AND p.author_id = $2));

-- name: GetPostForAuthorization :one
SELECT author_id, visibility, title FROM posts WHERE id = $1;

-- name: ListPosts :many
SELECT p.id, p.author_id, u.display_name AS author_name, u.avatar_path AS author_avatar,
       p.title, p.body_html, p.visibility,
       p.published_at, p.created_at,
       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)::int AS comment_count
FROM posts p
JOIN users u ON u.id = p.author_id
WHERE p.visibility = 'public'
   OR (p.visibility = 'private' AND p.author_id = $1)
ORDER BY p.published_at DESC NULLS LAST
LIMIT $2 OFFSET $3;

-- name: CountVisiblePosts :one
SELECT COUNT(*)::int FROM posts
WHERE visibility = 'public'
   OR (visibility = 'private' AND author_id = $1);

-- name: ListPostsByAuthor :many
SELECT p.id, p.author_id, u.display_name AS author_name, u.avatar_path AS author_avatar,
       p.title, p.body_html, p.visibility,
       p.published_at, p.created_at,
       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)::int AS comment_count
FROM posts p
JOIN users u ON u.id = p.author_id
WHERE p.author_id = $1
  AND (p.visibility = 'public' OR (p.visibility = 'private' AND p.author_id = $2))
ORDER BY p.published_at DESC NULLS LAST
LIMIT $3 OFFSET $4;

-- name: CountPostsByAuthor :one
SELECT COUNT(*)::int FROM posts
WHERE author_id = $1
  AND (visibility = 'public' OR (visibility = 'private' AND author_id = $2));

-- name: ListDrafts :many
SELECT p.id, p.author_id, u.display_name AS author_name, u.avatar_path AS author_avatar,
       p.title, p.body_html, p.visibility,
       p.published_at, p.created_at,
       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)::int AS comment_count
FROM posts p
JOIN users u ON u.id = p.author_id
WHERE p.visibility = 'draft' AND p.author_id = $1
ORDER BY p.created_at DESC;

-- name: SearchPosts :many
SELECT p.id, p.author_id, u.display_name AS author_name, u.avatar_path AS author_avatar,
       p.title, p.body_html, p.visibility,
       p.published_at, p.created_at,
       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)::int AS comment_count
FROM posts p
JOIN users u ON u.id = p.author_id
WHERE (p.visibility = 'public' OR (p.visibility = 'private' AND p.author_id = $1))
  AND (p.title ILIKE $2 OR p.body_html ILIKE $2)
ORDER BY p.published_at DESC NULLS LAST
LIMIT 50;

-- name: CreatePost :one
INSERT INTO posts (author_id, title, body_html, body_md, visibility, published_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: UpdatePostTitle :exec
UPDATE posts SET title = $1 WHERE id = $2;

-- name: UpdatePostBody :exec
UPDATE posts SET body_html = $1 WHERE id = $2;

-- name: UpdatePostBodyMD :exec
UPDATE posts SET body_md = $1 WHERE id = $2;

-- name: UpdatePostVisibility :exec
UPDATE posts SET visibility = $1, published_at = COALESCE(published_at, CASE WHEN $1 = 'draft' THEN NULL ELSE NOW() END) WHERE id = $2;

-- name: DeletePost :execrows
DELETE FROM posts WHERE id = $1 AND author_id = $2;

-- name: CountUnreadPublicPosts :one
SELECT COUNT(*)::int FROM posts
WHERE visibility = 'public'
  AND author_id != $1
  AND COALESCE(published_at, created_at) > $2;
