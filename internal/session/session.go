package session

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"
)

const (
	CookieName = "session"
	TTL        = 30 * 24 * time.Hour
	PendingTTL = 5 * time.Minute
)

type Manager struct {
	db     *sql.DB
	secure bool
}

func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// WithSecureCookies marks the session cookie as Secure so browsers only
// send it over HTTPS. Must be true in production; false for plain-HTTP
// local development (e.g. http://localhost:3000).
func (m *Manager) WithSecureCookies(secure bool) *Manager {
	m.secure = secure
	return m
}

type UserInfo struct {
	UserID      int64
	Login       string
	DisplayName string
	AvatarPath  sql.NullString
	Has2FA      bool
	HasTOTP     bool
	HasWebAuthn bool
}

func (m *Manager) Create(w http.ResponseWriter, userID int64, verified bool) error {
	id, err := generateID()
	if err != nil {
		return err
	}
	ttl := TTL
	if !verified {
		ttl = PendingTTL
	}
	expires := time.Now().Add(ttl)
	_, err = m.db.Exec(
		`INSERT INTO sessions (id, user_id, verified, expires_at) VALUES ($1, $2, $3, $4)`,
		id, userID, verified, expires,
	)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    id,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (m *Manager) Verify(r *http.Request) error {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(
		`UPDATE sessions SET verified = true, expires_at = $1 WHERE id = $2`,
		time.Now().Add(TTL), c.Value,
	)
	return err
}

// Get returns user info only for fully verified sessions.
func (m *Manager) Get(r *http.Request) (*UserInfo, error) {
	return m.get(r, true)
}

// GetPending returns user info for unverified (2FA pending) sessions.
func (m *Manager) GetPending(r *http.Request) (*UserInfo, error) {
	return m.get(r, false)
}

func (m *Manager) get(r *http.Request, verified bool) (*UserInfo, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return nil, err
	}
	var info UserInfo
	err = m.db.QueryRow(`
		SELECT s.user_id, u.login, u.display_name, u.avatar_path,
		       EXISTS(SELECT 1 FROM totp_secrets WHERE user_id = u.id AND verified = true) AS has_totp,
		       EXISTS(SELECT 1 FROM webauthn_credentials WHERE user_id = u.id) AS has_webauthn
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = $1 AND s.expires_at > NOW() AND s.verified = $2`,
		c.Value, verified,
	).Scan(&info.UserID, &info.Login, &info.DisplayName, &info.AvatarPath, &info.HasTOTP, &info.HasWebAuthn)
	if err != nil {
		return nil, err
	}
	info.Has2FA = info.HasTOTP || info.HasWebAuthn
	return &info, nil
}

func (m *Manager) Destroy(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return
	}
	m.db.Exec(`DELETE FROM sessions WHERE id = $1`, c.Value)
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *Manager) Cleanup() {
	m.db.Exec(`DELETE FROM sessions WHERE expires_at < NOW()`)
}

func generateID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
