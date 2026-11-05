package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/testutil"
)

// announcements and the suppression list are written under each club's
// binding, a restarted instance reloads every club, and the database refuses unbound reads.
func TestCommsRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := comms.NewServiceWithPool(appPool)
	for _, org := range []string{orgA, orgB} {
		if _, err := svc.CreateAnnouncement(comms.CreateAnnouncementInput{OrgID: org, Title: "T", Body: "B", Audience: "all", CreatedBy: uuid.NewString()}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.AddSuppression(comms.AddSuppressionInput{OrgID: org, Email: "x@" + org[:6] + ".test", Reason: "manual", CreatedBy: uuid.NewString()}); err != nil {
			t.Fatal(err)
		}
	}
	restarted := comms.NewServiceWithPool(appPool)
	for _, org := range []string{orgA, orgB} {
		anns, err := restarted.ListAnnouncements(org, "")
		if err != nil || len(anns) != 1 || anns[0].OrgID != org {
			t.Fatalf("club %s sees only its announcement after a restart: %+v %v", org, anns, err)
		}
		sup, err := restarted.ListSuppression(org)
		if err != nil || len(sup) != 1 || sup[0].OrgID != org {
			t.Fatalf("club %s sees only its suppression entry after a restart: %+v %v", org, sup, err)
		}
	}
	for _, table := range []string{"announcements", "suppression_list"} {
		var n int
		if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("unbound session must see nothing in %s: %d %v", table, n, err)
		}
	}
}
