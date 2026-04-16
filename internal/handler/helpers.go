package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/nananek/nekomisu-diary/internal/session"
)

type M map[string]any

type contextKey string

const userKey contextKey = "user"

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func UserFromContext(ctx context.Context) *session.UserInfo {
	u, _ := ctx.Value(userKey).(*session.UserInfo)
	return u
}

func ContextWithUser(ctx context.Context, u *session.UserInfo) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func nullStr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFromContext(r.Context()) == nil {
			writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
