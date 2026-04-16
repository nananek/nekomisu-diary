package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/nananek/nekomisu-diary/internal/dbq"
)

type CommentHandler struct {
	q        *dbq.Queries
	notifier CommentNotifier
}

type CommentNotifier interface {
	NotifyComment(postID, commentID int64, postTitle, author, body string)
}

func NewCommentHandler(db *sql.DB, n CommentNotifier) *CommentHandler {
	return &CommentHandler{q: dbq.New(db), notifier: n}
}

type commentJSON struct {
	ID           int64   `json:"id"`
	PostID       int64   `json:"post_id"`
	AuthorID     *int64  `json:"author_id"`
	AuthorName   string  `json:"author_name"`
	AuthorAvatar *string `json:"author_avatar"`
	Body         string  `json:"body"`
	ParentID     *int64  `json:"parent_id"`
	CreatedAt    string  `json:"created_at"`
}

func (h *CommentHandler) List(w http.ResponseWriter, r *http.Request) {
	postID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	rows, err := h.q.ListComments(r.Context(), postID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}

	comments := make([]commentJSON, 0, len(rows))
	for _, rr := range rows {
		c := commentJSON{
			ID:           rr.ID,
			PostID:       rr.PostID,
			Body:         rr.Body,
			AuthorAvatar: nullStr(rr.AuthorAvatar),
			CreatedAt:    rr.CreatedAt.Format(time.RFC3339),
		}
		if rr.AuthorID.Valid {
			c.AuthorID = &rr.AuthorID.Int64
		}
		// AuthorName column in the query is COALESCE(...) so it's always a string.
		c.AuthorName = rr.AuthorName
		if rr.ParentID.Valid {
			c.ParentID = &rr.ParentID.Int64
		}
		comments = append(comments, c)
	}
	writeJSON(w, http.StatusOK, M{"comments": comments})
}

func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	postID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	postTitle, err := h.q.GetPostTitle(r.Context(), postID)
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

	var parent sql.NullInt64
	if req.ParentID != nil {
		parent = sql.NullInt64{Int64: *req.ParentID, Valid: true}
	}
	id, err := h.q.CreateComment(r.Context(), dbq.CreateCommentParams{
		PostID:   postID,
		AuthorID: sql.NullInt64{Int64: u.UserID, Valid: true},
		Body:     req.Body,
		ParentID: parent,
	})
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

	n, err := h.q.DeleteComment(r.Context(), dbq.DeleteCommentParams{
		ID:       commentID,
		AuthorID: sql.NullInt64{Int64: u.UserID, Valid: true},
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	if n == 0 {
		writeJSON(w, http.StatusForbidden, M{"error": "forbidden or not found"})
		return
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}
