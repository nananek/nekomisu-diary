package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/nananek/nekomisu-diary/internal/dbq"
)

type MemberHandler struct {
	q *dbq.Queries
}

func NewMemberHandler(db *sql.DB) *MemberHandler {
	return &MemberHandler{q: dbq.New(db)}
}

func (h *MemberHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListMembers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	members := make([]M, 0, len(rows))
	for _, rr := range rows {
		members = append(members, M{
			"id":            rr.ID,
			"login":         rr.Login,
			"display_name":  rr.DisplayName,
			"avatar_path":   nullStr(rr.AvatarPath),
			"created_at":    rr.CreatedAt,
			"post_count":    rr.PostCount,
			"comment_count": rr.CommentCount,
		})
	}
	writeJSON(w, http.StatusOK, M{"members": members})
}

func (h *MemberHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	rr, err := h.q.GetMember(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, M{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, M{
		"id":            rr.ID,
		"login":         rr.Login,
		"display_name":  rr.DisplayName,
		"avatar_path":   nullStr(rr.AvatarPath),
		"created_at":    rr.CreatedAt,
		"post_count":    rr.PostCount,
		"comment_count": rr.CommentCount,
	})
}
