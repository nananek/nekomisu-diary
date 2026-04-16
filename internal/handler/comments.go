package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type CommentHandler struct {
	db       *sql.DB
	notifier CommentNotifier
}

type CommentNotifier interface {
	NotifyComment(postID, commentID int64, postTitle, author, body string)
}

func NewCommentHandler(db *sql.DB, n CommentNotifier) *CommentHandler {
	return &CommentHandler{db: db, notifier: n}
}

type commentJSON struct {
	ID          int64   `json:"id"`
	PostID      int64   `json:"post_id"`
	AuthorID    *int64  `json:"author_id"`
	AuthorName  string  `json:"author_name"`
	AuthorAvatar *string `json:"author_avatar"`
	Body        string  `json:"body"`
	ParentID    *int64  `json:"parent_id"`
	CreatedAt   string  `json:"created_at"`
}

func (h *CommentHandler) List(w http.ResponseWriter, r *http.Request) {
	postID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	rows, err := h.db.Query(`
		SELECT c.id, c.post_id, c.author_id,
		       COALESCE(u.display_name, c.author_name, 'Anonymous') AS author_name,
		       u.avatar_path,
		       c.body, c.parent_id, c.created_at
		FROM comments c
		LEFT JOIN users u ON u.id = c.author_id
		WHERE c.post_id = $1
		ORDER BY c.created_at ASC`, postID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	defer rows.Close()

	comments := make([]commentJSON, 0)
	for rows.Next() {
		var c commentJSON
		var avatar sql.NullString
		var parentID sql.NullInt64
		var authorID sql.NullInt64
		var createdAt time.Time
		if err := rows.Scan(&c.ID, &c.PostID, &authorID, &c.AuthorName, &avatar,
			&c.Body, &parentID, &createdAt); err != nil {
			continue
		}
		if authorID.Valid {
			c.AuthorID = &authorID.Int64
		}
		c.AuthorAvatar = nullStr(avatar)
		if parentID.Valid {
			c.ParentID = &parentID.Int64
		}
		c.CreatedAt = createdAt.Format(time.RFC3339)
		comments = append(comments, c)
	}
	writeJSON(w, http.StatusOK, M{"comments": comments})
}

func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	postID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	var postTitle string
	err := h.db.QueryRow(`SELECT title FROM posts WHERE id = $1`, postID).Scan(&postTitle)
	if err != nil {
		writeJSON(w, http.StatusNotFound, M{"error": "post not found"})
		return
	}

	var req struct {
		Body     string `json:"body"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Body == "" {
		writeJSON(w, http.StatusBadRequest, M{"error": "body required"})
		return
	}

	var id int64
	err = h.db.QueryRow(`
		INSERT INTO comments (post_id, author_id, body, parent_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		postID, u.UserID, req.Body, req.ParentID,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	if h.notifier != nil {
		h.notifier.NotifyComment(postID, id, postTitle, u.DisplayName, req.Body)
	}
	writeJSON(w, http.StatusCreated, M{"id": id})
}

func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	commentID, _ := strconv.ParseInt(r.PathValue("commentId"), 10, 64)

	res, err := h.db.Exec(`DELETE FROM comments WHERE id = $1 AND author_id = $2`, commentID, u.UserID)
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
