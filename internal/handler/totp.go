package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/nananek/nekomisu-diary/internal/dbq"
	"github.com/nananek/nekomisu-diary/internal/ratelimit"
	"github.com/nananek/nekomisu-diary/internal/session"
	"github.com/pquerna/otp/totp"
)

type TOTPHandler struct {
	q    *dbq.Queries
	sess *session.Manager
	rate *ratelimit.Limiter
}

func NewTOTPHandler(db *sql.DB, sess *session.Manager) *TOTPHandler {
	return &TOTPHandler{q: dbq.New(db), sess: sess}
}

func (h *TOTPHandler) WithRateLimit(l *ratelimit.Limiter) *TOTPHandler {
	h.rate = l
	return h
}

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
	if err := h.q.UpsertTOTPSecret(r.Context(), dbq.UpsertTOTPSecretParams{
		UserID: u.UserID,
		Secret: key.Secret(),
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, M{"secret": key.Secret(), "url": key.URL()})
}

func (h *TOTPHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	var req struct{ Code string `json:"code"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}
	secret, err := h.q.GetUnverifiedTOTPSecret(r.Context(), u.UserID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "no pending TOTP setup"})
		return
	}
	if !totp.Validate(req.Code, secret) {
		writeJSON(w, http.StatusUnauthorized, M{"error": "invalid code"})
		return
	}
	if err := h.q.ConfirmTOTP(r.Context(), u.UserID); err != nil {
		logIfErr("totp.Confirm.update", err)
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func (h *TOTPHandler) Disable(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	logIfErr("totp.Disable", h.q.DisableTOTP(r.Context(), u.UserID))
	writeJSON(w, http.StatusOK, M{"ok": true})
}

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
	var req struct{ Code string `json:"code"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}
	secret, err := h.q.GetVerifiedTOTPSecret(r.Context(), u.UserID)
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
	if h.rate != nil {
		h.rate.Reset("2fa:" + u.Login)
		h.rate.Reset("2fa-ip:" + clientIP(r))
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}
