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
)

// mockMisskey plays the Misskey side of MiAuth just enough for these
// tests: POST /api/miauth/{token}/check reports ok:true + a user only for
// tokens the test has explicitly "approved" (simulating the human clicking
// Approve on the real instance).
type mockMisskey struct {
	*httptest.Server
	approved map[string]string // token -> misskey user id
}

func newMockMisskey(t *testing.T) *mockMisskey {
	t.Helper()
	m := &mockMisskey{approved: map[string]string{}}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 4 || parts[0] != "api" || parts[1] != "miauth" || parts[3] != "check" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		token := parts[2]
		w.Header().Set("Content-Type", "application/json")
		id, ok := m.approved[token]
		if !ok {
			json.NewEncoder(w).Encode(map[string]any{"ok": false})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"user": map[string]any{"id": id, "username": "misskeyuser"},
		})
	}))
	t.Cleanup(m.Close)
	return m
}

func (m *mockMisskey) approve(token, misskeyUserID string) {
	m.approved[token] = misskeyUserID
}

type miauthHarness struct {
	db  *sql.DB
	mux http.Handler
}

func newMiauthHarness(t *testing.T, instance string) *miauthHarness {
	t.Helper()
	db := testutil.NewDB(t)
	sess := session.NewManager(db)
	mi := handler.NewMiAuthHandler(db, sess, instance, "Test App", "https://diary.example")
	auth := handler.NewAuthHandler(db, sess)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/miauth/config", mi.Config)
	mux.HandleFunc("POST /api/auth/miauth/start", mi.Start)
	mux.HandleFunc("POST /api/auth/miauth/finish", mi.Finish)
	mux.HandleFunc("GET /api/auth/me", auth.Me)

	return &miauthHarness{db: db, mux: injectUser(sess, mux)}
}

func (h *miauthHarness) req(t *testing.T, method, path string, body any, cookies ...*http.Cookie) *http.Response {
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

// linkUser creates a diary user and links it to a Misskey account,
// bypassing the miauthlink CLI (which the handler never touches directly).
func (h *miauthHarness) linkUser(t *testing.T, instance, login, misskeyUserID string) int64 {
	t.Helper()
	userID := testutil.InsertUser(t, h.db, login, login+"@ex.com", login, "hash")
	if _, err := h.db.Exec(
		`INSERT INTO misskey_links (user_id, misskey_instance, misskey_user_id, misskey_username) VALUES ($1, $2, $3, $4)`,
		userID, instance, misskeyUserID, "misskeyuser",
	); err != nil {
		t.Fatalf("link user: %v", err)
	}
	return userID
}

func (h *miauthHarness) enableTOTP(t *testing.T, userID int64) {
	t.Helper()
	if _, err := h.db.Exec(
		`INSERT INTO totp_secrets (user_id, secret, verified) VALUES ($1, 'JBSWY3DPEHPK3PXP', true)`,
		userID,
	); err != nil {
		t.Fatalf("enable totp: %v", err)
	}
}

// startMiauth calls Start and extracts the session token from the
// returned authorization URL, the same way the frontend would follow it.
func startMiauth(t *testing.T, h *miauthHarness) string {
	t.Helper()
	resp := h.req(t, "POST", "/api/auth/miauth/start", nil)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("start: got %d: %s", resp.StatusCode, body)
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.URL == "" {
		t.Fatalf("start: bad response (err=%v)", err)
	}
	u := strings.SplitN(out.URL, "?", 2)[0]
	segments := strings.Split(u, "/")
	return segments[len(segments)-1]
}

func TestMiAuth_Disabled(t *testing.T) {
	h := newMiauthHarness(t, "") // no -misskey-instance configured

	resp := h.req(t, "GET", "/api/auth/miauth/config", nil)
	var cfg struct {
		Enabled bool `json:"enabled"`
	}
	decode(t, resp, &cfg)
	if cfg.Enabled {
		t.Error("expected enabled:false")
	}

	if resp := h.req(t, "POST", "/api/auth/miauth/start", nil); resp.StatusCode != 404 {
		t.Errorf("start: got %d want 404", resp.StatusCode)
	}
	if resp := h.req(t, "POST", "/api/auth/miauth/finish", map[string]string{"session": "x"}); resp.StatusCode != 404 {
		t.Errorf("finish: got %d want 404", resp.StatusCode)
	}
}

func TestMiAuth_Enabled(t *testing.T) {
	ms := newMockMisskey(t)
	h := newMiauthHarness(t, ms.URL)

	resp := h.req(t, "GET", "/api/auth/miauth/config", nil)
	var cfg struct {
		Enabled bool `json:"enabled"`
	}
	decode(t, resp, &cfg)
	if !cfg.Enabled {
		t.Error("expected enabled:true")
	}
}

func TestMiAuth_Start_ReturnsAuthorizationURL(t *testing.T) {
	ms := newMockMisskey(t)
	h := newMiauthHarness(t, ms.URL)

	resp := h.req(t, "POST", "/api/auth/miauth/start", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	var out struct {
		URL string `json:"url"`
	}
	decode(t, resp, &out)
	if !strings.HasPrefix(out.URL, ms.URL+"/miauth/") {
		t.Errorf("unexpected url: %s", out.URL)
	}
}

func TestMiAuth_Finish_UnknownSession(t *testing.T) {
	ms := newMockMisskey(t)
	h := newMiauthHarness(t, ms.URL)

	resp := h.req(t, "POST", "/api/auth/miauth/finish", map[string]string{"session": "never-issued"})
	if resp.StatusCode != 400 {
		t.Errorf("got %d want 400", resp.StatusCode)
	}
}

func TestMiAuth_Finish_NotLinked(t *testing.T) {
	ms := newMockMisskey(t)
	h := newMiauthHarness(t, ms.URL)

	token := startMiauth(t, h)
	ms.approve(token, "some-unlinked-misskey-id")

	resp := h.req(t, "POST", "/api/auth/miauth/finish", map[string]string{"session": token})
	if resp.StatusCode != 403 {
		t.Errorf("got %d want 403", resp.StatusCode)
	}
}

func TestMiAuth_Finish_NotApprovedOnMisskeySide(t *testing.T) {
	ms := newMockMisskey(t)
	h := newMiauthHarness(t, ms.URL)
	h.linkUser(t, ms.URL, "alice", "misskey-alice-id")

	// A real (issued-by-us) token, but the check endpoint reports ok:false
	// because nobody approved it on the Misskey side.
	token := startMiauth(t, h)

	resp := h.req(t, "POST", "/api/auth/miauth/finish", map[string]string{"session": token})
	if resp.StatusCode != 401 {
		t.Errorf("got %d want 401", resp.StatusCode)
	}
}

func TestMiAuth_Finish_LinkedAccount_LogsIn(t *testing.T) {
	ms := newMockMisskey(t)
	h := newMiauthHarness(t, ms.URL)
	h.linkUser(t, ms.URL, "alice", "misskey-alice-id")

	token := startMiauth(t, h)
	ms.approve(token, "misskey-alice-id")

	resp := h.req(t, "POST", "/api/auth/miauth/finish", map[string]string{"session": token})
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("got %d: %s", resp.StatusCode, body)
	}
	var out map[string]any
	decode(t, resp, &out)
	if out["ok"] != true {
		t.Errorf("expected ok:true, got %v", out)
	}

	cookie := firstCookie(resp)
	if cookie == nil {
		t.Fatal("expected session cookie")
	}
	me := h.req(t, "GET", "/api/auth/me", nil, cookie)
	if me.StatusCode != 200 {
		t.Errorf("session from miauth login should authenticate /me, got %d", me.StatusCode)
	}
}

func TestMiAuth_Finish_TokenIsSingleUse(t *testing.T) {
	ms := newMockMisskey(t)
	h := newMiauthHarness(t, ms.URL)
	h.linkUser(t, ms.URL, "alice", "misskey-alice-id")

	token := startMiauth(t, h)
	ms.approve(token, "misskey-alice-id")

	if resp := h.req(t, "POST", "/api/auth/miauth/finish", map[string]string{"session": token}); resp.StatusCode != 200 {
		t.Fatalf("first finish: got %d", resp.StatusCode)
	}
	if resp := h.req(t, "POST", "/api/auth/miauth/finish", map[string]string{"session": token}); resp.StatusCode != 400 {
		t.Errorf("replayed token: got %d want 400", resp.StatusCode)
	}
}

func TestMiAuth_Finish_LinkedAccountWith2FA_ReturnsPendingSession(t *testing.T) {
	ms := newMockMisskey(t)
	h := newMiauthHarness(t, ms.URL)
	userID := h.linkUser(t, ms.URL, "alice", "misskey-alice-id")
	h.enableTOTP(t, userID)

	token := startMiauth(t, h)
	ms.approve(token, "misskey-alice-id")

	resp := h.req(t, "POST", "/api/auth/miauth/finish", map[string]string{"session": token})
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	var out map[string]any
	decode(t, resp, &out)
	if out["requires_2fa"] != true {
		t.Errorf("expected requires_2fa:true, got %v", out)
	}

	// The cookie is a pending session: enough to continue to 2FA, not
	// enough to hit authenticated endpoints yet.
	cookie := firstCookie(resp)
	if cookie == nil {
		t.Fatal("expected pending session cookie")
	}
	me := h.req(t, "GET", "/api/auth/me", nil, cookie)
	if me.StatusCode != 401 {
		t.Errorf("pending session should not authenticate /me, got %d", me.StatusCode)
	}
}

// Two diary accounts linking two different Misskey accounts on two
// different instances must not be confused for each other, even if the
// misskey_user_id string happens to collide.
func TestMiAuth_Finish_ScopedToConfiguredInstance(t *testing.T) {
	ms := newMockMisskey(t)
	h := newMiauthHarness(t, ms.URL)
	// Linked against a *different* instance string than the one this
	// handler is configured for.
	h.linkUser(t, "https://other.example", "alice", "misskey-alice-id")

	token := startMiauth(t, h)
	ms.approve(token, "misskey-alice-id")

	resp := h.req(t, "POST", "/api/auth/miauth/finish", map[string]string{"session": token})
	if resp.StatusCode != 403 {
		t.Errorf("got %d want 403 (link is for a different instance)", resp.StatusCode)
	}
}
