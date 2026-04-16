package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/nananek/nekomisu-diary/internal/db"
	"github.com/nananek/nekomisu-diary/internal/handler"
	"github.com/nananek/nekomisu-diary/internal/notifier"
	"github.com/nananek/nekomisu-diary/internal/session"
)

func main() {
	pgDSN := flag.String("pg", "postgres://diary:diary_dev_pw@postgres:5432/diary?sslmode=disable", "PostgreSQL DSN")
	addr := flag.String("addr", ":3000", "Listen address")
	uploadsDir := flag.String("uploads", "", "Path to uploads directory (for serving and storing media)")
	webDir := flag.String("web", "", "Path to frontend dist directory")
	rpID := flag.String("rp-id", "localhost", "WebAuthn Relying Party ID (domain)")
	rpOrigin := flag.String("rp-origin", "http://localhost:3000", "WebAuthn Relying Party origin")
	discordWebhook := flag.String("discord-webhook", "", "Discord webhook URL for post/comment notifications")
	flag.Parse()

	pool, err := db.Open(*pgDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	sess := session.NewManager(pool)

	go func() {
		for {
			sess.Cleanup()
			time.Sleep(1 * time.Hour)
		}
	}()

	disc := notifier.NewDiscord(*discordWebhook, *rpOrigin)
	if disc != nil {
		log.Printf("Discord notifier enabled")
	}

	auth := handler.NewAuthHandler(pool, sess)
	posts := handler.NewPostHandler(pool, disc)
	comments := handler.NewCommentHandler(pool, disc)
	totpH := handler.NewTOTPHandler(pool, sess)

	waH, err := handler.NewWebAuthnHandler(pool, sess, *rpID, *rpOrigin)
	if err != nil {
		log.Fatalf("webauthn: %v", err)
	}

	members := handler.NewMemberHandler(pool)

	var mediaH *handler.MediaHandler
	if *uploadsDir != "" {
		mediaH = handler.NewMediaHandler(pool, *uploadsDir)
	}

	mux := http.NewServeMux()

	// Auth (public)
	mux.HandleFunc("POST /api/auth/login", auth.Login)
	mux.HandleFunc("POST /api/auth/register", auth.Register)

	// 2FA login verification (pending session)
	mux.HandleFunc("POST /api/auth/totp/verify-login", totpH.Verify2FA)
	mux.HandleFunc("POST /api/auth/webauthn/login/begin", waH.LoginBegin)
	mux.HandleFunc("POST /api/auth/webauthn/login/finish", waH.LoginFinish)

	// Auth (session required)
	mux.Handle("POST /api/auth/logout", requireAuth(http.HandlerFunc(auth.Logout)))
	mux.Handle("GET /api/auth/me", injectUser(sess, http.HandlerFunc(auth.Me)))
	mux.Handle("PUT /api/auth/password", requireAuth(http.HandlerFunc(auth.ChangePassword)))
	mux.Handle("PUT /api/auth/profile", requireAuth(http.HandlerFunc(auth.UpdateProfile)))

	// TOTP management (session required)
	mux.Handle("POST /api/auth/totp/setup", requireAuth(http.HandlerFunc(totpH.Setup)))
	mux.Handle("POST /api/auth/totp/confirm", requireAuth(http.HandlerFunc(totpH.Confirm)))
	mux.Handle("DELETE /api/auth/totp", requireAuth(http.HandlerFunc(totpH.Disable)))

	// WebAuthn management (session required)
	mux.Handle("POST /api/auth/webauthn/register/begin", requireAuth(http.HandlerFunc(waH.RegisterBegin)))
	mux.Handle("POST /api/auth/webauthn/register/finish", requireAuth(http.HandlerFunc(waH.RegisterFinish)))
	mux.Handle("GET /api/auth/webauthn/credentials", requireAuth(http.HandlerFunc(waH.ListCredentials)))
	mux.Handle("DELETE /api/auth/webauthn/credentials/{credId}", requireAuth(http.HandlerFunc(waH.DeleteCredential)))

	// Posts (session required)
	mux.Handle("GET /api/posts", requireAuth(http.HandlerFunc(posts.List)))
	mux.Handle("GET /api/posts/{id}", requireAuth(http.HandlerFunc(posts.Get)))
	mux.Handle("POST /api/posts", requireAuth(http.HandlerFunc(posts.Create)))
	mux.Handle("PUT /api/posts/{id}", requireAuth(http.HandlerFunc(posts.Update)))
	mux.Handle("DELETE /api/posts/{id}", requireAuth(http.HandlerFunc(posts.Delete)))

	// Posts: drafts, search, by-author
	mux.Handle("GET /api/posts/drafts", requireAuth(http.HandlerFunc(posts.Drafts)))
	mux.Handle("GET /api/posts/search", requireAuth(http.HandlerFunc(posts.Search)))
	mux.Handle("GET /api/users/{userId}/posts", requireAuth(http.HandlerFunc(posts.ByAuthor)))

	// Members
	mux.Handle("GET /api/members", requireAuth(http.HandlerFunc(members.List)))

	// Comments (session required)
	mux.Handle("GET /api/posts/{id}/comments", requireAuth(http.HandlerFunc(comments.List)))
	mux.Handle("POST /api/posts/{id}/comments", requireAuth(http.HandlerFunc(comments.Create)))
	mux.Handle("DELETE /api/comments/{commentId}", requireAuth(http.HandlerFunc(comments.Delete)))

	// Media
	if mediaH != nil {
		mux.Handle("POST /api/media/upload", requireAuth(http.HandlerFunc(mediaH.Upload)))
		mux.Handle("POST /api/auth/avatar", requireAuth(http.HandlerFunc(mediaH.UploadAvatar)))
	}

	// Static: uploaded media
	if *uploadsDir != "" {
		abs, _ := filepath.Abs(*uploadsDir)
		mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(abs))))
		log.Printf("Serving uploads from %s", abs)
	}

	// Static: frontend SPA
	if *webDir != "" {
		mux.Handle("GET /", spaHandler(*webDir))
		log.Printf("Serving frontend from %s", *webDir)
	}

	loggedMux := loggingMiddleware(injectUser(sess, mux))

	log.Printf("Listening on %s", *addr)
	if err := http.ListenAndServe(*addr, loggedMux); err != nil {
		log.Fatal(err)
	}
}

func requireAuth(next http.Handler) http.Handler {
	return handler.RequireAuth(next)
}

func injectUser(sess *session.Manager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, err := sess.Get(r); err == nil {
			r = r.WithContext(handler.ContextWithUser(r.Context(), u))
		}
		next.ServeHTTP(w, r)
	})
}

func spaHandler(dir string) http.Handler {
	fs := http.Dir(dir)
	fileServer := http.FileServer(fs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if _, err := os.Stat(filepath.Join(dir, path)); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Microsecond))
	})
}
