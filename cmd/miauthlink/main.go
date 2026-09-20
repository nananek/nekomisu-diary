// miauthlink manages which Misskey account a nekomisu-diary account signs
// in as via MiAuth (see internal/handler/miauth.go). Linking is
// intentionally admin-only: there is no self-service "connect your
// Misskey account" flow in the app itself, so this is the only way to
// create, change, or remove a link.
//
// Three modes:
//
//  1. Link (or replace) a diary account's Misskey account:
//     miauthlink -pg <dsn> -misskey-instance <origin> -login <login> -misskey-username <name>
//     The Misskey username is resolved to its stable account ID via the
//     instance's own API at link time, so a later username change on the
//     Misskey side doesn't break the link.
//
//  2. Remove a link:
//     miauthlink -pg <dsn> -login <login> -unlink
//
//  3. List all links:
//     miauthlink -pg <dsn> -list
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"
)

func main() {
	pgDSN := flag.String("pg", "", "PostgreSQL DSN (required)")
	instance := flag.String("misskey-instance", "", "Misskey instance origin, e.g. https://kamisato.example.ts.net (required to link)")
	login := flag.String("login", "", "nekomisu-diary login to link/unlink")
	misskeyUsername := flag.String("misskey-username", "", "Misskey username to link (resolved via the instance's API)")
	unlink := flag.Bool("unlink", false, "Remove the Misskey link for -login")
	list := flag.Bool("list", false, "List all Misskey account links")
	flag.Parse()

	if *pgDSN == "" {
		usage()
	}

	db, err := sql.Open("postgres", *pgDSN)
	if err != nil {
		die("db open: %v", err)
	}
	defer db.Close()

	switch {
	case *list:
		listLinks(db)

	case *unlink:
		if *login == "" {
			die("-login is required with -unlink")
		}
		unlinkUser(db, *login)

	case *login != "" && *misskeyUsername != "":
		if *instance == "" {
			die("-misskey-instance is required")
		}
		linkUser(db, normalizeInstance(*instance), *login, *misskeyUsername)

	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  miauthlink -pg <dsn> -misskey-instance <origin> -login <login> -misskey-username <name>")
	fmt.Fprintln(os.Stderr, "  miauthlink -pg <dsn> -login <login> -unlink")
	fmt.Fprintln(os.Stderr, "  miauthlink -pg <dsn> -list")
	os.Exit(1)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func normalizeInstance(instance string) string {
	return strings.TrimRight(instance, "/")
}

type misskeyUser struct {
	ID       string  `json:"id"`
	Username string  `json:"username"`
	Host     *string `json:"host"`
}

// resolveMisskeyUser looks up a local account on the given instance by
// username, the same way an operator would type it — not by ID, which
// nobody has memorized.
func resolveMisskeyUser(instance, username string) (*misskeyUser, error) {
	payload, _ := json.Marshal(map[string]string{"username": username})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(instance+"/api/users/show", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("misskey API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("misskey user %q not found on %s", username, instance)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("misskey API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var u misskeyUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("decode misskey response: %w", err)
	}
	return &u, nil
}

func linkUser(db *sql.DB, instance, login, misskeyUsername string) {
	var userID int64
	if err := db.QueryRow(`SELECT id FROM users WHERE login = $1`, login).Scan(&userID); err != nil {
		die("diary user %q not found: %v", login, err)
	}

	mu, err := resolveMisskeyUser(instance, misskeyUsername)
	if err != nil {
		die("%v", err)
	}
	if mu.Host != nil {
		die("misskey user %q is a remote/federated account (host=%s); only local accounts on %s can be linked", misskeyUsername, *mu.Host, instance)
	}

	_, err = db.Exec(`
		INSERT INTO misskey_links (user_id, misskey_instance, misskey_user_id, misskey_username)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE
		  SET misskey_instance = EXCLUDED.misskey_instance,
		      misskey_user_id  = EXCLUDED.misskey_user_id,
		      misskey_username = EXCLUDED.misskey_username,
		      created_at       = NOW()
	`, userID, instance, mu.ID, mu.Username)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			die("misskey account @%s (id=%s) on %s is already linked to a different diary account", mu.Username, mu.ID, instance)
		}
		die("link: %v", err)
	}
	fmt.Fprintf(os.Stderr, "Linked %q -> @%s on %s (misskey id=%s)\n", login, mu.Username, instance, mu.ID)
}

func unlinkUser(db *sql.DB, login string) {
	res, err := db.Exec(`
		DELETE FROM misskey_links
		WHERE user_id = (SELECT id FROM users WHERE login = $1)
	`, login)
	if err != nil {
		die("unlink: %v", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fmt.Fprintf(os.Stderr, "No misskey link existed for %q\n", login)
		return
	}
	fmt.Fprintf(os.Stderr, "Unlinked %q\n", login)
}

func listLinks(db *sql.DB) {
	rows, err := db.Query(`
		SELECT u.login, ml.misskey_instance, ml.misskey_username, ml.misskey_user_id, ml.created_at
		FROM misskey_links ml
		JOIN users u ON u.id = ml.user_id
		ORDER BY u.login
	`)
	if err != nil {
		die("list: %v", err)
	}
	defer rows.Close()

	fmt.Fprintln(os.Stderr, "login\tmisskey_instance\tmisskey_username\tmisskey_user_id\tcreated_at")
	n := 0
	for rows.Next() {
		var login, instance, username, id string
		var createdAt time.Time
		if err := rows.Scan(&login, &instance, &username, &id, &createdAt); err != nil {
			die("scan: %v", err)
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", login, instance, username, id, createdAt.Format(time.RFC3339))
		n++
	}
	fmt.Fprintf(os.Stderr, "%d link(s).\n", n)
}
