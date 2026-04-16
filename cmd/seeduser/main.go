// Small helper used by CI to seed a test user into a fresh DB.
// Not used in production.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	pgDSN := flag.String("pg", "", "PostgreSQL DSN (required)")
	login := flag.String("login", "nananek", "login name")
	email := flag.String("email", "n@ex.com", "email")
	displayName := flag.String("name", "nananek", "display name")
	password := flag.String("password", "changeme123", "password (will be bcrypt-hashed)")
	cost := flag.Int("cost", bcrypt.MinCost, "bcrypt cost (MinCost=4 for fast tests)")
	flag.Parse()

	if *pgDSN == "" {
		log.Fatal("-pg is required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*password), *cost)
	if err != nil {
		log.Fatalf("bcrypt: %v", err)
	}

	db, err := sql.Open("postgres", *pgDSN)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO users (login, email, display_name, password_hash) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (login) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		*login, *email, *displayName, string(hash),
	)
	if err != nil {
		log.Fatalf("insert: %v", err)
	}
	fmt.Printf("seeded user: %s\n", *login)
}
