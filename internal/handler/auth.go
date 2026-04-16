package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/nananek/nekomisu-diary/internal/session"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db   *sql.DB
	sess *session.Manager
}

func NewAuthHandler(db *sql.DB, sess *session.Manager) *AuthHandler {
	return &AuthHandler{db: db, sess: sess}
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

	if err := h.sess.Create(w, userID); err != nil {
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
	})
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
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

	if err := h.sess.Create(w, userID); err != nil {
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
