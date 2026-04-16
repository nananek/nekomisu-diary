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
)

type Manager struct {
	db *sql.DB
}

func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

type UserInfo struct {
	UserID      int64
	Login       string
	DisplayName string
	AvatarPath  sql.NullString
	Has2FA      bool
}

func (m *Manager) Create(w http.ResponseWriter, userID int64) error {
	id, err := generateID()
	if err != nil {
		return err
	}
	expires := time.Now().Add(TTL)
	_, err = m.db.Exec(
		`INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, $3)`,
		id, userID, expires,
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
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (m *Manager) Get(r *http.Request) (*UserInfo, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return nil, err
	}
	var info UserInfo
	err = m.db.QueryRow(`
		SELECT s.user_id, u.login, u.display_name, u.avatar_path,
		       EXISTS(SELECT 1 FROM totp_secrets WHERE user_id = u.id AND verified = true)
		         OR EXISTS(SELECT 1 FROM webauthn_credentials WHERE user_id = u.id) AS has_2fa
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = $1 AND s.expires_at > NOW()`,
		c.Value,
	).Scan(&info.UserID, &info.Login, &info.DisplayName, &info.AvatarPath, &info.Has2FA)
	if err != nil {
		return nil, err
	}
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
