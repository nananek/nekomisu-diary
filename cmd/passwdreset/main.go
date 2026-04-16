package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	pgDSN := flag.String("pg", "postgres://diary:diary_dev_pw@postgres:5432/diary?sslmode=disable", "PostgreSQL DSN")
	login := flag.String("user", "", "User login name (required)")
	newPass := flag.String("password", "", "New password (required)")
	flag.Parse()

	if *login == "" || *newPass == "" {
		fmt.Fprintln(os.Stderr, "Usage: passwdreset -user <login> -password <newpass> [-pg <dsn>]")
		os.Exit(1)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*newPass), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("bcrypt: %v", err)
	}

	db, err := sql.Open("postgres", *pgDSN)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	res, err := db.Exec(`UPDATE users SET password_hash = $1 WHERE login = $2`, string(hash), *login)
	if err != nil {
		log.Fatalf("update: %v", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		log.Fatalf("user %q not found", *login)
	}
	fmt.Printf("Password updated for user %q\n", *login)
}
