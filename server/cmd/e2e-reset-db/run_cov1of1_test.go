package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func cov1xEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestCov1xRunRequiresDatabaseURL(t *testing.T) {
	var buf bytes.Buffer
	if code := run(cov1xEnv(nil), &buf); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(buf.String(), "DATABASE_URL is required") {
		t.Fatalf("stderr = %q", buf.String())
	}
}

func TestCov1xRunRefusesWithoutOptIn(t *testing.T) {
	var buf bytes.Buffer
	code := run(cov1xEnv(map[string]string{
		"DATABASE_URL": "postgres://u:p@127.0.0.1:5432/aquify_e2e?sslmode=disable",
	}), &buf)
	if code != 1 || !strings.Contains(buf.String(), "refusing to reset database: E2E_ALLOW_DB_RESET=true is required") {
		t.Fatalf("code=%d stderr=%q", code, buf.String())
	}
}

func TestCov1xValidateResetTargetEdgeCases(t *testing.T) {
	if err := validateResetTarget("://bad", "true"); err == nil || !strings.Contains(err.Error(), "parse DATABASE_URL") {
		t.Fatalf("unparseable URL err = %v", err)
	}
	if err := validateResetTarget("postgres://u:p@127.0.0.1:5432/?sslmode=disable", "true"); err == nil || !strings.Contains(err.Error(), "database name is empty") {
		t.Fatalf("empty db name err = %v", err)
	}
	if err := validateResetTarget("postgres://u:p@127.0.0.1:5432/aquify_staging_e2e?sslmode=disable", "true"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("staging name err = %v", err)
	}
}

func TestCov1xRunReportsOpenFailure(t *testing.T) {
	var buf bytes.Buffer
	// Port 1 on loopback refuses connections, so the ping fails.
	code := run(cov1xEnv(map[string]string{
		"DATABASE_URL":       "postgres://u:p@127.0.0.1:1/aquify_e2e?sslmode=disable&connect_timeout=2",
		"E2E_ALLOW_DB_RESET": "true",
	}), &buf)
	if code != 1 || !strings.Contains(buf.String(), "open db:") {
		t.Fatalf("code=%d stderr=%q", code, buf.String())
	}
}

func TestCov1xMainExitsWithRunCode(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	got := -1
	old := exit
	exit = func(c int) { got = c }
	t.Cleanup(func() { exit = old })
	main()
	if got != 1 {
		t.Fatalf("exit code = %d, want 1", got)
	}
}

// cov1xScratchDB creates a throwaway database (and returns a URL to it for the
// given user) on the server DATABASE_URL points at, without touching existing
// databases. It skips when DATABASE_URL is unset.
func cov1xScratchDB(t *testing.T) (adminURL *url.URL, dbName string, conn *pgx.Conn) {
	t.Helper()
	raw := os.Getenv("DATABASE_URL")
	if raw == "" {
		t.Skip("DATABASE_URL is not set; skipping DB reset tests")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { admin.Close(context.Background()) })

	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	dbName = "cov1x_e2e_" + hex.EncodeToString(b)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
	})
	return u, dbName, admin
}

func cov1xURLFor(u *url.URL, user, pass, dbName string) string {
	c := *u
	c.User = url.UserPassword(user, pass)
	c.Path = "/" + dbName
	return c.String()
}

func TestCov1xRunResetsPublicSchema(t *testing.T) {
	u, dbName, _ := cov1xScratchDB(t)
	pass, _ := u.User.Password()
	target := cov1xURLFor(u, u.User.Username(), pass, dbName)

	ctx := context.Background()
	c, err := pgx.Connect(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if _, err := c.Exec(ctx, "CREATE TABLE cov1x_marker (id int)"); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	code := run(cov1xEnv(map[string]string{"DATABASE_URL": target, "E2E_ALLOW_DB_RESET": "true"}), &buf)
	if code != 0 || buf.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, buf.String())
	}

	var tables, schemas int
	if err := c.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_name='cov1x_marker'").Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if err := c.QueryRow(ctx, "SELECT count(*) FROM information_schema.schemata WHERE schema_name='public'").Scan(&schemas); err != nil {
		t.Fatal(err)
	}
	if tables != 0 || schemas != 1 {
		t.Fatalf("marker tables=%d public schemas=%d, want 0 and 1", tables, schemas)
	}
}

func TestCov1xRunReportsResetFailureForNonOwner(t *testing.T) {
	u, dbName, admin := cov1xScratchDB(t)
	ctx := context.Background()
	role := dbName + "_role"
	if _, err := admin.Exec(ctx, "CREATE ROLE "+role+" LOGIN PASSWORD 'cov1x'"); err != nil {
		t.Skipf("cannot create role: %v", err)
	}
	t.Cleanup(func() {
		// The role holds CONNECT on the scratch database, so drop the database first.
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
		_, _ = admin.Exec(context.Background(), "DROP ROLE IF EXISTS "+role)
	})
	if _, err := admin.Exec(ctx, "GRANT CONNECT ON DATABASE "+dbName+" TO "+role); err != nil {
		t.Fatal(err)
	}
	// The role does not own schema public, so DROP SCHEMA must fail.
	target := cov1xURLFor(u, role, "cov1x", dbName)

	var buf bytes.Buffer
	code := run(cov1xEnv(map[string]string{"DATABASE_URL": target, "E2E_ALLOW_DB_RESET": "true"}), &buf)
	if code != 1 || !strings.Contains(buf.String(), "reset schema:") {
		t.Fatalf("code=%d stderr=%q", code, buf.String())
	}
}
