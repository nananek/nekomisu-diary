package handler

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"

	"github.com/nananek/nekomisu-diary/internal/ratelimit"
	"github.com/nananek/nekomisu-diary/internal/session"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db                 *sql.DB
	sess               *session.Manager
	loginRate          *ratelimit.Limiter // nil ok
	twoFARate          *ratelimit.Limiter // nil ok
	allowRegistration  bool
}

func NewAuthHandler(db *sql.DB, sess *session.Manager) *AuthHandler {
	return &AuthHandler{db: db, sess: sess}
}

// WithRateLimit enables rate limiting on login / 2FA verify endpoints.
// loginRate applies to password login; twoFARate to TOTP / WebAuthn
// verification after login succeeds.
func (h *AuthHandler) WithRateLimit(loginRate, twoFARate *ratelimit.Limiter) *AuthHandler {
	h.loginRate = loginRate
	h.twoFARate = twoFARate
	return h
}

// AllowRegistration controls whether POST /api/auth/register is available.
// Default is false: the endpoint returns 403 unless explicitly enabled.
func (h *AuthHandler) AllowRegistration(allow bool) *AuthHandler {
	h.allowRegistration = allow
	return h
}

// TwoFARate exposes the 2FA limiter so other handlers (WebAuthn) can
// share the same bucket.
func (h *AuthHandler) TwoFARate() *ratelimit.Limiter { return h.twoFARate }

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}

	ip := clientIP(r)
	if h.loginRate != nil {
		if !h.loginRate.Allow("ip:"+ip) || !h.loginRate.Allow("login:"+req.Login) {
			writeJSON(w, http.StatusTooManyRequests, M{"error": "too many attempts, try again later"})
			return
		}
	}

	var userID int64
	var hash string
	err := h.db.QueryRow(
		`SELECT id, password_hash FROM users WHERE login = $1`, req.Login,
	).Scan(&userID, &hash)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "invalid credentials"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "invalid credentials"})
		return
	}

	// Successful auth: reset counters
	if h.loginRate != nil {
		h.loginRate.Reset("ip:" + ip)
		h.loginRate.Reset("login:" + req.Login)
	}

	// Check if user has 2FA enabled
	var hasTotp, hasWebauthn bool
	h.db.QueryRow(`
		SELECT
			EXISTS(SELECT 1 FROM totp_secrets WHERE user_id = $1 AND verified = true),
			EXISTS(SELECT 1 FROM webauthn_credentials WHERE user_id = $1)`,
		userID).Scan(&hasTotp, &hasWebauthn)

	if hasTotp || hasWebauthn {
		if err := h.sess.Create(w, userID, false); err != nil {
			writeJSON(w, http.StatusInternalServerError, M{"error": "session error"})
			return
		}
		writeJSON(w, http.StatusOK, M{
			"requires_2fa": true,
			"has_totp":     hasTotp,
			"has_webauthn": hasWebauthn,
		})
		return
	}

	if err := h.sess.Create(w, userID, true); err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "session error"})
		return
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.sess.Destroy(w, r)
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	writeJSON(w, http.StatusOK, M{
		"id":           u.UserID,
		"login":        u.Login,
		"display_name": u.DisplayName,
		"avatar_path":  nullStr(u.AvatarPath),
		"has_2fa":      u.Has2FA,
		"has_totp":     u.HasTOTP,
		"has_webauthn": u.HasWebAuthn,
	})
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.allowRegistration {
		writeJSON(w, http.StatusForbidden, M{"error": "registration is disabled"})
		return
	}
	var req struct {
		Login       string `json:"login"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}
	if req.Login == "" || req.Email == "" || req.DisplayName == "" || len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, M{"error": "login, email, display_name required; password min 8 chars"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "internal error"})
		return
	}

	var userID int64
	err = h.db.QueryRow(`
		INSERT INTO users (login, email, display_name, password_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		req.Login, req.Email, req.DisplayName, string(hash),
	).Scan(&userID)
	if err != nil {
		writeJSON(w, http.StatusConflict, M{"error": "login or email already exists"})
		return
	}

	if err := h.sess.Create(w, userID, true); err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "session error"})
		return
	}
	writeJSON(w, http.StatusCreated, M{"ok": true, "id": userID})
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}
	if len(req.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, M{"error": "password min 8 chars"})
		return
	}

	var hash string
	h.db.QueryRow(`SELECT password_hash FROM users WHERE id = $1`, u.UserID).Scan(&hash)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)) != nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "old password incorrect"})
		return
	}

	newHash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	h.db.Exec(`UPDATE users SET password_hash = $1 WHERE id = $2`, string(newHash), u.UserID)
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	var req struct {
		DisplayName *string `json:"display_name"`
		Email       *string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}
	if req.DisplayName != nil {
		h.db.Exec(`UPDATE users SET display_name = $1 WHERE id = $2`, *req.DisplayName, u.UserID)
	}
	if req.Email != nil {
		_, err := h.db.Exec(`UPDATE users SET email = $1 WHERE id = $2`, *req.Email, u.UserID)
		if err != nil {
			writeJSON(w, http.StatusConflict, M{"error": "email already in use"})
			return
		}
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}
