package handler

import (
	"database/sql"
	"net/http"
)

type MemberHandler struct {
	db *sql.DB
}

func NewMemberHandler(db *sql.DB) *MemberHandler {
	return &MemberHandler{db: db}
}

func (h *MemberHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("userId")
	var (
		uid        int64
		login      string
		dispName   string
		createdAt  string
		avatar     sql.NullString
		posts      int
		comments   int
	)
	err := h.db.QueryRow(`
		SELECT u.id, u.login, u.display_name, u.avatar_path, u.created_at,
		       (SELECT COUNT(*) FROM posts WHERE author_id = u.id AND visibility != 'draft'),
		       (SELECT COUNT(*) FROM comments WHERE author_id = u.id)
		FROM users u WHERE u.id = $1`, id,
	).Scan(&uid, &login, &dispName, &avatar, &createdAt, &posts, &comments)
	if err != nil {
		writeJSON(w, http.StatusNotFound, M{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, M{
		"id":            uid,
		"login":         login,
		"display_name":  dispName,
		"avatar_path":   nullStr(avatar),
		"created_at":    createdAt,
		"post_count":    posts,
		"comment_count": comments,
	})
}

func (h *MemberHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT u.id, u.login, u.display_name, u.avatar_path, u.created_at,
		       (SELECT COUNT(*) FROM posts WHERE author_id = u.id AND visibility != 'draft') AS post_count,
		       (SELECT COUNT(*) FROM comments WHERE author_id = u.id) AS comment_count
		FROM users u
		ORDER BY u.created_at`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	defer rows.Close()

	var members []M
	for rows.Next() {
		var id int64
		var login, displayName, createdAt string
		var avatar sql.NullString
		var postCount, commentCount int
		if rows.Scan(&id, &login, &displayName, &avatar, &createdAt, &postCount, &commentCount) != nil {
			continue
		}
		members = append(members, M{
			"id":            id,
			"login":         login,
			"display_name":  displayName,
			"avatar_path":   nullStr(avatar),
			"created_at":    createdAt,
			"post_count":    postCount,
			"comment_count": commentCount,
		})
	}
	if members == nil {
		members = []M{}
	}
	writeJSON(w, http.StatusOK, M{"members": members})
}
