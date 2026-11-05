package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/meets"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// meets are written under each club's binding, a restarted instance
// reloads every club, and the database refuses unbound reads.
func TestMeetsRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := meets.NewServiceWithPool(appPool)
	staff := func(org string) *appmiddleware.Principal {
		id := uuid.NewString()
		// created_by is a foreign key to users, so the actor must exist.
		if _, err := admin.Exec(context.Background(), `INSERT INTO users (id, org_id, email, oidc_subject, display_name, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3, $1, 'Coach', 'active', now(), now())`, id, org, id+"@example.test"); err != nil {
			t.Fatal(err)
		}
		return &appmiddleware.Principal{UserID: id, OrgID: org, Roles: []string{"admin"}}
	}
	for _, org := range []string{orgA, orgB} {
		if _, _, err := svc.CreateMeet(staff(org), org, meets.CreateMeetInput{
			Name: "Invite " + org[:4], MeetType: "invitational", Course: "SCY",
			StartDate: "2027-01-10", EndDate: "2027-01-11", HostOrg: "Host", Location: "Pool",
		}); err != nil {
			t.Fatal(err)
		}
	}
	restarted := meets.NewServiceWithPool(appPool)
	if err := restarted.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, org := range []string{orgA, orgB} {
		got, err := restarted.ListMeets(staff(org), org)
		if err != nil || len(got) != 1 || got[0].OrgID != org || got[0].Name != "Invite "+org[:4] {
			t.Fatalf("club %s sees only its meet after a restart: %+v %v", org, got, err)
		}
	}
	var n int
	if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM meets`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("unbound session must see nothing: %d %v", n, err)
	}
}
