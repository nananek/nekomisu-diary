package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nananek/nekomisu-diary/internal/handler"
	"github.com/nananek/nekomisu-diary/internal/session"
	"github.com/nananek/nekomisu-diary/internal/testutil"
	"golang.org/x/crypto/bcrypt"
)

// --- Test harness ---

type harness struct {
	db   *sql.DB
	sess *session.Manager
	mux  http.Handler
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db := testutil.NewDB(t)
	sess := session.NewManager(db)

	auth := handler.NewAuthHandler(db, sess)
	posts := handler.NewPostHandler(db, nil)
	comments := handler.NewCommentHandler(db, nil)
	members := handler.NewMemberHandler(db)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", auth.Login)
	mux.HandleFunc("POST /api/auth/register", auth.Register)
	mux.Handle("GET /api/auth/me", injectUser(sess, http.HandlerFunc(auth.Me)))
	mux.Handle("POST /api/auth/logout", handler.RequireAuth(http.HandlerFunc(auth.Logout)))
	mux.Handle("PUT /api/auth/password", handler.RequireAuth(http.HandlerFunc(auth.ChangePassword)))
	mux.Handle("PUT /api/auth/profile", handler.RequireAuth(http.HandlerFunc(auth.UpdateProfile)))

	mux.Handle("GET /api/posts", handler.RequireAuth(http.HandlerFunc(posts.List)))
	mux.Handle("GET /api/posts/{id}", handler.RequireAuth(http.HandlerFunc(posts.Get)))
	mux.Handle("POST /api/posts", handler.RequireAuth(http.HandlerFunc(posts.Create)))
	mux.Handle("PUT /api/posts/{id}", handler.RequireAuth(http.HandlerFunc(posts.Update)))
	mux.Handle("DELETE /api/posts/{id}", handler.RequireAuth(http.HandlerFunc(posts.Delete)))
	mux.Handle("GET /api/posts/drafts", handler.RequireAuth(http.HandlerFunc(posts.Drafts)))
	mux.Handle("GET /api/posts/search", handler.RequireAuth(http.HandlerFunc(posts.Search)))
	mux.Handle("GET /api/users/{userId}/posts", handler.RequireAuth(http.HandlerFunc(posts.ByAuthor)))

	mux.Handle("GET /api/posts/{id}/comments", handler.RequireAuth(http.HandlerFunc(comments.List)))
	mux.Handle("POST /api/posts/{id}/comments", handler.RequireAuth(http.HandlerFunc(comments.Create)))
	mux.Handle("DELETE /api/comments/{commentId}", handler.RequireAuth(http.HandlerFunc(comments.Delete)))
	mux.Handle("GET /api/members", handler.RequireAuth(http.HandlerFunc(members.List)))

	return &harness{
		db:   db,
		sess: sess,
		mux:  injectUser(sess, mux),
	}
}

func injectUser(sess *session.Manager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, err := sess.Get(r); err == nil {
			r = r.WithContext(handler.ContextWithUser(r.Context(), u))
		}
		next.ServeHTTP(w, r)
	})
}

func (h *harness) req(t *testing.T, method, path string, body any, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec.Result()
}

func (h *harness) createUser(t *testing.T, login, password string) (int64, *http.Cookie) {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	id := testutil.InsertUser(t, h.db, login, login+"@ex.com", login, string(hash))

	resp := h.req(t, "POST", "/api/auth/login", map[string]string{"login": login, "password": password})
	if resp.StatusCode != 200 {
		t.Fatalf("login failed: %d", resp.StatusCode)
	}
	return id, firstCookie(resp)
}

func firstCookie(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == session.CookieName {
			return c
		}
	}
	return nil
}

func decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// --- Auth tests ---

func TestAuth_Login_WrongPassword(t *testing.T) {
	h := newHarness(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("right"), bcrypt.MinCost)
	testutil.InsertUser(t, h.db, "alice", "a@ex.com", "Alice", string(hash))

	resp := h.req(t, "POST", "/api/auth/login", map[string]string{"login": "alice", "password": "wrong"})
	if resp.StatusCode != 401 {
		t.Errorf("got %d want 401", resp.StatusCode)
	}
}

func TestAuth_Login_NonexistentUser(t *testing.T) {
	h := newHarness(t)
	resp := h.req(t, "POST", "/api/auth/login", map[string]string{"login": "nobody", "password": "x"})
	if resp.StatusCode != 401 {
		t.Errorf("got %d want 401", resp.StatusCode)
	}
}

func TestAuth_Register(t *testing.T) {
	h := newHarness(t)
	resp := h.req(t, "POST", "/api/auth/register", map[string]string{
		"login": "newbie", "email": "n@ex.com", "display_name": "New", "password": "supersecure",
	})
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("got %d want 201: %s", resp.StatusCode, body)
	}
	if firstCookie(resp) == nil {
		t.Error("expected session cookie on register")
	}
}

func TestAuth_Register_ShortPassword(t *testing.T) {
	h := newHarness(t)
	resp := h.req(t, "POST", "/api/auth/register", map[string]string{
		"login": "n", "email": "n@ex.com", "display_name": "N", "password": "short",
	})
	if resp.StatusCode != 400 {
		t.Errorf("got %d want 400", resp.StatusCode)
	}
}

func TestAuth_Register_DuplicateLogin(t *testing.T) {
	h := newHarness(t)
	testutil.InsertUser(t, h.db, "taken", "a@ex.com", "A", "hash")
	resp := h.req(t, "POST", "/api/auth/register", map[string]string{
		"login": "taken", "email": "b@ex.com", "display_name": "B", "password": "password123",
	})
	if resp.StatusCode != 409 {
		t.Errorf("got %d want 409", resp.StatusCode)
	}
}

func TestAuth_Me_Unauthenticated(t *testing.T) {
	h := newHarness(t)
	resp := h.req(t, "GET", "/api/auth/me", nil)
	if resp.StatusCode != 401 {
		t.Errorf("got %d want 401", resp.StatusCode)
	}
}

func TestAuth_Me_Authenticated(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.createUser(t, "alice", "password")

	resp := h.req(t, "GET", "/api/auth/me", nil, cookie)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	var me map[string]any
	decode(t, resp, &me)
	if me["login"] != "alice" {
		t.Errorf("login: got %v", me["login"])
	}
	if me["has_2fa"] != false {
		t.Errorf("has_2fa: got %v", me["has_2fa"])
	}
}

func TestAuth_ChangePassword(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.createUser(t, "alice", "original")

	resp := h.req(t, "PUT", "/api/auth/password", map[string]string{
		"old_password": "original", "new_password": "brand-new-pass",
	}, cookie)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}

	// New password works
	resp2 := h.req(t, "POST", "/api/auth/login", map[string]string{"login": "alice", "password": "brand-new-pass"})
	if resp2.StatusCode != 200 {
		t.Errorf("login with new pw failed: %d", resp2.StatusCode)
	}
}

func TestAuth_ChangePassword_WrongOld(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.createUser(t, "alice", "original")

	resp := h.req(t, "PUT", "/api/auth/password", map[string]string{
		"old_password": "wrong", "new_password": "brand-new-pass",
	}, cookie)
	if resp.StatusCode != 401 {
		t.Errorf("got %d want 401", resp.StatusCode)
	}
}

// --- Posts tests ---

func TestPosts_CreateListGetDelete(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.createUser(t, "alice", "password")

	// Create
	resp := h.req(t, "POST", "/api/posts", map[string]string{
		"title": "Hello", "body": "<p>world</p>", "visibility": "public",
	}, cookie)
	if resp.StatusCode != 201 {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	var created map[string]any
	decode(t, resp, &created)
	pid := int64(created["id"].(float64))

	// List
	resp = h.req(t, "GET", "/api/posts", nil, cookie)
	if resp.StatusCode != 200 {
		t.Fatalf("list: %d", resp.StatusCode)
	}
	var list struct {
		Posts []map[string]any `json:"posts"`
		Total int              `json:"total"`
	}
	decode(t, resp, &list)
	if list.Total != 1 {
		t.Errorf("total: got %d want 1", list.Total)
	}

	// Get
	resp = h.req(t, "GET", "/api/posts/"+itoa(pid), nil, cookie)
	if resp.StatusCode != 200 {
		t.Fatalf("get: %d", resp.StatusCode)
	}
	var got map[string]any
	decode(t, resp, &got)
	if got["title"] != "Hello" {
		t.Errorf("title: got %v", got["title"])
	}

	// Delete
	resp = h.req(t, "DELETE", "/api/posts/"+itoa(pid), nil, cookie)
	if resp.StatusCode != 200 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}

	// Get → 404
	resp = h.req(t, "GET", "/api/posts/"+itoa(pid), nil, cookie)
	if resp.StatusCode != 404 {
		t.Errorf("after delete: got %d want 404", resp.StatusCode)
	}
}

func TestPosts_PrivatePost_NotVisibleToOthers(t *testing.T) {
	h := newHarness(t)
	_, aliceCookie := h.createUser(t, "alice", "password")
	_, bobCookie := h.createUser(t, "bob", "password")

	// Alice creates a private post
	resp := h.req(t, "POST", "/api/posts", map[string]string{
		"title": "Secret", "body": "<p>shh</p>", "visibility": "private",
	}, aliceCookie)
	var created map[string]any
	decode(t, resp, &created)
	pid := int64(created["id"].(float64))

	// Alice can see it
	resp = h.req(t, "GET", "/api/posts/"+itoa(pid), nil, aliceCookie)
	if resp.StatusCode != 200 {
		t.Errorf("owner cannot see own private post: %d", resp.StatusCode)
	}

	// Bob cannot
	resp = h.req(t, "GET", "/api/posts/"+itoa(pid), nil, bobCookie)
	if resp.StatusCode != 404 {
		t.Errorf("other user sees private post: %d", resp.StatusCode)
	}

	// Bob's list doesn't include it
	resp = h.req(t, "GET", "/api/posts", nil, bobCookie)
	var list struct{ Total int `json:"total"` }
	decode(t, resp, &list)
	if list.Total != 0 {
		t.Errorf("bob sees alice's private post in list: total=%d", list.Total)
	}
}

func TestPosts_Delete_OtherUserForbidden(t *testing.T) {
	h := newHarness(t)
	_, aliceCookie := h.createUser(t, "alice", "password")
	_, bobCookie := h.createUser(t, "bob", "password")

	resp := h.req(t, "POST", "/api/posts", map[string]string{
		"title": "Alice", "body": "<p>x</p>",
	}, aliceCookie)
	var created map[string]any
	decode(t, resp, &created)
	pid := int64(created["id"].(float64))

	resp = h.req(t, "DELETE", "/api/posts/"+itoa(pid), nil, bobCookie)
	if resp.StatusCode != 403 {
		t.Errorf("got %d want 403", resp.StatusCode)
	}
}

func TestPosts_Drafts_OnlyOwn(t *testing.T) {
	h := newHarness(t)
	_, aliceCookie := h.createUser(t, "alice", "password")
	_, bobCookie := h.createUser(t, "bob", "password")

	h.req(t, "POST", "/api/posts", map[string]string{"title": "A", "body": "<p>x</p>", "visibility": "draft"}, aliceCookie)
	h.req(t, "POST", "/api/posts", map[string]string{"title": "B", "body": "<p>y</p>", "visibility": "draft"}, bobCookie)

	resp := h.req(t, "GET", "/api/posts/drafts", nil, aliceCookie)
	var drafts struct {
		Posts []map[string]any `json:"posts"`
	}
	decode(t, resp, &drafts)
	if len(drafts.Posts) != 1 {
		t.Errorf("alice drafts: got %d want 1", len(drafts.Posts))
	}
	if drafts.Posts[0]["title"] != "A" {
		t.Errorf("wrong draft shown: %v", drafts.Posts[0]["title"])
	}
}

func TestPosts_Search(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.createUser(t, "alice", "password")

	h.req(t, "POST", "/api/posts", map[string]string{"title": "猫の話", "body": "<p>にゃーん</p>"}, cookie)
	h.req(t, "POST", "/api/posts", map[string]string{"title": "犬の話", "body": "<p>わん</p>"}, cookie)

	resp := h.req(t, "GET", "/api/posts/search?q=猫", nil, cookie)
	var result struct {
		Posts []map[string]any `json:"posts"`
		Total int              `json:"total"`
	}
	decode(t, resp, &result)
	if result.Total != 1 {
		t.Errorf("search total: got %d want 1", result.Total)
	}
	if strings.Contains(result.Posts[0]["title"].(string), "犬") {
		t.Errorf("wrong post: %v", result.Posts[0]["title"])
	}
}

// --- Comments tests ---

func TestComments_CreateListDelete(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.createUser(t, "alice", "password")

	resp := h.req(t, "POST", "/api/posts", map[string]string{"title": "T", "body": "<p>B</p>"}, cookie)
	var post map[string]any
	decode(t, resp, &post)
	pid := int64(post["id"].(float64))

	// Create comment
	resp = h.req(t, "POST", "/api/posts/"+itoa(pid)+"/comments", map[string]string{"body": "nice"}, cookie)
	if resp.StatusCode != 201 {
		t.Fatalf("comment: %d", resp.StatusCode)
	}
	var cc map[string]any
	decode(t, resp, &cc)
	cid := int64(cc["id"].(float64))

	// List
	resp = h.req(t, "GET", "/api/posts/"+itoa(pid)+"/comments", nil, cookie)
	var list struct {
		Comments []map[string]any `json:"comments"`
	}
	decode(t, resp, &list)
	if len(list.Comments) != 1 {
		t.Errorf("list: got %d", len(list.Comments))
	}

	// Delete
	resp = h.req(t, "DELETE", "/api/comments/"+itoa(cid), nil, cookie)
	if resp.StatusCode != 200 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}

	// List again → empty
	resp = h.req(t, "GET", "/api/posts/"+itoa(pid)+"/comments", nil, cookie)
	decode(t, resp, &list)
	if len(list.Comments) != 0 {
		t.Errorf("after delete: got %d", len(list.Comments))
	}
}

func TestComments_Delete_OtherUserForbidden(t *testing.T) {
	h := newHarness(t)
	_, aliceCookie := h.createUser(t, "alice", "password")
	_, bobCookie := h.createUser(t, "bob", "password")

	resp := h.req(t, "POST", "/api/posts", map[string]string{"title": "T", "body": "<p>B</p>"}, aliceCookie)
	var post map[string]any
	decode(t, resp, &post)
	pid := int64(post["id"].(float64))

	resp = h.req(t, "POST", "/api/posts/"+itoa(pid)+"/comments", map[string]string{"body": "alice's comment"}, aliceCookie)
	var cc map[string]any
	decode(t, resp, &cc)
	cid := int64(cc["id"].(float64))

	resp = h.req(t, "DELETE", "/api/comments/"+itoa(cid), nil, bobCookie)
	if resp.StatusCode != 403 {
		t.Errorf("got %d want 403", resp.StatusCode)
	}
}

// --- Members tests ---

func TestMembers_List(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.createUser(t, "alice", "password")
	h.createUser(t, "bob", "password")

	// Alice creates a public post + comment
	resp := h.req(t, "POST", "/api/posts", map[string]string{"title": "T", "body": "<p>B</p>"}, cookie)
	var p map[string]any
	decode(t, resp, &p)
	pid := int64(p["id"].(float64))
	h.req(t, "POST", "/api/posts/"+itoa(pid)+"/comments", map[string]string{"body": "hi"}, cookie)

	resp = h.req(t, "GET", "/api/members", nil, cookie)
	var result struct {
		Members []map[string]any `json:"members"`
	}
	decode(t, resp, &result)
	if len(result.Members) < 2 {
		t.Fatalf("expected 2+ members, got %d", len(result.Members))
	}
	for _, m := range result.Members {
		if m["login"] == "alice" {
			if int(m["post_count"].(float64)) != 1 {
				t.Errorf("alice post_count: got %v want 1", m["post_count"])
			}
			if int(m["comment_count"].(float64)) != 1 {
				t.Errorf("alice comment_count: got %v want 1", m["comment_count"])
			}
		}
	}
}

// --- Helpers ---

func itoa(i int64) string {
	b := make([]byte, 0, 20)
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
