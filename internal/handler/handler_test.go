package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"image"
	"image/gif"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nananek/nekomisu-diary/internal/handler"
	"github.com/nananek/nekomisu-diary/internal/session"
	"github.com/nananek/nekomisu-diary/internal/testutil"
	"golang.org/x/crypto/bcrypt"
)

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

// --- Test harness ---

type harness struct {
	db         *sql.DB
	sess       *session.Manager
	mux        http.Handler
	uploadsDir string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db := testutil.NewDB(t)
	sess := session.NewManager(db)
	uploadsDir := t.TempDir()

	auth := handler.NewAuthHandler(db, sess).AllowRegistration(true)
	posts := handler.NewPostHandler(db, nil)
	comments := handler.NewCommentHandler(db, nil)
	members := handler.NewMemberHandler(db)
	media := handler.NewMediaHandler(db, uploadsDir)

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

	mux.Handle("POST /api/media/upload", handler.RequireAuth(http.HandlerFunc(media.Upload)))
	mux.Handle("POST /api/auth/avatar", handler.RequireAuth(http.HandlerFunc(media.UploadAvatar)))

	return &harness{
		db:         db,
		sess:       sess,
		mux:        injectUser(sess, mux),
		uploadsDir: uploadsDir,
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

// upload sends a multipart file to path and returns the response. The
// filename and contentType are attacker-controlled, like in a real request.
func (h *harness) upload(t *testing.T, path, field, filename, contentType string, data []byte, cookie *http.Cookie) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="` + field + `"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatalf("multipart: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("multipart write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec.Result()
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func gifBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gif.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatalf("gif encode: %v", err)
	}
	return buf.Bytes()
}

func storedFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk uploads: %v", err)
	}
	return files
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

func TestPosts_Draft_OwnerCanOpen(t *testing.T) {
	h := newHarness(t)
	_, aliceCookie := h.createUser(t, "alice", "password")
	_, bobCookie := h.createUser(t, "bob", "password")

	resp := h.req(t, "POST", "/api/posts", map[string]string{
		"title": "書きかけ", "body": "<p>下書き本文</p>", "visibility": "draft",
	}, aliceCookie)
	if resp.StatusCode != 201 {
		t.Fatalf("create draft: %d", resp.StatusCode)
	}
	var created map[string]any
	decode(t, resp, &created)
	pid := int64(created["id"].(float64))

	// The author must be able to open their own draft. This used to 404 even
	// for the author, making the drafts list's edit links a dead end.
	resp = h.req(t, "GET", "/api/posts/"+itoa(pid), nil, aliceCookie)
	if resp.StatusCode != 200 {
		t.Fatalf("owner cannot open own draft: %d", resp.StatusCode)
	}
	var got map[string]any
	decode(t, resp, &got)
	if got["visibility"] != "draft" {
		t.Errorf("visibility: got %v want draft", got["visibility"])
	}
	if got["body_html"] != "<p>下書き本文</p>" {
		t.Errorf("body_html: got %v", got["body_html"])
	}

	// Other users still can't see it.
	resp = h.req(t, "GET", "/api/posts/"+itoa(pid), nil, bobCookie)
	if resp.StatusCode != 404 {
		t.Errorf("other user can open draft: got %d want 404", resp.StatusCode)
	}

	// And it must not leak into the timeline or search results.
	resp = h.req(t, "GET", "/api/posts", nil, aliceCookie)
	var list struct {
		Total int `json:"total"`
	}
	decode(t, resp, &list)
	if list.Total != 0 {
		t.Errorf("draft leaked into timeline: total=%d", list.Total)
	}
	resp = h.req(t, "GET", "/api/posts/search?q=書きかけ", nil, aliceCookie)
	var search struct {
		Total int `json:"total"`
	}
	decode(t, resp, &search)
	if search.Total != 0 {
		t.Errorf("draft leaked into search: total=%d", search.Total)
	}
}

func TestPosts_Draft_EditAndPublish(t *testing.T) {
	h := newHarness(t)
	_, aliceCookie := h.createUser(t, "alice", "password")
	_, bobCookie := h.createUser(t, "bob", "password")

	resp := h.req(t, "POST", "/api/posts", map[string]string{
		"title": "WIP", "body": "<p>v1</p>", "visibility": "draft",
	}, aliceCookie)
	var created map[string]any
	decode(t, resp, &created)
	pid := int64(created["id"].(float64))

	// Edit the draft while it is still hidden.
	resp = h.req(t, "PUT", "/api/posts/"+itoa(pid), map[string]string{
		"title": "WIP2", "body": "<p>v2</p>",
	}, aliceCookie)
	if resp.StatusCode != 200 {
		t.Fatalf("update draft: %d", resp.StatusCode)
	}
	resp = h.req(t, "GET", "/api/posts/"+itoa(pid), nil, aliceCookie)
	var got map[string]any
	decode(t, resp, &got)
	if got["title"] != "WIP2" {
		t.Errorf("title after update: got %v", got["title"])
	}
	if got["visibility"] != "draft" {
		t.Errorf("visibility after update: got %v", got["visibility"])
	}

	// Publishing makes it readable by others.
	resp = h.req(t, "PUT", "/api/posts/"+itoa(pid), map[string]string{"visibility": "public"}, aliceCookie)
	if resp.StatusCode != 200 {
		t.Fatalf("publish draft: %d", resp.StatusCode)
	}
	resp = h.req(t, "GET", "/api/posts/"+itoa(pid), nil, bobCookie)
	if resp.StatusCode != 200 {
		t.Errorf("published draft not visible to others: %d", resp.StatusCode)
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

func TestPosts_VisibilityValidation(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.createUser(t, "alice", "password")

	resp := h.req(t, "POST", "/api/posts", map[string]string{
		"title": "T", "body": "<p>x</p>", "visibility": "unlisted",
	}, cookie)
	if resp.StatusCode != 400 {
		t.Errorf("create with bad visibility: got %d want 400", resp.StatusCode)
	}

	resp = h.req(t, "POST", "/api/posts", map[string]string{"title": "T", "body": "<p>x</p>"}, cookie)
	if resp.StatusCode != 201 {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	var created map[string]any
	decode(t, resp, &created)
	pid := int64(created["id"].(float64))

	resp = h.req(t, "PUT", "/api/posts/"+itoa(pid), map[string]string{"visibility": "unlisted"}, cookie)
	if resp.StatusCode != 400 {
		t.Errorf("update with bad visibility: got %d want 400", resp.StatusCode)
	}
}

// --- Media tests ---

func TestMedia_Upload_RejectsNonImageContent(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.createUser(t, "alice", "password")

	payload := []byte("<!doctype html><script>alert(document.cookie)</script>")

	// Lying about the filename and Content-Type must not get HTML stored
	// under /uploads, where it would be served as same-origin script.
	resp := h.upload(t, "/api/media/upload", "file", "evil.html", "image/png", payload, cookie)
	if resp.StatusCode != 400 {
		t.Errorf("media upload as evil.html: got %d want 400", resp.StatusCode)
	}
	resp = h.upload(t, "/api/media/upload", "file", "evil.png", "image/png", payload, cookie)
	if resp.StatusCode != 400 {
		t.Errorf("media upload as evil.png: got %d want 400", resp.StatusCode)
	}

	// Avatars are stored at predictable URLs, so the same check applies.
	resp = h.upload(t, "/api/auth/avatar", "file", "avatar.html", "image/png", payload, cookie)
	if resp.StatusCode != 400 {
		t.Errorf("avatar upload as avatar.html: got %d want 400", resp.StatusCode)
	}

	if files := storedFiles(t, h.uploadsDir); len(files) != 0 {
		t.Errorf("rejected uploads left files on disk: %v", files)
	}
}

func TestMedia_Upload_StoresByDecodedFormat(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.createUser(t, "alice", "password")
	data := pngBytes(t)

	// The filename says .html and the MIME type says text/plain, but the
	// bytes really are a PNG: accepted, stored as .png, never as .html.
	resp := h.upload(t, "/api/media/upload", "file", "photo.html", "text/plain", data, cookie)
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("media upload: got %d (%s)", resp.StatusCode, body)
	}
	var created map[string]any
	decode(t, resp, &created)
	if url, _ := created["url"].(string); !strings.HasSuffix(url, ".png") {
		t.Errorf("stored url %q does not end in .png", url)
	}

	resp = h.upload(t, "/api/auth/avatar", "file", "a.html", "text/plain", data, cookie)
	if resp.StatusCode != 200 {
		t.Fatalf("avatar upload: got %d", resp.StatusCode)
	}
	var avatar map[string]any
	decode(t, resp, &avatar)
	if p, _ := avatar["avatar_path"].(string); !strings.HasSuffix(p, ".png") {
		t.Errorf("avatar path %q does not end in .png", p)
	}

	// GIF (and WebP) uploads keep working and get their own extension.
	resp = h.upload(t, "/api/media/upload", "file", "animation.gif", "image/gif", gifBytes(t), cookie)
	if resp.StatusCode != 201 {
		t.Fatalf("gif upload: got %d", resp.StatusCode)
	}
	var gifCreated map[string]any
	decode(t, resp, &gifCreated)
	if url, _ := gifCreated["url"].(string); !strings.HasSuffix(url, ".gif") {
		t.Errorf("stored gif url %q does not end in .gif", url)
	}

	files := storedFiles(t, h.uploadsDir)
	if len(files) != 4 {
		t.Fatalf("expected 4 stored files (png, thumbnail, avatar, gif), got %v", files)
	}
	pngs := 0
	for _, f := range files {
		if strings.HasSuffix(f, ".html") {
			t.Errorf("stored file %q kept a client-chosen extension", f)
		}
		if strings.HasSuffix(f, ".png") {
			pngs++
		}
	}
	if pngs != 2 {
		t.Errorf("expected 2 .png files, got %d in %v", pngs, files)
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

// Comments inherit their post's visibility. Post IDs are sequential, so
// "you'd have to know the ID" is no protection at all.
func TestComments_HiddenPost_NotVisibleToOthers(t *testing.T) {
	h := newHarness(t)
	_, aliceCookie := h.createUser(t, "alice", "password")
	_, bobCookie := h.createUser(t, "bob", "password")

	for _, visibility := range []string{"private", "draft"} {
		resp := h.req(t, "POST", "/api/posts", map[string]string{
			"title": "Hidden " + visibility, "body": "<p>x</p>", "visibility": visibility,
		}, aliceCookie)
		if resp.StatusCode != 201 {
			t.Fatalf("%s: create: %d", visibility, resp.StatusCode)
		}
		var created map[string]any
		decode(t, resp, &created)
		pid := int64(created["id"].(float64))

		// The author can read and write comments on her own hidden post.
		resp = h.req(t, "POST", "/api/posts/"+itoa(pid)+"/comments", map[string]string{"body": "自分用メモ"}, aliceCookie)
		if resp.StatusCode != 201 {
			t.Errorf("%s: owner cannot comment: %d", visibility, resp.StatusCode)
		}
		resp = h.req(t, "GET", "/api/posts/"+itoa(pid)+"/comments", nil, aliceCookie)
		var own struct {
			Comments []map[string]any `json:"comments"`
		}
		decode(t, resp, &own)
		if len(own.Comments) != 1 {
			t.Errorf("%s: owner comment list: got %d want 1", visibility, len(own.Comments))
		}

		// Others can neither read nor write comments, even knowing the ID.
		resp = h.req(t, "GET", "/api/posts/"+itoa(pid)+"/comments", nil, bobCookie)
		if resp.StatusCode != 404 {
			t.Errorf("%s: other user can list comments: got %d want 404", visibility, resp.StatusCode)
		}
		resp = h.req(t, "POST", "/api/posts/"+itoa(pid)+"/comments", map[string]string{"body": "sneak"}, bobCookie)
		if resp.StatusCode != 404 {
			t.Errorf("%s: other user can comment: got %d want 404", visibility, resp.StatusCode)
		}
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

