package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/groups"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// groups are written under each club's binding, a restarted instance
// reloads every club, and the database refuses unbound reads.
func TestGroupsRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := groups.NewServiceWithPool(appPool)
	staff := func(org string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
	}
	for _, org := range []string{orgA, orgB} {
		if _, _, err := svc.CreateGroup(staff(org), org, groups.CreateGroupInput{Code: "GOLD", Name: "Gold " + org[:4]}); err != nil {
			t.Fatal(err)
		}
	}
	restarted := groups.NewServiceWithPool(appPool)
	if err := restarted.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, org := range []string{orgA, orgB} {
		got, err := restarted.ListGroups(staff(org), org, false)
		if err != nil || len(got) != 1 || got[0].OrgID != org || got[0].Name != "Gold "+org[:4] {
			t.Fatalf("club %s sees only its group after a restart: %+v %v", org, got, err)
		}
	}
	var n int
	if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM groups`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("unbound session must see nothing: %d %v", n, err)
	}
}
