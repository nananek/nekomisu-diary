// Password reset CLI. Three modes:
//
//   1. Single user, explicit password:
//      passwdreset -pg <dsn> -user <login> -password <newpass>
//
//   2. Single user, generated random password (printed to stdout):
//      passwdreset -pg <dsn> -user <login> -random
//
//   3. All users, generated random passwords (prompts for confirmation):
//      passwdreset -pg <dsn> -all -random
//      Pairs of "login <TAB> password" are written to stdout — redirect to
//      a file and distribute securely.
package main

import (
	"bufio"
	"crypto/rand"
	"database/sql"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// Base58-ish alphabet: excludes easily-confused characters (0 O 1 l I).
const randAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
const randDefaultLen = 16

func main() {
	pgDSN := flag.String("pg", "", "PostgreSQL DSN (required)")
	login := flag.String("user", "", "User login name (required unless -all)")
	newPass := flag.String("password", "", "New password (required unless -random)")
	randomMode := flag.Bool("random", false, "Generate a random password instead of taking one from -password")
	allUsers := flag.Bool("all", false, "Reset ALL users; requires -random and a confirmation prompt")
	length := flag.Int("length", randDefaultLen, "Random password length")
	yes := flag.Bool("yes", false, "Skip the interactive confirmation for -all (use with care)")
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
	case *allUsers:
		if !*randomMode {
			die("-all requires -random")
		}
		if !*yes && !confirmAll(db) {
			fmt.Fprintln(os.Stderr, "aborted")
			os.Exit(1)
		}
		resetAll(db, *length)

	case *login != "":
		pw := *newPass
		if *randomMode {
			pw = mustRandomPassword(*length)
		} else if pw == "" {
			die("-password or -random required")
		}
		if err := updatePassword(db, *login, pw); err != nil {
			die("update: %v", err)
		}
		fmt.Fprintf(os.Stderr, "Password updated for %q\n", *login)
		if *randomMode {
			fmt.Println(pw) // machine-friendly: only the password on stdout
		}

	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  passwdreset -pg <dsn> -user <login> -password <newpass>")
	fmt.Fprintln(os.Stderr, "  passwdreset -pg <dsn> -user <login> -random")
	fmt.Fprintln(os.Stderr, "  passwdreset -pg <dsn> -all -random [-yes]")
	os.Exit(1)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// confirmAll prompts interactively, showing only the count of affected
// users (not their logins) to avoid leaking identities into scrollback /
// terminal logs.
func confirmAll(db *sql.DB) bool {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		die("count users: %v", err)
	}

	fmt.Fprintf(os.Stderr, "⚠  This will reset passwords for all %d users to fresh random values.\n", count)
	fmt.Fprintln(os.Stderr, "   Existing passwords will be lost; you must hand the new ones to each user.")
	fmt.Fprintln(os.Stderr, "   2FA (TOTP / WebAuthn) is NOT affected.")
	fmt.Fprint(os.Stderr, "\nType 'yes' to proceed: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input) == "yes"
}

func resetAll(db *sql.DB, length int) {
	rows, err := db.Query(`SELECT login FROM users ORDER BY login`)
	if err != nil {
		die("query users: %v", err)
	}
	defer rows.Close()

	var logins []string
	for rows.Next() {
		var l string
		rows.Scan(&l)
		logins = append(logins, l)
	}

	// Print header once to stderr so stdout stays parseable.
	fmt.Fprintln(os.Stderr, "login\tpassword")

	for _, l := range logins {
		pw := mustRandomPassword(length)
		if err := updatePassword(db, l, pw); err != nil {
			die("update %s: %v", l, err)
		}
		fmt.Printf("%s\t%s\n", l, pw)
	}
	fmt.Fprintf(os.Stderr, "Reset %d users.\n", len(logins))
}

func updatePassword(db *sql.DB, login, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	res, err := db.Exec(`UPDATE users SET password_hash = $1 WHERE login = $2`, string(hash), login)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %q not found", login)
	}
	return nil
}

func mustRandomPassword(length int) string {
	if length < 8 {
		die("-length must be >= 8")
	}
	n := big.NewInt(int64(len(randAlphabet)))
	out := make([]byte, length)
	for i := range out {
		idx, err := rand.Int(rand.Reader, n)
		if err != nil {
			die("rand: %v", err)
		}
		out[i] = randAlphabet[idx.Int64()]
	}
	return string(out)
}
