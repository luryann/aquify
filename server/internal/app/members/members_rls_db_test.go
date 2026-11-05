package members

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// the members tables work through the application's non-superuser
// role (create, restart reload, link, delete), and the database refuses unbound reads. The
// Keycloak owner lookup, made before any club is known, finds exactly one club.
func TestMembersRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := NewServiceWithPool(appPool)
	staff := func(org string) *appmiddleware.Principal {
		id := uuid.NewString()
		// deleted_by is a foreign key to users, so the actor must exist.
		if _, err := admin.Exec(ctx, `INSERT INTO users (id, org_id, email, oidc_subject, display_name, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3, $1, 'Admin', 'active', now(), now())`, id, org, id+"@example.test"); err != nil {
			t.Fatal(err)
		}
		return &appmiddleware.Principal{UserID: id, OrgID: org, Roles: []string{"admin"}}
	}
	type club struct{ hh, sw string }
	mk := func(org string) club {
		p := staff(org)
		hh, err := svc.CreateHousehold(p, CreateHouseholdInput{OrgID: org, HouseholdName: "H " + org[:4]})
		if err != nil {
			t.Fatal(err)
		}
		sw, _, err := svc.CreateSwimmer(p, org, CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "S", LastName: "W", DateOfBirth: "2012-01-01", Gender: "female"})
		if err != nil {
			t.Fatal(err)
		}
		return club{hh.ID, sw.ID}
	}
	a, b := mk(orgA), mk(orgB)

	// A restarted instance loads every club under that club's binding.
	restarted := NewServiceWithPool(appPool)
	if err := restarted.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	for org, c := range map[string]club{orgA: a, orgB: b} {
		hhs, err := restarted.ListHouseholds(staff(org), org)
		if err != nil || len(hhs) != 1 || hhs[0].ID != c.hh {
			t.Fatalf("club %s sees only its household after a restart: %+v %v", org, hhs, err)
		}
	}

	// The pre-auth owner lookup finds exactly the club the Keycloak user is linked to.
	if err := svc.repo.SetHouseholdKeycloakUserID(ctx, orgA, a.hh, "kc-user-a"); err != nil {
		t.Fatal(err)
	}
	if org, ok, err := svc.repo.OrgOfHouseholdKeycloakUser(ctx, "kc-user-a"); err != nil || !ok || org != orgA {
		t.Fatalf("the owner lookup resolves club A: %q %v %v", org, ok, err)
	}
	if _, ok, err := svc.repo.OrgOfHouseholdKeycloakUser(ctx, "nobody"); err != nil || ok {
		t.Fatalf("an unknown user resolves to no club: %v %v", ok, err)
	}
	// Another club cannot link a user onto this club's household.
	if err := svc.repo.SetHouseholdKeycloakUserID(ctx, orgB, a.hh, "intruder"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := svc.repo.OrgOfHouseholdKeycloakUser(ctx, "intruder"); ok {
		t.Fatal("club B must not link a user to club A's household")
	}

	// Deletes carry the club: B cannot delete A's swimmer, A can.
	if _, _, err := svc.DeleteSwimmer(staff(orgB), orgB, a.sw); err == nil {
		t.Fatal("club B must not delete club A's swimmer")
	}
	if _, _, err := svc.DeleteSwimmer(staff(orgA), orgA, a.sw); err != nil && !errors.Is(err, ErrSwimmerHasDependencies) {
		t.Fatalf("club A deletes its swimmer: %v", err)
	}

	for _, table := range []string{"households", "swimmers", "contacts"} {
		var n int
		if err := appPool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("unbound session must see nothing in %s: %d %v", table, n, err)
		}
	}
}
