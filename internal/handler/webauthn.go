package handler

import (
	"context"
	"database/sql"
	"encoding/base64"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/nananek/nekomisu-diary/internal/dbq"
	"github.com/nananek/nekomisu-diary/internal/ratelimit"
	"github.com/nananek/nekomisu-diary/internal/session"
)

// Credential IDs are arbitrary bytes; stored base64url-encoded in TEXT.
func encodeCredID(id []byte) string { return base64.RawURLEncoding.EncodeToString(id) }
func decodeCredID(s string) []byte {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b
	}
	return []byte(s)
}

type WebAuthnHandler struct {
	q    *dbq.Queries
	sess *session.Manager
	wa   *webauthn.WebAuthn
	rate *ratelimit.Limiter
}

func (h *WebAuthnHandler) WithRateLimit(l *ratelimit.Limiter) *WebAuthnHandler {
	h.rate = l
	return h
}

func NewWebAuthnHandler(db *sql.DB, sess *session.Manager, rpID, rpOrigin string) (*WebAuthnHandler, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Exchange Diary",
		RPID:          rpID,
		RPOrigins:     []string{rpOrigin},
	})
	if err != nil {
		return nil, err
	}
	return &WebAuthnHandler{q: dbq.New(db), sess: sess, wa: wa}, nil
}

// waUser implements webauthn.User
type waUser struct {
	id          int64
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte {
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[i] = byte(u.id >> (i * 8))
	}
	return b
}
func (u *waUser) WebAuthnName() string                       { return u.name }
func (u *waUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (h *WebAuthnHandler) loadUser(ctx context.Context, userID int64) (*waUser, error) {
	info, err := h.q.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	u := &waUser{id: userID, name: info.Login, displayName: info.DisplayName}

	rows, err := h.q.LoadWebAuthnCredentials(ctx, userID)
	if err != nil {
		return u, nil
	}
	for _, rr := range rows {
		var cred webauthn.Credential
		cred.ID = decodeCredID(rr.ID)
		cred.PublicKey = rr.PublicKey
		if rr.AttestationType.Valid {
			cred.AttestationType = rr.AttestationType.String
		}
		cred.Authenticator.SignCount = uint32(rr.SignCount)
		for _, t := range rr.Transports {
			cred.Transport = append(cred.Transport, protocol.AuthenticatorTransport(t))
		}
		cred.Flags.BackupEligible = rr.FlagBackupEligible
		cred.Flags.BackupState = rr.FlagBackupState
		u.credentials = append(u.credentials, cred)
	}
	return u, nil
}

// --- WebAuthn ceremony session store ---

const waSessionTTL = 5 * time.Minute

type waSessionEntry struct {
	data    *webauthn.SessionData
	expires time.Time
}

var (
	waSessionMu    sync.Mutex
	waSessionStore = map[string]waSessionEntry{}
)

func waSessionPut(key string, sd *webauthn.SessionData) {
	waSessionMu.Lock()
	defer waSessionMu.Unlock()
	waSessionStore[key] = waSessionEntry{data: sd, expires: time.Now().Add(waSessionTTL)}
	waSessionSweepLocked()
}

func waSessionTake(key string) (*webauthn.SessionData, bool) {
	waSessionMu.Lock()
	defer waSessionMu.Unlock()
	e, ok := waSessionStore[key]
	if !ok {
		return nil, false
	}
	delete(waSessionStore, key)
	if time.Now().After(e.expires) {
		return nil, false
	}
	return e.data, true
}

func waSessionSweepLocked() {
	now := time.Now()
	for k, e := range waSessionStore {
		if now.After(e.expires) {
			delete(waSessionStore, k)
		}
	}
}

// --- Registration ---

func (h *WebAuthnHandler) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	waU, err := h.loadUser(r.Context(), u.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "user load error"})
		return
	}
	options, sd, err := h.wa.BeginRegistration(waU)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": err.Error()})
		return
	}
	waSessionPut("reg:"+u.Login, sd)
	writeJSON(w, http.StatusOK, options)
}

func (h *WebAuthnHandler) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	sd, ok := waSessionTake("reg:" + u.Login)
	if !ok {
		writeJSON(w, http.StatusBadRequest, M{"error": "no pending registration"})
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Security Key"
	}

	waU, _ := h.loadUser(r.Context(), u.UserID)
	cred, err := h.wa.FinishRegistration(waU, *sd, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": err.Error()})
		return
	}

	transports := make([]string, len(cred.Transport))
	for i, t := range cred.Transport {
		transports[i] = string(t)
	}

	if err := h.q.CreateWebAuthnCredential(r.Context(), dbq.CreateWebAuthnCredentialParams{
		ID:                 encodeCredID(cred.ID),
		UserID:             u.UserID,
		Name:               name,
		PublicKey:          cred.PublicKey,
		AttestationType:    sql.NullString{String: cred.AttestationType, Valid: cred.AttestationType != ""},
		SignCount:          int64(cred.Authenticator.SignCount),
		Transports:         transports,
		FlagBackupEligible: cred.Flags.BackupEligible,
		FlagBackupState:    cred.Flags.BackupState,
	}); err != nil {
		log.Printf("webauthn insert: %v", err)
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func (h *WebAuthnHandler) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	credID := r.PathValue("credId")
	logIfErr("webauthn.Delete", h.q.DeleteWebAuthnCredential(r.Context(), dbq.DeleteWebAuthnCredentialParams{
		ID:     credID,
		UserID: u.UserID,
	}))
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func (h *WebAuthnHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	rows, err := h.q.ListWebAuthnCredentials(r.Context(), u.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	creds := make([]M, 0, len(rows))
	for _, rr := range rows {
		creds = append(creds, M{
			"id":         rr.ID,
			"name":       rr.Name,
			"created_at": rr.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, M{"credentials": creds})
}

// --- Login ---

func (h *WebAuthnHandler) LoginBegin(w http.ResponseWriter, r *http.Request) {
	u, err := h.sess.GetPending(r)
	if err != nil || u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "no pending 2FA session"})
		return
	}
	waU, err := h.loadUser(r.Context(), u.UserID)
	if err != nil || len(waU.credentials) == 0 {
		writeJSON(w, http.StatusBadRequest, M{"error": "no webauthn credentials"})
		return
	}
	options, sd, err := h.wa.BeginLogin(waU)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": err.Error()})
		return
	}
	waSessionPut("login:"+u.Login, sd)
	writeJSON(w, http.StatusOK, options)
}

func (h *WebAuthnHandler) LoginFinish(w http.ResponseWriter, r *http.Request) {
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
	sd, ok := waSessionTake("login:" + u.Login)
	if !ok {
		writeJSON(w, http.StatusBadRequest, M{"error": "no pending login"})
		return
	}
	waU, _ := h.loadUser(r.Context(), u.UserID)
	cred, err := h.wa.FinishLogin(waU, *sd, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": err.Error()})
		return
	}
	logIfErr("webauthn.LoginFinish.sign_count", h.q.UpdateWebAuthnSignCount(r.Context(), dbq.UpdateWebAuthnSignCountParams{
		SignCount: int64(cred.Authenticator.SignCount),
		ID:        encodeCredID(cred.ID),
	}))
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
