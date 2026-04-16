// One-shot tool: sanitize all existing post body_html values.
// Idempotent — safe to run multiple times.
package main

import (
	"database/sql"
	"flag"
	"log"

	_ "github.com/lib/pq"
	"github.com/nananek/nekomisu-diary/internal/sanitize"
)

func main() {
	pgDSN := flag.String("pg", "", "PostgreSQL DSN (required)")
	dryRun := flag.Bool("dry-run", false, "only report changes, don't write")
	flag.Parse()

	if *pgDSN == "" {
		log.Fatal("-pg is required")
	}

	db, err := sql.Open("postgres", *pgDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT id, body_html FROM posts`)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var changed, total int
	for rows.Next() {
		var id int64
		var body string
		if err := rows.Scan(&id, &body); err != nil {
			continue
		}
		total++
		clean := sanitize.HTML(body)
		if clean == body {
			continue
		}
		changed++
		log.Printf("post %d: %d → %d bytes", id, len(body), len(clean))
		if !*dryRun {
			if _, err := db.Exec(`UPDATE posts SET body_html = $1 WHERE id = $2`, clean, id); err != nil {
				log.Printf("  update failed: %v", err)
			}
		}
	}

	log.Printf("Scanned %d posts, %d changed (dry-run=%v)", total, changed, *dryRun)
}
