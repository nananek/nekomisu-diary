// Package migrate wraps goose with an embedded migration filesystem
// and a bootstrap step that flags pre-existing schemas as already-applied.
//
// The server runs `Up` on startup; a standalone `cmd/migrate-db` CLI is
// also available for ops tasks (up/down/status/create).
package migrate

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

const dialect = "postgres"

func init() {
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect(dialect); err != nil {
		panic(err)
	}
}

// Up runs all pending migrations.
func Up(db *sql.DB) error {
	if err := bootstrapIfNeeded(db); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	return goose.Up(db, "migrations")
}

// Down rolls back the most recent migration.
func Down(db *sql.DB) error {
	return goose.Down(db, "migrations")
}

// Status prints each migration's applied/pending state to stdout.
func Status(db *sql.DB) error {
	return goose.Status(db, "migrations")
}

// bootstrapIfNeeded stamps 0001 as applied on databases that already
// have our tables from the pre-migration era. Idempotent: safe to run
// on every startup; no-op on fresh DBs (where users doesn't exist yet).
func bootstrapIfNeeded(db *sql.DB) error {
	var usersExists bool
	if err := db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='public' AND tablename='users')
	`).Scan(&usersExists); err != nil {
		return err
	}
	if !usersExists {
		return nil
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS goose_db_version (
			id SERIAL PRIMARY KEY,
			version_id BIGINT NOT NULL,
			is_applied BOOLEAN NOT NULL,
			tstamp TIMESTAMP DEFAULT NOW()
		);
		INSERT INTO goose_db_version (version_id, is_applied)
		  SELECT 0, true WHERE NOT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id = 0);
		INSERT INTO goose_db_version (version_id, is_applied)
		  SELECT 1, true WHERE NOT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id = 1);
	`)
	return err
}
