package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/nananek/nekomisu-diary/internal/ratelimit"
	"github.com/nananek/nekomisu-diary/internal/session"
	"github.com/pquerna/otp/totp"
)

type TOTPHandler struct {
	db   *sql.DB
	sess *session.Manager
	rate *ratelimit.Limiter // nil ok; limits Verify2FA per login name
}

func NewTOTPHandler(db *sql.DB, sess *session.Manager) *TOTPHandler {
	return &TOTPHandler{db: db, sess: sess}
}

func (h *TOTPHandler) WithRateLimit(l *ratelimit.Limiter) *TOTPHandler {
	h.rate = l
	return h
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

	// Upsert: replace any existing secret atomically so a failed insert
	// doesn't leave the user without a TOTP secret.
	_, err = h.db.Exec(`
		INSERT INTO totp_secrets (user_id, secret, verified)
		VALUES ($1, $2, false)
		ON CONFLICT (user_id) DO UPDATE SET secret = EXCLUDED.secret, verified = false`,
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

	_, err = h.db.Exec(`UPDATE totp_secrets SET verified = true WHERE user_id = $1`, u.UserID)
	logIfErr("totp.Confirm.update", err)
	writeJSON(w, http.StatusOK, M{"ok": true})
}

// Disable removes the TOTP secret.
func (h *TOTPHandler) Disable(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	_, err := h.db.Exec(`DELETE FROM totp_secrets WHERE user_id = $1`, u.UserID)
	logIfErr("totp.Disable", err)
	writeJSON(w, http.StatusOK, M{"ok": true})
}

// Verify2FA is called during login to complete the 2FA step.
func (h *TOTPHandler) Verify2FA(w http.ResponseWriter, r *http.Request) {
	u, err := h.sess.GetPending(r)
	if err != nil || u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "no pending 2FA session"})
		return
	}

	if h.rate != nil {
		if !h.rate.Allow("2fa:"+u.Login) || !h.rate.Allow("2fa-ip:"+clientIP(r)) {
			writeJSON(w, http.StatusTooManyRequests, M{"error": "too many attempts, try again later"})
			return
		}
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
	// Success — clear the 2FA counters
	if h.rate != nil {
		h.rate.Reset("2fa:" + u.Login)
		h.rate.Reset("2fa-ip:" + clientIP(r))
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}
