package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/nananek/nekomisu-diary/internal/ratelimit"
	"github.com/nananek/nekomisu-diary/internal/session"
)

type WebAuthnHandler struct {
	db   *sql.DB
	sess *session.Manager
	wa   *webauthn.WebAuthn
	rate *ratelimit.Limiter // nil ok; limits LoginFinish per login name
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
	return &WebAuthnHandler{db: db, sess: sess, wa: wa}, nil
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
func (u *waUser) WebAuthnName() string        { return u.name }
func (u *waUser) WebAuthnDisplayName() string  { return u.displayName }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (h *WebAuthnHandler) loadUser(userID int64) (*waUser, error) {
	var login, displayName string
	err := h.db.QueryRow(`SELECT login, display_name FROM users WHERE id = $1`, userID).Scan(&login, &displayName)
	if err != nil {
		return nil, err
	}
	u := &waUser{id: userID, name: login, displayName: displayName}

	rows, err := h.db.Query(`SELECT id, public_key, attestation_type, sign_count, transports FROM webauthn_credentials WHERE user_id = $1`, userID)
	if err != nil {
		return u, nil
	}
	defer rows.Close()

	for rows.Next() {
		var cred webauthn.Credential
		var credID string
		var attType sql.NullString
		var transports []string
		if err := rows.Scan(&credID, &cred.PublicKey, &attType, &cred.Authenticator.SignCount, (*pgTextArray)(&transports)); err != nil {
			continue
		}
		cred.ID = []byte(credID)
		if attType.Valid {
			cred.AttestationType = attType.String
		}
		for _, t := range transports {
			cred.Transport = append(cred.Transport, protocol.AuthenticatorTransport(t))
		}
		u.credentials = append(u.credentials, cred)
	}
	return u, nil
}

// --- WebAuthn ceremony session store ---
//
// In-memory, mutex-protected, with a TTL sweep. Each registration/login
// ceremony stores a short-lived SessionData keyed by "purpose:login".
// Abandoned entries (no matching Finish) are reaped after waSessionTTL.

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

func (h *WebAuthnHandler) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	waU, err := h.loadUser(u.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "user load error"})
		return
	}

	options, sessionData, err := h.wa.BeginRegistration(waU)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": err.Error()})
		return
	}

	waSessionPut("reg:"+u.Login, sessionData)
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

	// Credential name comes from a query param; body is consumed by
	// FinishRegistration below.
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Security Key"
	}

	waU, _ := h.loadUser(u.UserID)
	cred, err := h.wa.FinishRegistration(waU, *sd, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": err.Error()})
		return
	}

	transports := make([]string, len(cred.Transport))
	for i, t := range cred.Transport {
		transports[i] = string(t)
	}

	_, err = h.db.Exec(`
		INSERT INTO webauthn_credentials (id, user_id, name, public_key, attestation_type, sign_count, transports)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		string(cred.ID), u.UserID, name, cred.PublicKey, cred.AttestationType,
		cred.Authenticator.SignCount, (*pgTextArray)(&transports),
	)
	if err != nil {
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
	h.db.Exec(`DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2`, credID, u.UserID)
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func (h *WebAuthnHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	rows, err := h.db.Query(`SELECT id, name, created_at FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at`, u.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	defer rows.Close()

	var creds []M
	for rows.Next() {
		var id, name, createdAt string
		if rows.Scan(&id, &name, &createdAt) == nil {
			creds = append(creds, M{"id": id, "name": name, "created_at": createdAt})
		}
	}
	if creds == nil {
		creds = []M{}
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

	waU, err := h.loadUser(u.UserID)
	if err != nil || len(waU.credentials) == 0 {
		writeJSON(w, http.StatusBadRequest, M{"error": "no webauthn credentials"})
		return
	}

	options, sessionData, err := h.wa.BeginLogin(waU)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": err.Error()})
		return
	}

	waSessionPut("login:"+u.Login, sessionData)
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

	waU, _ := h.loadUser(u.UserID)
	cred, err := h.wa.FinishLogin(waU, *sd, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": err.Error()})
		return
	}

	// Update sign count
	h.db.Exec(`UPDATE webauthn_credentials SET sign_count = $1 WHERE id = $2`,
		cred.Authenticator.SignCount, string(cred.ID))

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

// pgTextArray implements sql.Scanner for PostgreSQL TEXT[] type
type pgTextArray []string

func (a *pgTextArray) Scan(src any) error {
	if src == nil {
		*a = nil
		return nil
	}
	// PostgreSQL returns TEXT[] as a string like {a,b,c}
	s, ok := src.(string)
	if !ok {
		if b, ok2 := src.([]byte); ok2 {
			s = string(b)
		}
	}
	if len(s) < 2 || s[0] != '{' {
		*a = nil
		return nil
	}
	s = s[1 : len(s)-1]
	if s == "" {
		*a = nil
		return nil
	}
	result := []string{}
	for _, item := range splitPgArray(s) {
		result = append(result, item)
	}
	*a = result
	return nil
}

func splitPgArray(s string) []string {
	var result []string
	var cur []byte
	quoted := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			quoted = !quoted
		case s[i] == ',' && !quoted:
			result = append(result, string(cur))
			cur = nil
		default:
			cur = append(cur, s[i])
		}
	}
	if len(cur) > 0 {
		result = append(result, string(cur))
	}
	return result
}

func (a pgTextArray) Value() (any, error) {
	if a == nil {
		return nil, nil
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, v := range a {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		// Escape backslashes and double quotes per PostgreSQL array literal rules.
		for _, r := range v {
			switch r {
			case '\\', '"':
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String(), nil
}
