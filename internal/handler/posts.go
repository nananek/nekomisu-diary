package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type PostHandler struct {
	db       *sql.DB
	notifier PostNotifier
}

type PostNotifier interface {
	NotifyPost(postID int64, title string)
}

func NewPostHandler(db *sql.DB, n PostNotifier) *PostHandler {
	return &PostHandler{db: db, notifier: n}
}

type postJSON struct {
	ID          int64   `json:"id"`
	AuthorID    int64   `json:"author_id"`
	AuthorName  string  `json:"author_name"`
	AuthorAvatar *string `json:"author_avatar"`
	Title       string  `json:"title"`
	BodyHTML    string  `json:"body_html"`
	Visibility  string  `json:"visibility"`
	PublishedAt *string `json:"published_at"`
	CreatedAt   string  `json:"created_at"`
	CommentCount int    `json:"comment_count"`
}

func (h *PostHandler) List(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	rows, err := h.db.Query(`
		SELECT p.id, p.author_id, u.display_name, u.avatar_path,
		       p.title, p.body_html, p.visibility,
		       p.published_at, p.created_at,
		       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)
		FROM posts p
		JOIN users u ON u.id = p.author_id
		WHERE p.visibility = 'public'
		   OR (p.visibility = 'private' AND p.author_id = $1)
		ORDER BY p.published_at DESC NULLS LAST
		LIMIT $2 OFFSET $3`,
		u.UserID, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	defer rows.Close()

	posts := make([]postJSON, 0)
	for rows.Next() {
		var p postJSON
		var avatar sql.NullString
		var publishedAt, createdAt time.Time
		var pubAtNull sql.NullTime
		if err := rows.Scan(&p.ID, &p.AuthorID, &p.AuthorName, &avatar,
			&p.Title, &p.BodyHTML, &p.Visibility,
			&pubAtNull, &createdAt, &p.CommentCount); err != nil {
			continue
		}
		p.AuthorAvatar = nullStr(avatar)
		p.CreatedAt = createdAt.Format(time.RFC3339)
		if pubAtNull.Valid {
			publishedAt = pubAtNull.Time
			s := publishedAt.Format(time.RFC3339)
			p.PublishedAt = &s
		}
		posts = append(posts, p)
	}

	var total int
	h.db.QueryRow(`
		SELECT COUNT(*) FROM posts
		WHERE visibility = 'public'
		   OR (visibility = 'private' AND author_id = $1)`, u.UserID).Scan(&total)

	writeJSON(w, http.StatusOK, M{
		"posts": posts,
		"total": total,
		"page":  page,
		"pages": (total + limit - 1) / limit,
	})
}

func (h *PostHandler) Get(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	var p postJSON
	var avatar sql.NullString
	var pubAtNull sql.NullTime
	var createdAt time.Time
	err := h.db.QueryRow(`
		SELECT p.id, p.author_id, u.display_name, u.avatar_path,
		       p.title, p.body_html, p.visibility,
		       p.published_at, p.created_at,
		       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)
		FROM posts p
		JOIN users u ON u.id = p.author_id
		WHERE p.id = $1
		  AND (p.visibility = 'public' OR (p.visibility = 'private' AND p.author_id = $2))`,
		id, u.UserID).Scan(&p.ID, &p.AuthorID, &p.AuthorName, &avatar,
		&p.Title, &p.BodyHTML, &p.Visibility,
		&pubAtNull, &createdAt, &p.CommentCount)
	if err != nil {
		writeJSON(w, http.StatusNotFound, M{"error": "not found"})
		return
	}
	p.AuthorAvatar = nullStr(avatar)
	p.CreatedAt = createdAt.Format(time.RFC3339)
	if pubAtNull.Valid {
		s := pubAtNull.Time.Format(time.RFC3339)
		p.PublishedAt = &s
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *PostHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	var req struct {
		Title      string `json:"title"`
		Body       string `json:"body"`
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}
	if req.Title == "" || req.Body == "" {
		writeJSON(w, http.StatusBadRequest, M{"error": "title and body required"})
		return
	}
	if req.Visibility == "" {
		req.Visibility = "public"
	}

	now := time.Now()
	var publishedAt *time.Time
	if req.Visibility != "draft" {
		publishedAt = &now
	}

	var id int64
	err := h.db.QueryRow(`
		INSERT INTO posts (author_id, title, body_html, visibility, published_at)
		VALUES ($1, $2, $3, $4::post_visibility, $5)
		RETURNING id`,
		u.UserID, req.Title, req.Body, req.Visibility, publishedAt,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	// Notify Discord only for public publishes (not private, not draft)
	if h.notifier != nil && req.Visibility == "public" {
		h.notifier.NotifyPost(id, req.Title)
	}
	writeJSON(w, http.StatusCreated, M{"id": id})
}

func (h *PostHandler) Update(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	var authorID int64
	var prevVisibility, prevTitle string
	err := h.db.QueryRow(`SELECT author_id, visibility, title FROM posts WHERE id = $1`, id).Scan(&authorID, &prevVisibility, &prevTitle)
	if err != nil || authorID != u.UserID {
		writeJSON(w, http.StatusForbidden, M{"error": "forbidden"})
		return
	}

	var req struct {
		Title      *string `json:"title"`
		Body       *string `json:"body"`
		Visibility *string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}

	if req.Title != nil {
		h.db.Exec(`UPDATE posts SET title = $1 WHERE id = $2`, *req.Title, id)
	}
	if req.Body != nil {
		h.db.Exec(`UPDATE posts SET body_html = $1 WHERE id = $2`, *req.Body, id)
	}
	newlyPublic := false
	if req.Visibility != nil {
		h.db.Exec(`UPDATE posts SET visibility = $1::post_visibility WHERE id = $2`, *req.Visibility, id)
		if *req.Visibility != "draft" {
			h.db.Exec(`UPDATE posts SET published_at = COALESCE(published_at, NOW()) WHERE id = $1`, id)
		}
		if *req.Visibility == "public" && prevVisibility != "public" {
			newlyPublic = true
		}
	}
	// Notify on first publish (draft/private → public)
	if newlyPublic && h.notifier != nil {
		title := prevTitle
		if req.Title != nil {
			title = *req.Title
		}
		h.notifier.NotifyPost(id, title)
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func (h *PostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	res, err := h.db.Exec(`DELETE FROM posts WHERE id = $1 AND author_id = $2`, id, u.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, http.StatusForbidden, M{"error": "forbidden or not found"})
		return
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func (h *PostHandler) Drafts(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	rows, err := h.db.Query(`
		SELECT p.id, p.author_id, u.display_name, u.avatar_path,
		       p.title, p.body_html, p.visibility,
		       p.published_at, p.created_at,
		       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)
		FROM posts p
		JOIN users u ON u.id = p.author_id
		WHERE p.visibility = 'draft' AND p.author_id = $1
		ORDER BY p.created_at DESC`, u.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	defer rows.Close()
	writeJSON(w, http.StatusOK, M{"posts": scanPosts(rows)})
}

func (h *PostHandler) Search(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, M{"error": "q parameter required"})
		return
	}
	like := "%" + q + "%"
	rows, err := h.db.Query(`
		SELECT p.id, p.author_id, u.display_name, u.avatar_path,
		       p.title, p.body_html, p.visibility,
		       p.published_at, p.created_at,
		       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)
		FROM posts p
		JOIN users u ON u.id = p.author_id
		WHERE (p.visibility = 'public' OR (p.visibility = 'private' AND p.author_id = $1))
		  AND (p.title ILIKE $2 OR p.body_html ILIKE $2)
		ORDER BY p.published_at DESC NULLS LAST
		LIMIT 50`, u.UserID, like)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	defer rows.Close()
	posts := scanPosts(rows)
	writeJSON(w, http.StatusOK, M{"posts": posts, "total": len(posts)})
}

func (h *PostHandler) ByAuthor(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	authorID, _ := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit
	rows, err := h.db.Query(`
		SELECT p.id, p.author_id, u.display_name, u.avatar_path,
		       p.title, p.body_html, p.visibility,
		       p.published_at, p.created_at,
		       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)
		FROM posts p
		JOIN users u ON u.id = p.author_id
		WHERE p.author_id = $1
		  AND (p.visibility = 'public' OR (p.visibility = 'private' AND p.author_id = $2))
		ORDER BY p.published_at DESC NULLS LAST
		LIMIT $3 OFFSET $4`, authorID, u.UserID, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	defer rows.Close()
	posts := scanPosts(rows)
	var total int
	h.db.QueryRow(`SELECT COUNT(*) FROM posts WHERE author_id = $1
		AND (visibility = 'public' OR (visibility = 'private' AND author_id = $2))`,
		authorID, u.UserID).Scan(&total)
	writeJSON(w, http.StatusOK, M{"posts": posts, "total": total, "page": page, "pages": (total + limit - 1) / limit})
}

func scanPosts(rows *sql.Rows) []postJSON {
	posts := make([]postJSON, 0)
	for rows.Next() {
		var p postJSON
		var avatar sql.NullString
		var pubAtNull sql.NullTime
		var createdAt time.Time
		if err := rows.Scan(&p.ID, &p.AuthorID, &p.AuthorName, &avatar,
			&p.Title, &p.BodyHTML, &p.Visibility,
			&pubAtNull, &createdAt, &p.CommentCount); err != nil {
			continue
		}
		p.AuthorAvatar = nullStr(avatar)
		p.CreatedAt = createdAt.Format(time.RFC3339)
		if pubAtNull.Valid {
			s := pubAtNull.Time.Format(time.RFC3339)
			p.PublishedAt = &s
		}
		posts = append(posts, p)
	}
	return posts
}
