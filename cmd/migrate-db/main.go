// Standalone migration CLI. Usage:
//   migrate-db -pg <dsn> up        apply all pending
//   migrate-db -pg <dsn> down      roll back one
//   migrate-db -pg <dsn> status    list migrations and state
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
	"github.com/nananek/nekomisu-diary/internal/db"
	"github.com/nananek/nekomisu-diary/internal/migrate"
)

func main() {
	pgDSN := flag.String("pg", "", "PostgreSQL DSN (required)")
	flag.Parse()

	if *pgDSN == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "Usage: migrate-db -pg <dsn> {up|down|status}")
		os.Exit(1)
	}

	pool, err := db.Open(*pgDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	switch flag.Arg(0) {
	case "up":
		if err := migrate.Up(pool); err != nil {
			log.Fatalf("up: %v", err)
		}
	case "down":
		if err := migrate.Down(pool); err != nil {
			log.Fatalf("down: %v", err)
		}
	case "status":
		if err := migrate.Status(pool); err != nil {
			log.Fatalf("status: %v", err)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", flag.Arg(0))
		os.Exit(1)
	}
}
