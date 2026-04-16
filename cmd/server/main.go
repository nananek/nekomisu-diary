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
	"github.com/nananek/nekomisu-diary/internal/session"
)

func main() {
	pgDSN := flag.String("pg", "postgres://diary:diary_dev_pw@postgres:5432/diary?sslmode=disable", "PostgreSQL DSN")
	addr := flag.String("addr", ":3000", "Listen address")
	uploadsDir := flag.String("uploads", "", "Path to wp-content/uploads (for serving media)")
	webDir := flag.String("web", "", "Path to frontend dist directory")
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

	auth := handler.NewAuthHandler(pool, sess)
	posts := handler.NewPostHandler(pool)
	comments := handler.NewCommentHandler(pool)

	mux := http.NewServeMux()

	// Auth (public)
	mux.HandleFunc("POST /api/auth/login", auth.Login)
	mux.HandleFunc("POST /api/auth/register", auth.Register)

	// Auth (session required)
	mux.Handle("POST /api/auth/logout", handler.RequireAuth(http.HandlerFunc(auth.Logout)))
	mux.Handle("GET /api/auth/me", injectUser(sess, http.HandlerFunc(auth.Me)))
	mux.Handle("PUT /api/auth/password", handler.RequireAuth(http.HandlerFunc(auth.ChangePassword)))

	// Posts (session required)
	mux.Handle("GET /api/posts", handler.RequireAuth(http.HandlerFunc(posts.List)))
	mux.Handle("GET /api/posts/{id}", handler.RequireAuth(http.HandlerFunc(posts.Get)))
	mux.Handle("POST /api/posts", handler.RequireAuth(http.HandlerFunc(posts.Create)))
	mux.Handle("PUT /api/posts/{id}", handler.RequireAuth(http.HandlerFunc(posts.Update)))
	mux.Handle("DELETE /api/posts/{id}", handler.RequireAuth(http.HandlerFunc(posts.Delete)))

	// Comments (session required)
	mux.Handle("GET /api/posts/{id}/comments", handler.RequireAuth(http.HandlerFunc(comments.List)))
	mux.Handle("POST /api/posts/{id}/comments", handler.RequireAuth(http.HandlerFunc(comments.Create)))

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
