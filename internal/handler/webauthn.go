package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/nananek/nekomisu-diary/internal/session"
)

type WebAuthnHandler struct {
	db   *sql.DB
	sess *session.Manager
	wa   *webauthn.WebAuthn
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

// --- Registration ---

// sessionStore holds WebAuthn session data in memory (adequate for 3 users)
var waSessionStore = make(map[string]*webauthn.SessionData)

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

	waSessionStore[u.Login] = sessionData
	writeJSON(w, http.StatusOK, options)
}

func (h *WebAuthnHandler) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	sd, ok := waSessionStore[u.Login]
	if !ok {
		writeJSON(w, http.StatusBadRequest, M{"error": "no pending registration"})
		return
	}
	delete(waSessionStore, u.Login)

	waU, _ := h.loadUser(u.UserID)
	cred, err := h.wa.FinishRegistration(waU, *sd, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": err.Error()})
		return
	}

	var name string
	json.NewDecoder(r.Body).Decode(&struct{ Name *string }{&name})
	if name == "" {
		name = "Security Key"
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

	waSessionStore["login:"+u.Login] = sessionData
	writeJSON(w, http.StatusOK, options)
}

func (h *WebAuthnHandler) LoginFinish(w http.ResponseWriter, r *http.Request) {
	u, err := h.sess.GetPending(r)
	if err != nil || u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "no pending 2FA session"})
		return
	}

	sd, ok := waSessionStore["login:"+u.Login]
	if !ok {
		writeJSON(w, http.StatusBadRequest, M{"error": "no pending login"})
		return
	}
	delete(waSessionStore, "login:"+u.Login)

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
	s := "{"
	for i, v := range a {
		if i > 0 {
			s += ","
		}
		s += `"` + v + `"`
	}
	s += "}"
	return s, nil
}
