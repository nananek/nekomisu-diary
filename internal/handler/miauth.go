package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nananek/nekomisu-diary/internal/dbq"
	"github.com/nananek/nekomisu-diary/internal/ratelimit"
	"github.com/nananek/nekomisu-diary/internal/session"
)

// MiAuth lets a member sign in by authenticating with a Misskey account
// instead of a password (https://misskey-hub.net/docs/for-developers/api/token/miauth/).
// Which Misskey account maps to which diary account is never self-service:
// an operator sets that link with cmd/miauthlink. A Misskey account that
// isn't linked cannot sign in, no matter how it authenticates upstream.
const miAuthPendingTTL = 10 * time.Minute

type MiAuthHandler struct {
	q        *dbq.Queries
	sess     *session.Manager
	instance string // Misskey instance origin, e.g. "https://kamisato.tail2c8c7.ts.net"
	appName  string
	callback string // full URL Misskey redirects back to after approval
	client   *http.Client
	rate     *ratelimit.Limiter
}

// NewMiAuthHandler wires up MiAuth login against a single Misskey
// instance. instance is that instance's origin (scheme + host, no trailing
// slash required); publicOrigin is this diary's own public origin, used to
// build the callback URL Misskey redirects the browser back to.
func NewMiAuthHandler(db *sql.DB, sess *session.Manager, instance, appName, publicOrigin string) *MiAuthHandler {
	return &MiAuthHandler{
		q:        dbq.New(db),
		sess:     sess,
		instance: strings.TrimRight(instance, "/"),
		appName:  appName,
		callback: strings.TrimRight(publicOrigin, "/") + "/login",
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *MiAuthHandler) WithRateLimit(l *ratelimit.Limiter) *MiAuthHandler {
	h.rate = l
	return h
}

// Enabled reports whether MiAuth login was configured (-misskey-instance).
// Routes stay registered either way; handlers just 404 when disabled so
// deployments that don't use it see a clean "not found" instead of a
// dangling link.
func (h *MiAuthHandler) Enabled() bool { return h.instance != "" }

// --- pending session store ---
//
// Tracks MiAuth session tokens this server itself issued, so Finish only
// ever acts on a session that started here (mirrors the wa_disc /
// waSessionStore pattern used for WebAuthn passkey sign-in). The token is
// generated with crypto/rand (via randomKey, shared with webauthn.go) and
// is the bearer secret for the whole exchange, exactly as the MiAuth
// protocol intends — knowledge of it is what lets Finish call the
// instance's /check endpoint at all.

var (
	miAuthMu    sync.Mutex
	miAuthStore = map[string]time.Time{}
)

func miAuthPut(token string) {
	miAuthMu.Lock()
	defer miAuthMu.Unlock()
	miAuthStore[token] = time.Now().Add(miAuthPendingTTL)
	for k, exp := range miAuthStore {
		if time.Now().After(exp) {
			delete(miAuthStore, k)
		}
	}
}

func miAuthTake(token string) bool {
	miAuthMu.Lock()
	defer miAuthMu.Unlock()
	exp, ok := miAuthStore[token]
	if !ok {
		return false
	}
	delete(miAuthStore, token)
	return time.Now().Before(exp)
}

// Config tells the frontend whether MiAuth login is available at all, so
// it can hide the "Misskeyでログイン" button on deployments that haven't
// configured -misskey-instance instead of offering a button that 404s.
func (h *MiAuthHandler) Config(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, M{"enabled": h.Enabled()})
}

// Start issues a fresh MiAuth session and returns the authorization URL
// the frontend should send the browser to.
func (h *MiAuthHandler) Start(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled() {
		writeJSON(w, http.StatusNotFound, M{"error": "misskey login is not configured"})
		return
	}
	if h.rate != nil && !h.rate.Allow("miauth-start-ip:"+clientIP(r)) {
		writeJSON(w, http.StatusTooManyRequests, M{"error": "too many attempts, try again later"})
		return
	}

	token := randomKey()
	miAuthPut(token)

	// No `permission` scope is requested: we only need to identify the
	// approving Misskey account, not act on its behalf, so the consent
	// screen on the Misskey side stays to "confirm it's you."
	q := url.Values{}
	q.Set("name", h.appName)
	q.Set("callback", h.callback)

	writeJSON(w, http.StatusOK, M{
		"url": fmt.Sprintf("%s/miauth/%s?%s", h.instance, token, q.Encode()),
	})
}

type miAuthCheckResult struct {
	OK   bool `json:"ok"`
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
}

// Finish is called by the frontend once the browser lands back on the
// callback URL with ?session=<token>. It verifies the session against the
// configured Misskey instance and, if the resulting account is linked to
// a diary account, signs that account in directly — like passkey
// discoverable login, a successful MiAuth exchange is treated as a strong
// enough credential on its own, so the diary's own 2FA step is skipped
// even if the linked account has TOTP/WebAuthn configured. (2FA on the
// Misskey side, if any, already gated getting here.)
func (h *MiAuthHandler) Finish(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled() {
		writeJSON(w, http.StatusNotFound, M{"error": "misskey login is not configured"})
		return
	}
	if h.rate != nil && !h.rate.Allow("miauth-finish-ip:"+clientIP(r)) {
		writeJSON(w, http.StatusTooManyRequests, M{"error": "too many attempts, try again later"})
		return
	}

	var req struct {
		Session string `json:"session"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Session == "" {
		writeJSON(w, http.StatusBadRequest, M{"error": "invalid request"})
		return
	}
	if !miAuthTake(req.Session) {
		writeJSON(w, http.StatusBadRequest, M{"error": "expired or unknown miauth session"})
		return
	}

	check, err := h.checkSession(r.Context(), req.Session)
	if err != nil || !check.OK || check.User.ID == "" {
		writeJSON(w, http.StatusUnauthorized, M{"error": "misskey authentication failed"})
		return
	}

	userID, err := h.q.GetUserIDByMisskeyAccount(r.Context(), dbq.GetUserIDByMisskeyAccountParams{
		MisskeyInstance: h.instance,
		MisskeyUserID:   check.User.ID,
	})
	if err != nil {
		writeJSON(w, http.StatusForbidden, M{"error": "このMisskeyアカウントは連携されていません"})
		return
	}

	if err := h.sess.Create(w, userID, true); err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "session error"})
		return
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}

// checkSession calls POST {instance}/api/miauth/{token}/check. No auth is
// required for this endpoint on the Misskey side — the token itself is the
// bearer secret, and it's single-use there regardless of what we do here.
func (h *MiAuthHandler) checkSession(ctx context.Context, token string) (*miAuthCheckResult, error) {
	endpoint := fmt.Sprintf("%s/api/miauth/%s/check", h.instance, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("miauth check: unexpected status %d", resp.StatusCode)
	}
	var out miAuthCheckResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
