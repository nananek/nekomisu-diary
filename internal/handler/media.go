package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

type MediaHandler struct {
	db         *sql.DB
	uploadsDir string
}

func NewMediaHandler(db *sql.DB, uploadsDir string) *MediaHandler {
	return &MediaHandler{db: db, uploadsDir: uploadsDir}
}

// List returns media uploaded by the current user.
func (h *MediaHandler) List(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	rows, err := h.db.Query(`
		SELECT id, filename, storage_path, thumbnail_path, mime_type, byte_size, width, height, created_at
		FROM media
		WHERE uploader_id = $1
		ORDER BY created_at DESC
		LIMIT 200`, u.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	defer rows.Close()

	items := make([]M, 0)
	for rows.Next() {
		var id int64
		var filename, storagePath, mimeType, createdAt string
		var thumbPath sql.NullString
		var byteSize sql.NullInt64
		var width, height sql.NullInt64
		if rows.Scan(&id, &filename, &storagePath, &thumbPath, &mimeType, &byteSize, &width, &height, &createdAt) != nil {
			continue
		}
		m := M{
			"id":           id,
			"filename":     filename,
			"url":          "/uploads/" + storagePath,
			"mime_type":    mimeType,
			"created_at":   createdAt,
		}
		if thumbPath.Valid {
			m["thumbnail_url"] = "/uploads/" + thumbPath.String
		}
		if byteSize.Valid {
			m["byte_size"] = byteSize.Int64
		}
		if width.Valid {
			m["width"] = width.Int64
		}
		if height.Valid {
			m["height"] = height.Int64
		}
		items = append(items, m)
	}
	writeJSON(w, http.StatusOK, M{"items": items})
}

// Delete removes the uploader's own media file (DB + file on disk).
func (h *MediaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	var mediaID int64
	fmt.Sscanf(r.PathValue("id"), "%d", &mediaID)

	var storagePath string
	var thumbPath sql.NullString
	err := h.db.QueryRow(
		`SELECT storage_path, thumbnail_path FROM media WHERE id = $1 AND uploader_id = $2`,
		mediaID, u.UserID,
	).Scan(&storagePath, &thumbPath)
	if err != nil {
		writeJSON(w, http.StatusForbidden, M{"error": "forbidden or not found"})
		return
	}

	h.db.Exec(`DELETE FROM media WHERE id = $1`, mediaID)
	// Best-effort file removal
	os.Remove(filepath.Join(h.uploadsDir, storagePath))
	if thumbPath.Valid {
		os.Remove(filepath.Join(h.uploadsDir, thumbPath.String))
	}

	writeJSON(w, http.StatusOK, M{"ok": true})
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

	// Try to get image dimensions and generate a thumbnail (best-effort)
	var width, height int
	var thumbnailPath sql.NullString
	if img, format, err := decodeImage(absPath); err == nil {
		bounds := img.Bounds()
		width = bounds.Dx()
		height = bounds.Dy()
		if format == "jpeg" || format == "png" {
			thumbRel := strings.TrimSuffix(storagePath, filepath.Ext(storagePath)) + "-thumb.jpg"
			thumbAbs := filepath.Join(h.uploadsDir, thumbRel)
			if err := saveThumbnail(img, thumbAbs, 800); err == nil {
				thumbnailPath = sql.NullString{String: thumbRel, Valid: true}
			}
		}
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
		INSERT INTO media (uploader_id, filename, storage_path, thumbnail_path, mime_type, byte_size, width, height, attached_post_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		u.UserID, header.Filename, storagePath, thumbnailPath, mime, written,
		nilIfZero(width), nilIfZero(height), attachedPostID,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}

	resp := M{
		"id":   id,
		"url":  "/uploads/" + storagePath,
		"path": storagePath,
	}
	if thumbnailPath.Valid {
		resp["thumbnail_url"] = "/uploads/" + thumbnailPath.String
	}
	writeJSON(w, http.StatusCreated, resp)
}

func decodeImage(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	return image.Decode(f)
}

func saveThumbnail(img image.Image, path string, maxDim int) error {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= maxDim && h <= maxDim {
		// Already small enough; still write as JPEG for consistent thumb format
		return writeJPEG(img, path, 85)
	}
	var tw, th int
	if w > h {
		tw = maxDim
		th = h * maxDim / w
	} else {
		th = maxDim
		tw = w * maxDim / h
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
	return writeJPEG(dst, path, 85)
}

func writeJPEG(img image.Image, path string, quality int) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return jpeg.Encode(out, img, &jpeg.Options{Quality: quality})
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
