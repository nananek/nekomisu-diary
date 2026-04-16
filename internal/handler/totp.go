package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/nananek/nekomisu-diary/internal/session"
	"github.com/pquerna/otp/totp"
)

type TOTPHandler struct {
	db   *sql.DB
	sess *session.Manager
}

func NewTOTPHandler(db *sql.DB, sess *session.Manager) *TOTPHandler {
	return &TOTPHandler{db: db, sess: sess}
}

// Setup generates a new TOTP secret and returns the provisioning URI.
func (h *TOTPHandler) Setup(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Exchange Diary",
		AccountName: u.Login,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "totp generation failed"})
		return
	}

	// Upsert: replace any existing unverified secret
	h.db.Exec(`DELETE FROM totp_secrets WHERE user_id = $1`, u.UserID)
	_, err = h.db.Exec(
		`INSERT INTO totp_secrets (user_id, secret, verified) VALUES ($1, $2, false)`,
		u.UserID, key.Secret(),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}

	writeJSON(w, http.StatusOK, M{
		"secret": key.Secret(),
		"url":    key.URL(),
	})
}

// Confirm verifies the code and marks the TOTP secret as active.
func (h *TOTPHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}

	var secret string
	err := h.db.QueryRow(
		`SELECT secret FROM totp_secrets WHERE user_id = $1 AND verified = false`, u.UserID,
	).Scan(&secret)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "no pending TOTP setup"})
		return
	}

	if !totp.Validate(req.Code, secret) {
		writeJSON(w, http.StatusUnauthorized, M{"error": "invalid code"})
		return
	}

	h.db.Exec(`UPDATE totp_secrets SET verified = true WHERE user_id = $1`, u.UserID)
	writeJSON(w, http.StatusOK, M{"ok": true})
}

// Disable removes the TOTP secret.
func (h *TOTPHandler) Disable(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	h.db.Exec(`DELETE FROM totp_secrets WHERE user_id = $1`, u.UserID)
	writeJSON(w, http.StatusOK, M{"ok": true})
}

// Verify2FA is called during login to complete the 2FA step.
func (h *TOTPHandler) Verify2FA(w http.ResponseWriter, r *http.Request) {
	u, err := h.sess.GetPending(r)
	if err != nil || u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "no pending 2FA session"})
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}

	var secret string
	err = h.db.QueryRow(
		`SELECT secret FROM totp_secrets WHERE user_id = $1 AND verified = true`, u.UserID,
	).Scan(&secret)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "TOTP not configured"})
		return
	}

	if !totp.Validate(req.Code, secret) {
		writeJSON(w, http.StatusUnauthorized, M{"error": "invalid code"})
		return
	}

	if err := h.sess.Verify(r); err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "session error"})
		return
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}
