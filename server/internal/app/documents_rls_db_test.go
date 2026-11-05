package app

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/documents"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/tenantdb"
	"github.com/luryann/aquify/server/internal/testutil"
)

// waivers and medical forms. Besides isolation, the
// cross-domain reader matters: members refuses to delete a swimmer who still has
// document submissions, and that count must not silently become zero under RLS.
func TestDocumentsRowLevelSecurityAndTheSwimmerDeleteGuard(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	docs := documents.NewServiceWithPool(appPool) // the application's pool: RLS applies
	mem := members.NewServiceWithPool(appPool)
	adminOf := func(org string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
	}

	type club struct{ hh, sw, typ, sub string }
	mk := func(org string) club {
		p := adminOf(org)
		hh, err := mem.CreateHousehold(p, members.CreateHouseholdInput{OrgID: org, HouseholdName: "H"})
		if err != nil {
			t.Fatal(err)
		}
		sw, _, err := mem.CreateSwimmer(p, org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "S", LastName: "W", DateOfBirth: "2012-01-01", Gender: "female"})
		if err != nil {
			t.Fatal(err)
		}
		dt, err := docs.CreateDocumentType(org, "Waiver "+org[:4], "waiver", "", "swimmer", true, true, nil, p.UserID)
		if err != nil {
			t.Fatal(err)
		}
		sub, err := docs.CreateSubmission(org, dt.ID, hh.ID, sw.ID, p.UserID)
		if err != nil {
			t.Fatal(err)
		}
		return club{hh.ID, sw.ID, dt.ID, sub.ID}
	}
	a, b := mk(orgA), mk(orgB)

	if got, err := docs.ListDocumentTypes(orgA, false); err != nil || len(got) != 1 || got[0].ID != a.typ {
		t.Fatalf("org A sees only its document type: %+v %v", got, err)
	}
	if _, err := docs.GetSubmission(orgA, b.sub); err == nil {
		t.Fatal("org A must not read org B's submission")
	}
	count := func(bound string) int {
		var n int
		var err error
		if bound == "" {
			err = appPool.QueryRow(ctx, `SELECT count(*) FROM document_submissions`).Scan(&n)
		} else {
			err = tenantdb.Pool{P: appPool}.For(bound).QueryRow(ctx, `SELECT count(*) FROM document_submissions`).Scan(&n)
		}
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("") != 0 || count(orgA) != 1 || count(orgB) != 1 {
		t.Fatalf("unbound=%d A=%d B=%d, want 0/1/1", count(""), count(orgA), count(orgB))
	}

	// The delete guard still sees the swimmer's submission under RLS.
	if _, _, err := mem.DeleteSwimmer(adminOf(orgA), orgA, a.sw); !errors.Is(err, members.ErrSwimmerHasDependencies) {
		t.Fatalf("a swimmer with a document submission must not be deletable (guard must not count zero under RLS): %v", err)
	}
}
