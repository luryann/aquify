package app

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/seasons"
	"github.com/luryann/aquify/server/internal/tenantdb"
	"github.com/luryann/aquify/server/internal/testutil"
)

// seasons, exercised through the application's
// non-superuser role.
func TestSeasonsRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := seasons.NewServiceWithPool(appPool)

	a, err := svc.Create(orgA, "2026-27", "2026-09-01", "2027-08-31", "active", "", "u")
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(orgB, "B season", "2026-09-01", "2027-08-31", "active", "", "u")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.List(orgA); err != nil || len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("org A lists only its season: %+v %v", got, err)
	}
	if _, err := svc.Get(orgA, b.ID); !errors.Is(err, seasons.ErrNotFound) {
		t.Fatalf("org A must not read org B's season: %v", err)
	}
	name := "hijack"
	if _, err := svc.Update(orgA, b.ID, seasons.SeasonUpdates{Name: &name}, "u"); !errors.Is(err, seasons.ErrNotFound) {
		t.Fatalf("org A must not update org B's season: %v", err)
	}
	if err := svc.Delete(orgA, b.ID); !errors.Is(err, seasons.ErrNotFound) {
		t.Fatalf("org A must not delete org B's season: %v", err)
	}
	count := func(bound string) int {
		var n int
		q := `SELECT count(*) FROM seasons`
		var err error
		if bound == "" {
			err = appPool.QueryRow(context.Background(), q).Scan(&n)
		} else {
			err = tenantdb.Pool{P: appPool}.For(bound).QueryRow(context.Background(), q).Scan(&n)
		}
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("") != 0 || count(orgA) != 1 || count(orgB) != 1 {
		t.Fatalf("unbound=%d A=%d B=%d, want 0/1/1", count(""), count(orgA), count(orgB))
	}
}
