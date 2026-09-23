package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nananek/nekomisu-diary/internal/dbq"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type MediaHandler struct {
	q          *dbq.Queries
	uploadsDir string
}

func NewMediaHandler(db *sql.DB, uploadsDir string) *MediaHandler {
	return &MediaHandler{q: dbq.New(db), uploadsDir: uploadsDir}
}

func (h *MediaHandler) List(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	uploader := sql.NullInt64{Int64: u.UserID, Valid: true}
	rows, err := h.q.ListMediaByUploader(r.Context(), uploader)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	items := make([]M, 0, len(rows))
	for _, rr := range rows {
		item := M{
			"id":         rr.ID,
			"filename":   rr.Filename,
			"url":        "/uploads/" + rr.StoragePath,
			"mime_type":  rr.MimeType,
			"created_at": rr.CreatedAt,
		}
		if rr.ThumbnailPath.Valid {
			item["thumbnail_url"] = "/uploads/" + rr.ThumbnailPath.String
		}
		if rr.ByteSize.Valid {
			item["byte_size"] = rr.ByteSize.Int64
		}
		if rr.Width.Valid {
			item["width"] = rr.Width.Int32
		}
		if rr.Height.Valid {
			item["height"] = rr.Height.Int32
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, M{"items": items})
}

func (h *MediaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}
	var mediaID int64
	fmt.Sscanf(r.PathValue("id"), "%d", &mediaID)

	uploader := sql.NullInt64{Int64: u.UserID, Valid: true}
	info, err := h.q.GetMediaForDelete(r.Context(), dbq.GetMediaForDeleteParams{
		ID:         mediaID,
		UploaderID: uploader,
	})
	if err != nil {
		writeJSON(w, http.StatusForbidden, M{"error": "forbidden or not found"})
		return
	}
	logIfErr("media.Delete", h.q.DeleteMedia(r.Context(), mediaID))
	os.Remove(filepath.Join(h.uploadsDir, info.StoragePath))
	if info.ThumbnailPath.Valid {
		os.Remove(filepath.Join(h.uploadsDir, info.ThumbnailPath.String))
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
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

	// Validate and decode the bytes before anything touches disk. Only real
	// JPEG/PNG/GIF/WebP content is accepted; the stored extension is derived
	// from the decoded format, never from the client-supplied filename or
	// Content-Type (a file named evil.html served from /uploads would run as
	// same-origin HTML).
	img, format, err := decodeUpload(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "unsupported image format"})
		return
	}
	meta := imageFormats[format]

	randBytes := make([]byte, 8)
	rand.Read(randBytes)
	randName := hex.EncodeToString(randBytes)

	now := time.Now()
	relDir := fmt.Sprintf("%d/%02d", now.Year(), now.Month())
	absDir := filepath.Join(h.uploadsDir, relDir)
	os.MkdirAll(absDir, 0o755)

	storagePath := filepath.Join(relDir, randName+meta.ext)
	absPath := filepath.Join(h.uploadsDir, storagePath)

	out, err := os.Create(absPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "file create error"})
		return
	}
	written, err := io.Copy(out, file)
	if err != nil {
		out.Close()
		writeJSON(w, http.StatusInternalServerError, M{"error": "file write error"})
		return
	}
	out.Close()

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	var thumbnailPath sql.NullString
	if format == "jpeg" {
		// Re-encode JPEGs in place to strip EXIF metadata.
		if err := writeJPEG(img, absPath, 95); err == nil {
			if st, err := os.Stat(absPath); err == nil {
				written = st.Size()
			}
		}
	}
	if format == "jpeg" || format == "png" {
		thumbRel := strings.TrimSuffix(storagePath, filepath.Ext(storagePath)) + "-thumb.jpg"
		thumbAbs := filepath.Join(h.uploadsDir, thumbRel)
		if err := saveThumbnail(img, thumbAbs, 800); err == nil {
			thumbnailPath = sql.NullString{String: thumbRel, Valid: true}
		}
	}

	postIDStr := r.FormValue("post_id")
	var attached sql.NullInt64
	if postIDStr != "" {
		var pid int64
		fmt.Sscanf(postIDStr, "%d", &pid)
		if pid > 0 {
			attached = sql.NullInt64{Int64: pid, Valid: true}
		}
	}

	id, err := h.q.CreateMedia(r.Context(), dbq.CreateMediaParams{
		UploaderID:     sql.NullInt64{Int64: u.UserID, Valid: true},
		Filename:       header.Filename,
		StoragePath:    storagePath,
		ThumbnailPath:  thumbnailPath,
		MimeType:       meta.mime,
		ByteSize:       sql.NullInt64{Int64: written, Valid: true},
		Width:          nullInt32(width),
		Height:         nullInt32(height),
		AttachedPostID: attached,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}

	resp := M{"id": id, "url": "/uploads/" + storagePath, "path": storagePath}
	if thumbnailPath.Valid {
		resp["thumbnail_url"] = "/uploads/" + thumbnailPath.String
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *MediaHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, M{"error": "not logged in"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "file too large"})
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "file field required"})
		return
	}
	defer file.Close()

	// Same content-based validation as regular uploads. Avatars live at a
	// predictable URL (/uploads/avatars/<user id><ext>), so a client-chosen
	// extension would be an easy same-origin XSS for every member.
	img, format, err := decodeUpload(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, M{"error": "unsupported image format"})
		return
	}
	meta := imageFormats[format]

	avatarDir := filepath.Join(h.uploadsDir, "avatars")
	os.MkdirAll(avatarDir, 0o755)
	filename := fmt.Sprintf("%d%s", u.UserID, meta.ext)
	absPath := filepath.Join(avatarDir, filename)

	if format == "jpeg" {
		// Re-encode to strip EXIF metadata, like regular uploads.
		if err := writeJPEG(img, absPath, 90); err != nil {
			writeJSON(w, http.StatusInternalServerError, M{"error": "file write error"})
			return
		}
	} else {
		out, err := os.Create(absPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, M{"error": "file create error"})
			return
		}
		if _, err := io.Copy(out, file); err != nil {
			out.Close()
			writeJSON(w, http.StatusInternalServerError, M{"error": "file write error"})
			return
		}
		out.Close()
	}

	avatarPath := "/uploads/avatars/" + filename
	if err := h.q.UpdateUserAvatar(r.Context(), dbq.UpdateUserAvatarParams{
		AvatarPath: sql.NullString{String: avatarPath, Valid: true},
		ID:         u.UserID,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, M{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, M{"avatar_path": avatarPath})
}

// imageFormats is the allow-list of decodable image formats and the
// extension / MIME type each is stored and served with.
var imageFormats = map[string]struct{ ext, mime string }{
	"jpeg": {".jpg", "image/jpeg"},
	"png":  {".png", "image/png"},
	"gif":  {".gif", "image/gif"},
	"webp": {".webp", "image/webp"},
}

// maxImagePixels caps Width*Height before a full decode, so a small file
// claiming enormous dimensions cannot exhaust memory (decompression bomb).
const maxImagePixels = 50_000_000

// decodeUpload validates the multipart part as a real image and rewinds it
// for the caller. The format it returns is the only source of truth for the
// extension and MIME type an upload is stored with.
func decodeUpload(file multipart.File) (image.Image, string, error) {
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, "", err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxImagePixels {
		return nil, "", fmt.Errorf("image dimensions out of range: %dx%d", cfg.Width, cfg.Height)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	img, format, err := image.Decode(file)
	if err != nil {
		return nil, "", err
	}
	if _, ok := imageFormats[format]; !ok {
		return nil, "", fmt.Errorf("unsupported format %q", format)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	return img, format, nil
}

func saveThumbnail(img image.Image, path string, maxDim int) error {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= maxDim && h <= maxDim {
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

func nullInt32(v int) sql.NullInt32 {
	if v == 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(v), Valid: true}
}
