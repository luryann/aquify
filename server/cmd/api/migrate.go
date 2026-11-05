package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/luryann/aquify/server/internal/app"
	"github.com/luryann/aquify/server/internal/dbmigrate"
)

// migrationSource is replaced in tests; production uses the migrations embedded
// into the binary at image build time.
var migrationSource = func() fs.FS { return dbmigrate.Embedded() }

// migrationDSN picks the connection the migrator uses. Outside development it
// must be MIGRATE_DATABASE_URL, a role that owns the schema: the application's
// own DATABASE_URL role is deliberately not allowed to alter it, so falling back
// to it would defeat that separation.
func migrationDSN(getenv func(string) string) (string, error) {
	appEnv := app.NormalizeAppEnv(getenv("APP_ENV"))
	if appEnv == "" {
		return "", fmt.Errorf("APP_ENV must be set explicitly (development, local, test, staging or production)")
	}
	if dsn := strings.TrimSpace(getenv("MIGRATE_DATABASE_URL")); dsn != "" {
		return dsn, nil
	}
	if app.IsDevelopmentEnv(appEnv) {
		if dsn := strings.TrimSpace(getenv("DATABASE_URL")); dsn != "" {
			return dsn, nil
		}
	}
	return "", fmt.Errorf("MIGRATE_DATABASE_URL must be set when APP_ENV=%q (a schema-owning role, not the application role)", appEnv)
}

// runMigrate implements `aquify-api migrate <up|status>` and returns the exit code.
func runMigrate(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) != 1 || (args[0] != "up" && args[0] != "status") {
		fmt.Fprintln(stderr, "usage: aquify-api migrate <up|status>")
		return 2
	}
	dsn, err := migrationDSN(getenv)
	if err != nil {
		fmt.Fprintln(stderr, "migrate:", err)
		return 1
	}
	src := migrationSource()
	switch args[0] {
	case "up":
		applied, err := dbmigrate.Up(ctx, dsn, src)
		for _, a := range applied {
			fmt.Fprintf(stdout, "applied %s\n", a.Source)
		}
		if err != nil {
			fmt.Fprintln(stderr, "migrate:", err)
			return 1
		}
		fmt.Fprintf(stdout, "migrate up: %d applied, schema is current\n", len(applied))
	case "status":
		states, err := dbmigrate.Status(ctx, dsn, src)
		if err != nil {
			fmt.Fprintln(stderr, "migrate:", err)
			return 1
		}
		pending := 0
		for _, s := range states {
			mark := "applied"
			if !s.Applied {
				mark = "PENDING"
				pending++
			}
			fmt.Fprintf(stdout, "%-8s %s\n", mark, s.Source)
		}
		if pending > 0 {
			fmt.Fprintf(stderr, "migrate status: %d pending\n", pending)
			return 1
		}
	}
	return 0
}
