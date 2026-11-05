package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A superuser connection must be refused outside development; a plain role
// without BYPASSRLS must be accepted. Needs DATABASE_URL to be a superuser on a
// disposable Postgres (it creates and drops a throwaway login role).
func TestValidateDatabaseRoleRejectsRolesThatBypassRLS(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if url == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	if err := validateDatabaseRole(ctx, admin); err == nil {
		t.Fatal("a superuser connection was accepted")
	} else if !strings.Contains(err.Error(), "row-level security") {
		t.Fatalf("unhelpful error: %v", err)
	}

	for _, tc := range []struct {
		name, attrs string
		wantErr     bool
	}{
		{"restricted", "NOSUPERUSER NOBYPASSRLS", false},
		{"bypassrls", "NOSUPERUSER BYPASSRLS", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			role := "aqt_role_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
			pw := strings.ReplaceAll(uuid.NewString(), "-", "")
			if _, err := admin.Exec(ctx, `CREATE ROLE `+role+` LOGIN `+tc.attrs+` PASSWORD '`+pw+`'`); err != nil {
				t.Skipf("cannot create a role: %v", err)
			}
			t.Cleanup(func() { _, _ = admin.Exec(ctx, `DROP ROLE IF EXISTS `+role) })
			cfg, _ := pgxpool.ParseConfig(url)
			cfg.ConnConfig.User, cfg.ConnConfig.Password = role, pw
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			err = validateDatabaseRole(ctx, pool)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
