package app

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/compliance"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// compliance requests are read per club and isolated by the database.
func TestComplianceRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := compliance.NewServiceWithPool(appPool, 7, 30)
	mem := members.NewServiceWithPool(appPool)
	mkHH := func(org string) string {
		p := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
		hh, err := mem.CreateHousehold(p, members.CreateHouseholdInput{OrgID: org, HouseholdName: "H"})
		if err != nil {
			t.Fatal(err)
		}
		return hh.ID
	}
	hhA, hhB := mkHH(orgA), mkHH(orgB)
	a, err := svc.CreateDataExportRequest(orgA, hhA, "u")
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreateDataExportRequest(orgB, hhB, "u")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.ListDataExportRequests(orgA, ""); err != nil || len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("org A lists only its export: %+v %v", got, err)
	}
	if _, err := svc.GetDataExportRequest(orgA, b.ID); !errors.Is(err, compliance.ErrExportNotFound) {
		t.Fatalf("org A must not read org B's export: %v", err)
	}
	if _, err := svc.GetDataExportRequest(orgB, b.ID); err != nil {
		t.Fatalf("org B's export must survive org A's probe: %v", err)
	}
	var n int
	if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM data_export_requests`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("unbound session must see nothing: %d %v", n, err)
	}
}
