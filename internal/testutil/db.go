// Package testutil provides shared helpers for tests that need a real
// PostgreSQL instance. Each test acquires a fresh, isolated database by
// creating a new DB on the same server, loading schema.sql, and dropping
// it on cleanup.
package testutil

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
	"github.com/nananek/nekomisu-diary/internal/migrate"
)

// adminDSN points to the test postgres instance. TEST_PG_DSN is required;
// there is no default to avoid baking credentials into the repo.
func adminDSN() string {
	v := os.Getenv("TEST_PG_DSN")
	if v == "" {
		panic("TEST_PG_DSN not set; integration tests require a test postgres DSN")
	}
	return v
}

// NewDB creates a fresh, randomly-named database, loads schema.sql, and
// returns an *sql.DB pointing at it. Schedules the DB to be dropped on
// test cleanup.
func NewDB(t *testing.T) *sql.DB {
	t.Helper()

	// Random DB name
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	dbName := "test_" + hex.EncodeToString(buf)

	// Admin connection to create the DB
	admin, err := sql.Open("postgres", adminDSN())
	if err != nil {
		t.Fatalf("admin open: %v", err)
	}
	defer admin.Close()

	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatalf("create db: %v", err)
	}

	// Load schema via migrations into the new DB
	targetDSN := strings.Replace(adminDSN(), "/diary?", "/"+dbName+"?", 1)
	db, err := sql.Open("postgres", targetDSN)
	if err != nil {
		t.Fatalf("target open: %v", err)
	}
	if err := migrate.Up(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		admin2, err := sql.Open("postgres", adminDSN())
		if err != nil {
			return
		}
		defer admin2.Close()
		// Terminate any leftover connections, then drop.
		admin2.Exec(
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
			dbName,
		)
		admin2.Exec("DROP DATABASE IF EXISTS " + dbName)
	})

	return db
}

// InsertUser is a convenience helper to create a test user directly in DB.
// Returns the new user's ID.
func InsertUser(t *testing.T, db *sql.DB, login, email, displayName, passwordHash string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(
		`INSERT INTO users (login, email, display_name, password_hash) VALUES ($1,$2,$3,$4) RETURNING id`,
		login, email, displayName, passwordHash,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// Unique returns a short unique suffix for test data names.
func Unique() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
