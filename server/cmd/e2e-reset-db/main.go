package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dareaquatics/aquify/server/internal/db"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}
	if err := validateResetTarget(databaseURL, os.Getenv("E2E_ALLOW_DB_RESET")); err != nil {
		fmt.Fprintf(os.Stderr, "refusing to reset database: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres;
GRANT ALL ON SCHEMA public TO public;
`); err != nil {
		fmt.Fprintf(os.Stderr, "reset schema: %v\n", err)
		os.Exit(1)
	}
}

func validateResetTarget(databaseURL, allowReset string) error {
	if allowReset != "true" {
		return errors.New("E2E_ALLOW_DB_RESET=true is required")
	}

	cfg, err := pgconn.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	host := strings.ToLower(strings.TrimSpace(cfg.Host))
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("host %q is not an allowed local E2E database host", cfg.Host)
	}

	dbName := strings.ToLower(strings.TrimSpace(cfg.Database))
	if dbName == "" {
		return errors.New("database name is empty")
	}
	if dbName == "postgres" || strings.Contains(dbName, "prod") || strings.Contains(dbName, "staging") {
		return fmt.Errorf("database name %q is not allowed for E2E reset", cfg.Database)
	}
	if !hasTestToken(dbName) {
		return fmt.Errorf("database name %q must contain a whole `test` or `e2e` word (for example aquify_e2e)", cfg.Database)
	}

	return nil
}

// hasTestToken reports whether the database name contains "test" or "e2e" as a
// whole word delimited by underscores or hyphens. Plain substring matching
// would accept names such as "contest_results" or "latest".
func hasTestToken(dbName string) bool {
	for _, token := range strings.FieldsFunc(dbName, func(r rune) bool { return r == '_' || r == '-' }) {
		if token == "test" || token == "e2e" {
			return true
		}
	}
	return false
}
