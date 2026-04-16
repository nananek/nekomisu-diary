package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nananek/nekomisu-diary/internal/testutil"
)

func TestSession_CreateAndGet_Verified(t *testing.T) {
	db := testutil.NewDB(t)
	m := NewManager(db)

	userID := testutil.InsertUser(t, db, "alice", "a@ex.com", "Alice", "hash")

	rec := httptest.NewRecorder()
	if err := m.Create(rec, userID, true); err != nil {
		t.Fatalf("Create: %v", err)
	}

	cookie := rec.Result().Cookies()[0]
	if cookie.Name != CookieName {
		t.Fatalf("wrong cookie name: %s", cookie.Name)
	}
	if !cookie.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)

	info, err := m.Get(req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info.UserID != userID {
		t.Errorf("UserID: got %d want %d", info.UserID, userID)
	}
	if info.Login != "alice" {
		t.Errorf("Login: got %s", info.Login)
	}
}

func TestSession_Pending_NotReturnedByGet(t *testing.T) {
	db := testutil.NewDB(t)
	m := NewManager(db)
	userID := testutil.InsertUser(t, db, "bob", "b@ex.com", "Bob", "hash")

	rec := httptest.NewRecorder()
	m.Create(rec, userID, false) // pending (not verified)
	cookie := rec.Result().Cookies()[0]

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)

	if _, err := m.Get(req); err == nil {
		t.Error("Get() should reject pending session")
	}
	if _, err := m.GetPending(req); err != nil {
		t.Errorf("GetPending() should accept pending: %v", err)
	}
}

func TestSession_Verify_UpgradesPending(t *testing.T) {
	db := testutil.NewDB(t)
	m := NewManager(db)
	userID := testutil.InsertUser(t, db, "carol", "c@ex.com", "Carol", "hash")

	rec := httptest.NewRecorder()
	m.Create(rec, userID, false)
	cookie := rec.Result().Cookies()[0]

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)

	if err := m.Verify(req); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if _, err := m.Get(req); err != nil {
		t.Errorf("Get() should accept verified session: %v", err)
	}
}

func TestSession_Destroy(t *testing.T) {
	db := testutil.NewDB(t)
	m := NewManager(db)
	userID := testutil.InsertUser(t, db, "dave", "d@ex.com", "Dave", "hash")

	rec := httptest.NewRecorder()
	m.Create(rec, userID, true)
	cookie := rec.Result().Cookies()[0]

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)

	rec2 := httptest.NewRecorder()
	m.Destroy(rec2, req)

	if _, err := m.Get(req); err == nil {
		t.Error("session should be gone after Destroy")
	}

	// Should set expired cookie
	deleted := rec2.Result().Cookies()[0]
	if deleted.MaxAge >= 0 {
		t.Errorf("cookie should be marked for deletion, got MaxAge=%d", deleted.MaxAge)
	}
}

func TestSession_ExpiredRejected(t *testing.T) {
	db := testutil.NewDB(t)
	m := NewManager(db)
	userID := testutil.InsertUser(t, db, "eve", "e@ex.com", "Eve", "hash")

	rec := httptest.NewRecorder()
	m.Create(rec, userID, true)
	cookie := rec.Result().Cookies()[0]

	// Force expiry
	db.Exec("UPDATE sessions SET expires_at = $1", time.Now().Add(-1*time.Hour))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)

	if _, err := m.Get(req); err == nil {
		t.Error("expired session should be rejected")
	}
}

func TestSession_Cleanup_RemovesExpired(t *testing.T) {
	db := testutil.NewDB(t)
	m := NewManager(db)
	userID := testutil.InsertUser(t, db, "frank", "f@ex.com", "Frank", "hash")

	m.Create(httptest.NewRecorder(), userID, true)
	db.Exec("UPDATE sessions SET expires_at = $1", time.Now().Add(-1*time.Hour))

	m.Cleanup()

	var count int
	db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 sessions after cleanup, got %d", count)
	}
}

func TestSession_Has2FA_TOTP(t *testing.T) {
	db := testutil.NewDB(t)
	m := NewManager(db)
	userID := testutil.InsertUser(t, db, "grace", "g@ex.com", "Grace", "hash")

	db.Exec("INSERT INTO totp_secrets (user_id, secret, verified) VALUES ($1, 'sec', true)", userID)

	rec := httptest.NewRecorder()
	m.Create(rec, userID, true)
	cookie := rec.Result().Cookies()[0]

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)

	info, err := m.Get(req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !info.HasTOTP {
		t.Error("HasTOTP should be true")
	}
	if !info.Has2FA {
		t.Error("Has2FA should be true")
	}
}

func TestCookie_HasCorrectAttributes(t *testing.T) {
	db := testutil.NewDB(t)
	m := NewManager(db)
	userID := testutil.InsertUser(t, db, "henry", "h@ex.com", "Henry", "hash")

	rec := httptest.NewRecorder()
	m.Create(rec, userID, true)
	cookie := rec.Result().Cookies()[0]

	if cookie.Path != "/" {
		t.Errorf("Path: got %s want /", cookie.Path)
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite: got %v", cookie.SameSite)
	}
}
