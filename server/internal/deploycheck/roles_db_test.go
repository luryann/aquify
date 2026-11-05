package deploycheck

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/luryann/aquify/server/internal/dbmigrate"
)

// This is the acceptance test for the production database role model
// (deploy/postgres/init/roles.sql.in): it bootstraps the roles in a scratch
// database, runs EVERY migration as the non-superuser owner, and then proves the
// application role is subject to row-level security. It needs DATABASE_URL to
// point at a superuser connection to a disposable Postgres.

func dsnFor(t *testing.T, base, user, password, db string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(user, password)
	u.Path = "/" + db
	return u.String()
}

func TestRoleBootstrapGivesTheAppRoleRowLevelSecurityAndNoDDL(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if base == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)

	tag := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	dbName := "aqt_" + tag
	names := map[string]string{
		"owner": "aqt_" + tag + "_owner", "app": "aqt_" + tag + "_app",
		"backup": "aqt_" + tag + "_backup", "kc": "aqt_" + tag + "_kc",
	}
	pw := "pw-" + strings.ReplaceAll(uuid.NewString(), "-", "")

	if _, err := admin.Exec(ctx, `CREATE DATABASE `+dbName); err != nil {
		t.Skipf("cannot create a scratch database: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, base)
		if err != nil {
			return
		}
		defer c.Close(ctx)
		_, _ = c.Exec(ctx, `DROP DATABASE IF EXISTS `+dbName+` WITH (FORCE)`)
		for _, r := range names {
			_, _ = c.Exec(ctx, `DROP ROLE IF EXISTS `+r)
		}
	})

	// Run the real role script on a connection to the scratch database.
	su := dsnFor(t, base, mustUser(t, base), mustPassword(base), dbName)
	conn, err := pgx.Connect(ctx, su)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for k, v := range map[string]string{
		"aquify.db_name": dbName, "aquify.owner_user": names["owner"], "aquify.owner_password": pw,
		"aquify.app_user": names["app"], "aquify.app_password": pw,
		"aquify.backup_user": names["backup"], "aquify.backup_password": pw,
		"aquify.kc_user": names["kc"], "aquify.kc_password": pw,
	} {
		if _, err := conn.Exec(ctx, `SELECT set_config($1, $2, false)`, k, v); err != nil {
			t.Fatal(err)
		}
	}
	script, err := os.ReadFile(repoFile(t, "deploy/postgres/init/roles.sql.in"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, string(script)); err != nil {
		t.Fatalf("roles.sql.in: %v", err)
	}
	// Idempotent: running it again must succeed and keep the same shape.
	if _, err := conn.Exec(ctx, string(script)); err != nil {
		t.Fatalf("roles.sql.in is not re-runnable: %v", err)
	}

	// All migrations run as the non-superuser owner.
	ownerDSN := dsnFor(t, base, names["owner"], pw, dbName)
	applied, err := dbmigrate.Up(ctx, ownerDSN, os.DirFS(repoFile(t, "db/migrations")))
	if err != nil {
		t.Fatalf("migrations as the non-superuser owner: %v (applied %d)", err, len(applied))
	}
	if len(applied) == 0 {
		t.Fatal("no migrations applied")
	}

	// Seed two clubs and a household in the first one, as the superuser.
	orgA, orgB, household := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO orgs (id) VALUES ($1::uuid), ($2::uuid)`, []any{orgA, orgB}},
		{`INSERT INTO households (id, org_id, household_name) VALUES ($1::uuid, $2::uuid, 'Seeded')`, []any{household, orgA}},
	} {
		if _, err := conn.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	app, err := pgx.Connect(ctx, dsnFor(t, base, names["app"], pw, dbName))
	if err != nil {
		t.Fatalf("connect as app role: %v", err)
	}
	defer app.Close(ctx)

	var super, bypass bool
	if err := app.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&super, &bypass); err != nil {
		t.Fatal(err)
	}
	if super || bypass {
		t.Fatalf("app role rolsuper=%v rolbypassrls=%v; both must be false", super, bypass)
	}

	count := func(c *pgx.Conn) int {
		var n int
		if err := c.QueryRow(ctx, `SELECT count(*) FROM households`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(app); n != 0 {
		t.Fatalf("unbound app session sees %d households; RLS should hide all of them", n)
	}
	bind := func(c *pgx.Conn, org string) {
		if _, err := c.Exec(ctx, `SELECT set_config('app.org_id', $1, false)`, org); err != nil {
			t.Fatal(err)
		}
	}
	bind(app, orgB)
	if n := count(app); n != 0 {
		t.Fatalf("session bound to another club sees %d households", n)
	}
	if _, err := app.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES (gen_random_uuid(), $1::uuid, 'Cross-club write')`, orgA); err == nil {
		t.Fatal("app role wrote a row for a club it is not bound to")
	}
	bind(app, orgA)
	if n := count(app); n != 1 {
		t.Fatalf("session bound to the right club sees %d households, want 1", n)
	}
	if _, err := app.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES (gen_random_uuid(), $1::uuid, 'Own-club write')`, orgA); err != nil {
		t.Fatalf("app role cannot write its own club's data: %v", err)
	}

	// No DDL for the application role.
	if _, err := app.Exec(ctx, `CREATE TABLE app_made_this (id int)`); err == nil {
		t.Fatal("app role created a table")
	}
	if _, err := app.Exec(ctx, `DROP TABLE households`); err == nil {
		t.Fatal("app role dropped a table")
	}

	// The backup role reads every club's rows (BYPASSRLS) but cannot write.
	bk, err := pgx.Connect(ctx, dsnFor(t, base, names["backup"], pw, dbName))
	if err != nil {
		t.Fatalf("connect as backup role: %v", err)
	}
	defer bk.Close(ctx)
	if n := count(bk); n < 2 {
		t.Fatalf("backup role sees %d households, want every row", n)
	}
	if _, err := bk.Exec(ctx, `DELETE FROM households`); err == nil {
		t.Fatal("backup role deleted rows")
	}

	// Keycloak's role has no access to the application database.
	if c, err := pgx.Connect(ctx, dsnFor(t, base, names["kc"], pw, dbName)); err == nil {
		c.Close(ctx)
		t.Fatal("the keycloak role can connect to the application database")
	}
}

func mustUser(t *testing.T, base string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil || u.User == nil {
		t.Fatalf("DATABASE_URL has no user: %v", err)
	}
	return u.User.Username()
}

func mustPassword(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.User == nil {
		return ""
	}
	p, _ := u.User.Password()
	return p
}
