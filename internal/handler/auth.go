package handler

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"

	"github.com/nananek/nekomisu-diary/internal/dbq"
	"github.com/nananek/nekomisu-diary/internal/ratelimit"
	"github.com/nananek/nekomisu-diary/internal/session"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	q                 *dbq.Queries
	sess              *session.Manager
	loginRate         *ratelimit.Limiter // nil ok
	twoFARate         *ratelimit.Limiter // nil ok
	allowRegistration bool
}

func NewAuthHandler(db *sql.DB, sess *session.Manager) *AuthHandler {
	return &AuthHandler{q: dbq.New(db), sess: sess}
}

func (h *AuthHandler) WithRateLimit(loginRate, twoFARate *ratelimit.Limiter) *AuthHandler {
	h.loginRate = loginRate
	h.twoFARate = twoFARate
	return h
}

func (h *AuthHandler) AllowRegistration(allow bool) *AuthHandler {
	h.allowRegistration = allow
	return h
}

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

	user, err := h.q.GetUserByLogin(r.Context(), req.Login)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "invalid credentials"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "invalid credentials"})
		return
	}

	has2fa, err := h.q.UserHas2FA(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "internal error"})
		return
	}

	if has2fa.HasTotp || has2fa.HasWebauthn {
		if err := h.sess.Create(w, user.ID, false); err != nil {
			writeJSON(w, http.StatusInternalServerError, M{"error": "session error"})
			return
		}
		writeJSON(w, http.StatusOK, M{
			"requires_2fa": true,
			"has_totp":     has2fa.HasTotp,
			"has_webauthn": has2fa.HasWebauthn,
		})
		return
	}

	if err := h.sess.Create(w, user.ID, true); err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "session error"})
		return
	}
	if h.loginRate != nil {
		h.loginRate.Reset("ip:" + ip)
		h.loginRate.Reset("login:" + req.Login)
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

	userID, err := h.q.CreateUser(r.Context(), dbq.CreateUserParams{
		Login:        req.Login,
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PasswordHash: string(hash),
	})
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

	hash, err := h.q.GetUserPasswordHash(r.Context(), u.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "internal error"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)) != nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "old password incorrect"})
		return
	}

	newHash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err := h.q.UpdateUserPassword(r.Context(), dbq.UpdateUserPasswordParams{
		PasswordHash: string(newHash),
		ID:           u.UserID,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "internal error"})
		return
	}
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
		_ = h.q.UpdateUserDisplayName(r.Context(), dbq.UpdateUserDisplayNameParams{
			DisplayName: *req.DisplayName,
			ID:          u.UserID,
		})
	}
	if req.Email != nil {
		if err := h.q.UpdateUserEmail(r.Context(), dbq.UpdateUserEmailParams{
			Email: *req.Email,
			ID:    u.UserID,
		}); err != nil {
			writeJSON(w, http.StatusConflict, M{"error": "email already in use"})
			return
		}
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}

