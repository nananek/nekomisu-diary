// mockmisskey stands in for a real Misskey instance in e2e tests: just
// enough of the MiAuth flow (https://misskey-hub.net/docs/for-developers/api/token/miauth/)
// and the couple of plain API endpoints internal/handler/miauth.go and
// cmd/miauthlink talk to, so the diary's actual MiAuth login path can be
// driven through a real browser instead of being skipped in e2e because
// there's no real Misskey instance to hit. Not used in production.
//
// Every approval on /miauth/{token} is auto-granted (no consent screen) to
// a single fixed account, configured with -username, since the point here
// is exercising the diary side, not re-implementing Misskey's UI.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/url"
	"sync"
)

type approval struct {
	id       string
	username string
}

func main() {
	addr := flag.String("addr", ":3001", "listen address")
	username := flag.String("username", "e2enekomisu", "misskey username every /miauth/{token} approval grants")
	flag.Parse()

	id := "mi-" + *username

	var mu sync.Mutex
	approved := map[string]approval{} // miauth session token -> approval

	mux := http.NewServeMux()

	// Resolved by cmd/miauthlink at link time, and by nothing else.
	mux.HandleFunc("POST /api/users/show", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":       "mi-" + req.Username,
			"username": req.Username,
			"host":     nil,
		})
	})

	// The consent screen a real instance shows before redirecting back to
	// the callback URL. Auto-approves instead of waiting on a human.
	mux.HandleFunc("GET /miauth/{token}", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		callback := r.URL.Query().Get("callback")
		if callback == "" {
			http.Error(w, "missing callback", http.StatusBadRequest)
			return
		}
		mu.Lock()
		approved[token] = approval{id: id, username: *username}
		mu.Unlock()

		u, err := url.Parse(callback)
		if err != nil {
			http.Error(w, "bad callback", http.StatusBadRequest)
			return
		}
		q := u.Query()
		q.Set("session", token)
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	})

	// checkSession's target. Single-use, like the real endpoint.
	mux.HandleFunc("POST /api/miauth/{token}/check", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		mu.Lock()
		a, ok := approved[token]
		if ok {
			delete(approved, token)
		}
		mu.Unlock()
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"token": "at-" + token,
			"user":  map[string]any{"id": a.id, "username": a.username},
		})
	})

	// revokeToken's target. Mirrors the real self-revoke-only rule for
	// parity with internal/handler/miauth_test.go's mock, though nothing
	// in this e2e suite currently asserts on it.
	mux.HandleFunc("POST /api/i/revoke-token", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			I     string `json:"i"`
			Token string `json:"token"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.I == "" || body.I != body.Token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	log.Printf("mockmisskey: listening on %s, auto-approving as @%s (id=%s)", *addr, *username, id)
	log.Fatal(http.ListenAndServe(*addr, logRequests(mux)))
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("mockmisskey: %s %s", r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
