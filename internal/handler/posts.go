package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nananek/nekomisu-diary/internal/dbq"
	"github.com/nananek/nekomisu-diary/internal/sanitize"
)

type PostHandler struct {
	q        *dbq.Queries
	notifier PostNotifier
}

type PostNotifier interface {
	NotifyPost(postID int64, title string)
}

func NewPostHandler(db *sql.DB, n PostNotifier) *PostHandler {
	return &PostHandler{q: dbq.New(db), notifier: n}
}

type postJSON struct {
	ID           int64   `json:"id"`
	AuthorID     int64   `json:"author_id"`
	AuthorName   string  `json:"author_name"`
	AuthorAvatar *string `json:"author_avatar"`
	Title        string  `json:"title"`
	BodyHTML     string  `json:"body_html"`
	BodyMD       *string `json:"body_md,omitempty"`
	Excerpt      string  `json:"excerpt"`
	Visibility   string  `json:"visibility"`
	PublishedAt  *string `json:"published_at"`
	CreatedAt    string  `json:"created_at"`
	CommentCount int     `json:"comment_count"`
}

// makeExcerpt strips tags from HTML and truncates to the given rune count.
func makeExcerpt(htmlStr string, runes int) string {
	var b strings.Builder
	inTag := false
	for _, r := range htmlStr {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	s := strings.TrimSpace(b.String())
	rs := []rune(s)
	if len(rs) > runes {
		return string(rs[:runes]) + "…"
	}
	return s
}

// postFromListRow adapts generated list-row types into postJSON for the
// API response. Each list query shares the same column shape.
type listRow struct {
	ID           int64
	AuthorID     int64
	AuthorName   string
	AuthorAvatar sql.NullString
	Title        string
	BodyHTML     string
	Visibility   string
	PublishedAt  sql.NullTime
	CreatedAt    time.Time
	CommentCount int32
}

func listRowToJSON(r listRow) postJSON {
	p := postJSON{
		ID:           r.ID,
		AuthorID:     r.AuthorID,
		AuthorName:   r.AuthorName,
		AuthorAvatar: nullStr(r.AuthorAvatar),
		Title:        r.Title,
		Visibility:   r.Visibility,
		CreatedAt:    r.CreatedAt.Format(time.RFC3339),
		CommentCount: int(r.CommentCount),
		Excerpt:      makeExcerpt(r.BodyHTML, 160),
	}
	if r.PublishedAt.Valid {
		s := r.PublishedAt.Time.Format(time.RFC3339)
		p.PublishedAt = &s
	}
	return p
}

func (h *PostHandler) List(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const limit = 20
	offset := (page - 1) * limit

	rows, err := h.q.ListPosts(r.Context(), dbq.ListPostsParams{
		AuthorID: u.UserID,
		Limit:    limit,
		Offset:   int32(offset),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	total, _ := h.q.CountVisiblePosts(r.Context(), u.UserID)

	posts := make([]postJSON, 0, len(rows))
	for _, rr := range rows {
		posts = append(posts, listRowToJSON(listRow{
			ID: rr.ID, AuthorID: rr.AuthorID, AuthorName: rr.AuthorName,
			AuthorAvatar: rr.AuthorAvatar, Title: rr.Title, BodyHTML: rr.BodyHtml,
			Visibility: rr.Visibility, PublishedAt: rr.PublishedAt,
			CreatedAt: rr.CreatedAt, CommentCount: rr.CommentCount,
		}))
	}
	writeJSON(w, http.StatusOK, M{
		"posts": posts, "total": total, "page": page,
		"pages": (int(total) + limit - 1) / limit,
	})
}

func (h *PostHandler) Get(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	row, err := h.q.GetPost(r.Context(), dbq.GetPostParams{
		ID:       id,
		AuthorID: u.UserID,
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, M{"error": "not found"})
		return
	}
	p := postJSON{
		ID:           row.ID,
		AuthorID:     row.AuthorID,
		AuthorName:   row.AuthorName,
		AuthorAvatar: nullStr(row.AuthorAvatar),
		Title:        row.Title,
		BodyHTML:     row.BodyHtml,
		Visibility:   row.Visibility,
		CreatedAt:    row.CreatedAt.Format(time.RFC3339),
		CommentCount: int(row.CommentCount),
	}
	if row.BodyMd.Valid {
		p.BodyMD = &row.BodyMd.String
	}
	if row.PublishedAt.Valid {
		s := row.PublishedAt.Time.Format(time.RFC3339)
		p.PublishedAt = &s
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *PostHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	var req struct {
		Title      string  `json:"title"`
		Body       string  `json:"body"`
		BodyMD     *string `json:"body_md"`
		Visibility string  `json:"visibility"`
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

	var publishedAt sql.NullTime
	if req.Visibility != "draft" {
		publishedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	var bodyMD sql.NullString
	if req.BodyMD != nil {
		bodyMD = sql.NullString{String: *req.BodyMD, Valid: true}
	}

	id, err := h.q.CreatePost(r.Context(), dbq.CreatePostParams{
		AuthorID:    u.UserID,
		Title:       req.Title,
		BodyHtml:    sanitize.HTML(req.Body),
		BodyMd:      bodyMD,
		Visibility:  req.Visibility,
		PublishedAt: publishedAt,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	if h.notifier != nil && req.Visibility == "public" {
		h.notifier.NotifyPost(id, req.Title)
	}
	writeJSON(w, http.StatusCreated, M{"id": id})
}

func (h *PostHandler) Update(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	info, err := h.q.GetPostForAuthorization(r.Context(), id)
	if err != nil || info.AuthorID != u.UserID {
		writeJSON(w, http.StatusForbidden, M{"error": "forbidden"})
		return
	}

	var req struct {
		Title      *string `json:"title"`
		Body       *string `json:"body"`
		BodyMD     *string `json:"body_md"`
		Visibility *string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}

	ctx := r.Context()
	if req.Title != nil {
		_ = h.q.UpdatePostTitle(ctx, dbq.UpdatePostTitleParams{Title: *req.Title, ID: id})
	}
	if req.Body != nil {
		_ = h.q.UpdatePostBody(ctx, dbq.UpdatePostBodyParams{BodyHtml: sanitize.HTML(*req.Body), ID: id})
	}
	if req.BodyMD != nil {
		_ = h.q.UpdatePostBodyMD(ctx, dbq.UpdatePostBodyMDParams{
			BodyMd: sql.NullString{String: *req.BodyMD, Valid: true},
			ID:     id,
		})
	}
	newlyPublic := false
	if req.Visibility != nil {
		_ = h.q.UpdatePostVisibility(ctx, dbq.UpdatePostVisibilityParams{
			Visibility: *req.Visibility, ID: id,
		})
		if *req.Visibility == "public" && info.Visibility != "public" {
			newlyPublic = true
		}
	}
	if newlyPublic && h.notifier != nil {
		title := info.Title
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

	n, err := h.q.DeletePost(r.Context(), dbq.DeletePostParams{ID: id, AuthorID: u.UserID})
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

func (h *PostHandler) Drafts(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	rows, err := h.q.ListDrafts(r.Context(), u.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	posts := make([]postJSON, 0, len(rows))
	for _, rr := range rows {
		posts = append(posts, listRowToJSON(listRow{
			ID: rr.ID, AuthorID: rr.AuthorID, AuthorName: rr.AuthorName,
			AuthorAvatar: rr.AuthorAvatar, Title: rr.Title, BodyHTML: rr.BodyHtml,
			Visibility: rr.Visibility, PublishedAt: rr.PublishedAt,
			CreatedAt: rr.CreatedAt, CommentCount: rr.CommentCount,
		}))
	}
	writeJSON(w, http.StatusOK, M{"posts": posts})
}

func (h *PostHandler) Search(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, M{"error": "q parameter required"})
		return
	}
	rows, err := h.q.SearchPosts(r.Context(), dbq.SearchPostsParams{
		AuthorID: u.UserID,
		Title:    "%" + q + "%",
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	posts := make([]postJSON, 0, len(rows))
	for _, rr := range rows {
		posts = append(posts, listRowToJSON(listRow{
			ID: rr.ID, AuthorID: rr.AuthorID, AuthorName: rr.AuthorName,
			AuthorAvatar: rr.AuthorAvatar, Title: rr.Title, BodyHTML: rr.BodyHtml,
			Visibility: rr.Visibility, PublishedAt: rr.PublishedAt,
			CreatedAt: rr.CreatedAt, CommentCount: rr.CommentCount,
		}))
	}
	writeJSON(w, http.StatusOK, M{"posts": posts, "total": len(posts)})
}

func (h *PostHandler) ByAuthor(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	authorID, _ := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const limit = 20
	offset := (page - 1) * limit

	rows, err := h.q.ListPostsByAuthor(r.Context(), dbq.ListPostsByAuthorParams{
		AuthorID:   authorID,
		AuthorID_2: u.UserID,
		Limit:      limit,
		Offset:     int32(offset),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	total, _ := h.q.CountPostsByAuthor(r.Context(), dbq.CountPostsByAuthorParams{
		AuthorID:   authorID,
		AuthorID_2: u.UserID,
	})

	posts := make([]postJSON, 0, len(rows))
	for _, rr := range rows {
		posts = append(posts, listRowToJSON(listRow{
			ID: rr.ID, AuthorID: rr.AuthorID, AuthorName: rr.AuthorName,
			AuthorAvatar: rr.AuthorAvatar, Title: rr.Title, BodyHTML: rr.BodyHtml,
			Visibility: rr.Visibility, PublishedAt: rr.PublishedAt,
			CreatedAt: rr.CreatedAt, CommentCount: rr.CommentCount,
		}))
	}
	writeJSON(w, http.StatusOK, M{
		"posts": posts, "total": total, "page": page,
		"pages": (int(total) + limit - 1) / limit,
	})
}
