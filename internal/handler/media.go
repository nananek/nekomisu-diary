package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type MediaHandler struct {
	db         *sql.DB
	uploadsDir string
}

func NewMediaHandler(db *sql.DB, uploadsDir string) *MediaHandler {
	return &MediaHandler{db: db, uploadsDir: uploadsDir}
}

func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<20) // 64MB
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "file too large or invalid form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "file field required"})
		return
	}
	defer file.Close()

	mime := header.Header.Get("Content-Type")
	if !strings.HasPrefix(mime, "image/") {
		writeJSON(w, http.StatusBadRequest, M{"error": "only images allowed"})
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		switch mime {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		default:
			ext = ".bin"
		}
	}

	randBytes := make([]byte, 8)
	rand.Read(randBytes)
	randName := hex.EncodeToString(randBytes)

	now := time.Now()
	relDir := fmt.Sprintf("%d/%02d", now.Year(), now.Month())
	absDir := filepath.Join(h.uploadsDir, relDir)
	os.MkdirAll(absDir, 0o755)

	storagePath := filepath.Join(relDir, randName+ext)
	absPath := filepath.Join(h.uploadsDir, storagePath)

	out, err := os.Create(absPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "file create error"})
		return
	}
	defer out.Close()

	written, err := io.Copy(out, file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "file write error"})
		return
	}
	out.Close()

	// Try to get image dimensions
	var width, height int
	f2, err := os.Open(absPath)
	if err == nil {
		cfg, _, err := image.DecodeConfig(f2)
		if err == nil {
			width = cfg.Width
			height = cfg.Height
		}
		f2.Close()
	}

	postIDStr := r.FormValue("post_id")
	var attachedPostID sql.NullInt64
	if postIDStr != "" {
		var pid int64
		fmt.Sscanf(postIDStr, "%d", &pid)
		if pid > 0 {
			attachedPostID = sql.NullInt64{Int64: pid, Valid: true}
		}
	}

	var id int64
	err = h.db.QueryRow(`
		INSERT INTO media (uploader_id, filename, storage_path, mime_type, byte_size, width, height, attached_post_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		u.UserID, header.Filename, storagePath, mime, written,
		nilIfZero(width), nilIfZero(height), attachedPostID,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}

	writeJSON(w, http.StatusCreated, M{
		"id":   id,
		"url":  "/uploads/" + storagePath,
		"path": storagePath,
	})
}

func (h *MediaHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 5<<20) // 5MB
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "file too large"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "file field required"})
		return
	}
	defer file.Close()

	mime := header.Header.Get("Content-Type")
	if !strings.HasPrefix(mime, "image/") {
		writeJSON(w, http.StatusBadRequest, M{"error": "only images allowed"})
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".jpg"
	}

	avatarDir := filepath.Join(h.uploadsDir, "avatars")
	os.MkdirAll(avatarDir, 0o755)

	filename := fmt.Sprintf("%d%s", u.UserID, ext)
	absPath := filepath.Join(avatarDir, filename)

	out, err := os.Create(absPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "file create error"})
		return
	}
	defer out.Close()
	io.Copy(out, file)

	avatarPath := "/uploads/avatars/" + filename
	h.db.Exec(`UPDATE users SET avatar_path = $1 WHERE id = $2`, avatarPath, u.UserID)

	writeJSON(w, http.StatusOK, M{"avatar_path": avatarPath})
}

func nilIfZero(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}
