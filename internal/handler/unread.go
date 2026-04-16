package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/nananek/nekomisu-diary/internal/dbq"
)

type UnreadHandler struct {
	q *dbq.Queries
}

func NewUnreadHandler(db *sql.DB) *UnreadHandler {
	return &UnreadHandler{q: dbq.New(db)}
}

func (h *UnreadHandler) Count(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	seenAt, err := h.q.GetReadMarker(r.Context(), u.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		seenAt = time.Unix(0, 0)
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}

	count, _ := h.q.CountUnreadPublicPosts(r.Context(), dbq.CountUnreadPublicPostsParams{
		AuthorID:    u.UserID,
		PublishedAt: sql.NullTime{Time: seenAt, Valid: true},
	})
	writeJSON(w, http.StatusOK, M{
		"unread":    count,
		"last_seen": seenAt.Format(time.RFC3339),
	})
}

func (h *UnreadHandler) MarkSeen(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	if err := h.q.UpsertReadMarker(r.Context(), u.UserID); err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}
