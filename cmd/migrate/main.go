package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

type renderedPost struct {
	ID     int64  `json:"id"`
	HTML   string `json:"html"`
	Source string `json:"source"`
}

func main() {
	mariaDSN := flag.String("maria", "wpuser_b58f:wp_pass_8d9c2@tcp(db:3306)/wpdb_9f3a?parseTime=true&charset=utf8mb4", "MariaDB DSN")
	pgDSN := flag.String("pg", "postgres://diary:diary_dev_pw@postgres:5432/diary?sslmode=disable", "PostgreSQL DSN")
	renderedFile := flag.String("rendered", "tools/rendered.json", "Path to rendered posts JSON from wp-cli")
	defaultPass := flag.String("default-password", "", "Default bcrypt password for all migrated users (required)")
	flag.Parse()

	if *defaultPass == "" {
		log.Fatal("-default-password is required (plain text, will be bcrypt-hashed)")
	}

	passHash, err := bcrypt.GenerateFromPassword([]byte(*defaultPass), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("bcrypt: %v", err)
	}

	rendered, err := loadRendered(*renderedFile)
	if err != nil {
		log.Fatalf("load rendered.json: %v", err)
	}
	log.Printf("Loaded %d rendered posts", len(rendered))

	maria, err := sql.Open("mysql", *mariaDSN)
	if err != nil {
		log.Fatalf("maria open: %v", err)
	}
	defer maria.Close()

	pg, err := sql.Open("postgres", *pgDSN)
	if err != nil {
		log.Fatalf("pg open: %v", err)
	}
	defer pg.Close()

	if err := migrateUsers(maria, pg, string(passHash)); err != nil {
		log.Fatalf("migrate users: %v", err)
	}
	if err := migratePosts(maria, pg, rendered); err != nil {
		log.Fatalf("migrate posts: %v", err)
	}
	if err := migrateComments(maria, pg); err != nil {
		log.Fatalf("migrate comments: %v", err)
	}
	if err := migrateMedia(maria, pg); err != nil {
		log.Fatalf("migrate media: %v", err)
	}

	log.Println("Migration completed successfully")
}

func loadRendered(path string) (map[int64]renderedPost, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var posts []renderedPost
	if err := json.Unmarshal(data, &posts); err != nil {
		return nil, err
	}
	m := make(map[int64]renderedPost, len(posts))
	for _, p := range posts {
		m[p.ID] = p
	}
	return m, nil
}

func migrateUsers(maria, pg *sql.DB, passHash string) error {
	log.Println("Migrating users...")

	rows, err := maria.Query(`
		SELECT u.ID, u.user_login, u.user_email, u.display_name, u.user_registered,
		       (SELECT meta_value FROM wp_usermeta WHERE user_id = u.ID AND meta_key = 'wp_user_avatar') AS avatar_attachment_id
		FROM wp_users u
		ORDER BY u.ID`)
	if err != nil {
		return fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			wpID        int64
			login       string
			email       string
			displayName string
			registered  time.Time
			avatarAttID sql.NullString
		)
		if err := rows.Scan(&wpID, &login, &email, &displayName, &registered, &avatarAttID); err != nil {
			return fmt.Errorf("scan user: %w", err)
		}

		var avatarPath sql.NullString
		if avatarAttID.Valid && avatarAttID.String != "" && avatarAttID.String != "0" {
			var path string
			err := maria.QueryRow(`
				SELECT REPLACE(guid, SUBSTRING_INDEX(guid, '/wp-content/uploads/', 1), '')
				FROM wp_posts WHERE ID = ?`, avatarAttID.String).Scan(&path)
			if err == nil {
				avatarPath = sql.NullString{String: path, Valid: true}
			}
		}

		_, err := pg.Exec(`
			INSERT INTO users (login, email, display_name, password_hash, avatar_path, created_at, updated_at, wp_user_id)
			VALUES ($1, $2, $3, $4, $5, $6, $6, $7)
			ON CONFLICT (wp_user_id) DO UPDATE SET
				login = EXCLUDED.login, email = EXCLUDED.email, display_name = EXCLUDED.display_name,
				password_hash = EXCLUDED.password_hash, avatar_path = EXCLUDED.avatar_path`,
			login, email, displayName, passHash, avatarPath, registered, wpID)
		if err != nil {
			return fmt.Errorf("insert user %s: %w", login, err)
		}
		log.Printf("  user: %s (wp_id=%d)", login, wpID)
	}
	return rows.Err()
}

func migratePosts(maria, pg *sql.DB, rendered map[int64]renderedPost) error {
	log.Println("Migrating posts...")

	rows, err := maria.Query(`
		SELECT ID, post_author, post_title, post_content, post_status, post_date
		FROM wp_posts
		WHERE post_type = 'post' AND post_status IN ('publish', 'private', 'draft')
		ORDER BY ID`)
	if err != nil {
		return fmt.Errorf("query posts: %w", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var (
			wpID      int64
			authorID  int64
			title     string
			content   string
			status    string
			postDate  time.Time
		)
		if err := rows.Scan(&wpID, &authorID, &title, &content, &status, &postDate); err != nil {
			return fmt.Errorf("scan post: %w", err)
		}

		visibility := "public"
		switch status {
		case "private":
			visibility = "private"
		case "draft":
			visibility = "draft"
		}

		bodyHTML := content
		bodySource := content
		if rp, ok := rendered[wpID]; ok {
			bodyHTML = rp.HTML
			bodySource = rp.Source
		}

		var publishedAt *time.Time
		if visibility != "draft" {
			publishedAt = &postDate
		}

		_, err := pg.Exec(`
			INSERT INTO posts (author_id, title, body_html, body_source, visibility, published_at, created_at, updated_at, wp_post_id)
			VALUES (
				(SELECT id FROM users WHERE wp_user_id = $1),
				$2, $3, $4, $5::post_visibility, $6, $7, $7, $8
			)
			ON CONFLICT (wp_post_id) DO UPDATE SET
				title = EXCLUDED.title, body_html = EXCLUDED.body_html, body_source = EXCLUDED.body_source,
				visibility = EXCLUDED.visibility, published_at = EXCLUDED.published_at`,
			authorID, title, bodyHTML, bodySource, visibility, publishedAt, postDate, wpID)
		if err != nil {
			return fmt.Errorf("insert post wp_id=%d: %w", wpID, err)
		}
		count++
	}
	log.Printf("  migrated %d posts", count)
	return rows.Err()
}

func migrateComments(maria, pg *sql.DB) error {
	log.Println("Migrating comments...")

	rows, err := maria.Query(`
		SELECT comment_ID, comment_post_ID, user_id, comment_author, comment_content, comment_parent, comment_date
		FROM wp_comments
		WHERE comment_approved = '1'
		ORDER BY comment_ID`)
	if err != nil {
		return fmt.Errorf("query comments: %w", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var (
			wpCommentID int64
			wpPostID    int64
			wpUserID    int64
			authorName  string
			body        string
			wpParentID  int64
			commentDate time.Time
		)
		if err := rows.Scan(&wpCommentID, &wpPostID, &wpUserID, &authorName, &body, &wpParentID, &commentDate); err != nil {
			return fmt.Errorf("scan comment: %w", err)
		}

		var authorID sql.NullInt64
		if wpUserID > 0 {
			var uid int64
			err := pg.QueryRow(`SELECT id FROM users WHERE wp_user_id = $1`, wpUserID).Scan(&uid)
			if err == nil {
				authorID = sql.NullInt64{Int64: uid, Valid: true}
			}
		}

		var parentID sql.NullInt64
		if wpParentID > 0 {
			var pid int64
			err := pg.QueryRow(`SELECT id FROM comments WHERE wp_comment_id = $1`, wpParentID).Scan(&pid)
			if err == nil {
				parentID = sql.NullInt64{Int64: pid, Valid: true}
			}
		}

		var authorNamePtr *string
		if !authorID.Valid {
			authorNamePtr = &authorName
		}

		_, err := pg.Exec(`
			INSERT INTO comments (post_id, author_id, author_name, body, parent_id, created_at, wp_comment_id)
			VALUES (
				(SELECT id FROM posts WHERE wp_post_id = $1),
				$2, $3, $4, $5, $6, $7
			)
			ON CONFLICT (wp_comment_id) DO UPDATE SET
				body = EXCLUDED.body, author_name = EXCLUDED.author_name`,
			wpPostID, authorID, authorNamePtr, body, parentID, commentDate, wpCommentID)
		if err != nil {
			return fmt.Errorf("insert comment wp_id=%d: %w", wpCommentID, err)
		}
		count++
	}
	log.Printf("  migrated %d comments", count)
	return rows.Err()
}

func migrateMedia(maria, pg *sql.DB) error {
	log.Println("Migrating media...")

	rows, err := maria.Query(`
		SELECT p.ID, p.post_author, p.post_title, p.guid, p.post_mime_type, p.post_parent, p.post_date
		FROM wp_posts p
		WHERE p.post_type = 'attachment'
		ORDER BY p.ID`)
	if err != nil {
		return fmt.Errorf("query media: %w", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var (
			wpID     int64
			authorID int64
			filename string
			guid     string
			mimeType string
			parentID int64
			created  time.Time
		)
		if err := rows.Scan(&wpID, &authorID, &filename, &guid, &mimeType, &parentID, &created); err != nil {
			return fmt.Errorf("scan media: %w", err)
		}

		storagePath := extractUploadPath(guid)

		var attachedPostID sql.NullInt64
		if parentID > 0 {
			var pid int64
			err := pg.QueryRow(`SELECT id FROM posts WHERE wp_post_id = $1`, parentID).Scan(&pid)
			if err == nil {
				attachedPostID = sql.NullInt64{Int64: pid, Valid: true}
			}
		}

		_, err := pg.Exec(`
			INSERT INTO media (uploader_id, filename, storage_path, mime_type, attached_post_id, created_at, wp_attachment_id)
			VALUES (
				(SELECT id FROM users WHERE wp_user_id = $1),
				$2, $3, $4, $5, $6, $7
			)
			ON CONFLICT (wp_attachment_id) DO UPDATE SET
				filename = EXCLUDED.filename, storage_path = EXCLUDED.storage_path`,
			authorID, filename, storagePath, mimeType, attachedPostID, created, wpID)
		if err != nil {
			return fmt.Errorf("insert media wp_id=%d: %w", wpID, err)
		}
		count++
	}
	log.Printf("  migrated %d media", count)
	return rows.Err()
}

func extractUploadPath(guid string) string {
	const marker = "/wp-content/uploads/"
	idx := 0
	for i := range guid {
		if i+len(marker) <= len(guid) && guid[i:i+len(marker)] == marker {
			idx = i + len(marker)
			break
		}
	}
	if idx > 0 {
		return guid[idx:]
	}
	return guid
}
