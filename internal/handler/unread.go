package handler

import (
	"database/sql"
	"net/http"
	"time"
)

type UnreadHandler struct {
	db *sql.DB
}

func NewUnreadHandler(db *sql.DB) *UnreadHandler {
	return &UnreadHandler{db: db}
}

// Count returns the number of unread public posts for the current user
// (posts by others, published after their last seen timestamp).
func (h *UnreadHandler) Count(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	var seenAt time.Time
	err := h.db.QueryRow(`SELECT posts_seen_at FROM read_markers WHERE user_id = $1`, u.UserID).Scan(&seenAt)
	if err == sql.ErrNoRows {
		seenAt = time.Unix(0, 0)
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}

	var count int
	h.db.QueryRow(`
		SELECT COUNT(*) FROM posts
		WHERE visibility = 'public'
		  AND author_id != $1
		  AND COALESCE(published_at, created_at) > $2`,
		u.UserID, seenAt,
	).Scan(&count)

	writeJSON(w, http.StatusOK, M{
		"unread":      count,
		"last_seen":   seenAt.Format(time.RFC3339),
	})
}

// MarkSeen updates the user's seen timestamp to now.
func (h *UnreadHandler) MarkSeen(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	_, err := h.db.Exec(`
		INSERT INTO read_markers (user_id, posts_seen_at) VALUES ($1, NOW())
		ON CONFLICT (user_id) DO UPDATE SET posts_seen_at = NOW()`,
		u.UserID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}
