package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/nananek/nekomisu-diary/internal/dbq"
)

const (
	CookieName = "session"
	// Fixed window is 180 days; rolling extension happens on every Get()
	// when less than rollingThreshold of the TTL remains, so active users
	// effectively never log out.
	TTL               = 180 * 24 * time.Hour
	rollingThreshold  = TTL / 2
	PendingTTL        = 5 * time.Minute
)

type Manager struct {
	q      *dbq.Queries
	db     *sql.DB // retained for Cleanup's raw exec; dbq generates this too
	secure bool
}

func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db, q: dbq.New(db)}
}

// WithSecureCookies marks the session cookie as Secure so browsers only
// send it over HTTPS. Must be true in production; false for plain-HTTP
// local development (e.g. http://localhost:3000).
func (m *Manager) WithSecureCookies(secure bool) *Manager {
	m.secure = secure
	return m
}

// SecureCookies reports whether cookies issued by this Manager have the
// Secure flag set.
func (m *Manager) SecureCookies() bool { return m.secure }

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
	if err := m.q.CreateSession(context.Background(), dbq.CreateSessionParams{
		ID:        id,
		UserID:    userID,
		Verified:  verified,
		ExpiresAt: expires,
	}); err != nil {
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
	return m.q.VerifySession(r.Context(), dbq.VerifySessionParams{
		ExpiresAt: time.Now().Add(TTL),
		ID:        c.Value,
	})
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
	row, err := m.q.GetSession(r.Context(), dbq.GetSessionParams{
		ID:       c.Value,
		Verified: verified,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, err
	}

	// Rolling session: if the remaining TTL is less than half the full
	// TTL, extend expires_at back to now+TTL so active users never log
	// out. Only applies to verified sessions (not 2FA-pending).
	if verified {
		remaining := time.Until(row.ExpiresAt)
		if remaining < rollingThreshold {
			newExpiry := time.Now().Add(TTL)
			_ = m.q.ExtendSessionExpiry(r.Context(), dbq.ExtendSessionExpiryParams{
				ExpiresAt: newExpiry,
				ID:        c.Value,
			})
		}
	}

	info := &UserInfo{
		UserID:      row.UserID,
		Login:       row.Login,
		DisplayName: row.DisplayName,
		AvatarPath:  row.AvatarPath,
		HasTOTP:     row.HasTotp,
		HasWebAuthn: row.HasWebauthn,
	}
	info.Has2FA = info.HasTOTP || info.HasWebAuthn
	return info, nil
}

func (m *Manager) Destroy(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(CookieName)
	if err == nil {
		_ = m.q.DeleteSession(r.Context(), c.Value)
	}
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
	_ = m.q.DeleteExpiredSessions(context.Background())
}

func generateID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
