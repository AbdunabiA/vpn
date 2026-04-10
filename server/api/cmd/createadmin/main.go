// createadmin is a one-shot CLI that inserts an admin user into the VPN API
// database. It is intended to be run once per environment to bootstrap the
// first admin account, after which further admins can be promoted via the
// /admin/users/:id endpoint.
//
// Usage:
//
//	DATABASE_URL=postgres://... ./createadmin -email=you@example.com -password=secret
//
// The command is idempotent in the sense that it will refuse to overwrite an
// existing user with the same email.
package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"vpnapp/server/api/internal/model"
	"vpnapp/server/api/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	email := flag.String("email", "", "admin email address")
	password := flag.String("password", "", "admin password (8-72 chars)")
	dbURL := flag.String("db", os.Getenv("DATABASE_URL"), "database URL (defaults to $DATABASE_URL)")
	flag.Parse()

	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "usage: createadmin -email=<email> -password=<password>")
		flag.Usage()
		os.Exit(2)
	}
	if len(*password) < 8 || len(*password) > 72 {
		log.Fatal("password must be 8-72 characters")
	}
	if *dbURL == "" {
		log.Fatal("database URL required: set -db flag or DATABASE_URL env var")
	}

	db, err := repository.NewDB(*dbURL)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}

	// Refuse to overwrite an existing account with the same email.
	// Two checks: a friendly pre-flight lookup, and a unique-constraint
	// catch on insert in case two concurrent createadmin runs race.
	emailHash := fmt.Sprintf("%x", sha256.Sum256([]byte(*email)))
	if existing, err := repository.FindUserByEmailHash(db, emailHash); err == nil {
		log.Fatalf("user with email %s already exists (id=%s, role=%s)", *email, existing.ID, existing.Role)
	} else if !errors.Is(err, repository.ErrNotFound) {
		log.Fatalf("checking existing user: %v", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hashing password: %v", err)
	}

	hashStr := string(hash)
	user := model.User{
		EmailHash:        &emailHash,
		PasswordHash:     &hashStr,
		FullName:         "Admin",
		Role:             "admin",
		SubscriptionTier: "ultimate",
	}
	if err := repository.CreateUser(db, &user); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			log.Fatalf("user with email %s already exists (concurrent createadmin?)", *email)
		}
		log.Fatalf("creating admin user: %v", err)
	}

	fmt.Printf("admin user created\n  id:    %s\n  email: %s\n  role:  %s\n", user.ID, *email, user.Role)
}
