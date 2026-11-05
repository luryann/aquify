package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/exports"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/testutil"
)

// exports are loaded per club and isolated by the database, and the
// purge sweep for one club never touches another club's artifacts.
func TestExportsRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	store := storage.NewStubStorage("") // shared by both instances, like one bucket
	svc := exports.NewServiceWithPool(appPool, store)
	staff := func(org string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
	}
	gen := func(org string) *exports.FileExport {
		ex, _, err := svc.GenerateExport(staff(org), org, "", exports.ReportInput{ReportType: exports.ReportTypeEvent, Meet: exports.ReportMeet{Name: "Meet " + org[:4]}, GeneratedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		return ex
	}
	a, b := gen(orgA), gen(orgB)

	// A second instance (fresh cache) sees only the caller's club.
	other := exports.NewServiceWithPool(appPool, store)
	if got, err := other.ListExports(staff(orgA), orgA, ""); err != nil || len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("org A lists only its export: %+v %v", got, err)
	}
	if _, err := other.GetExport(staff(orgA), orgA, b.ID); !errors.Is(err, exports.ErrExportNotFound) {
		t.Fatalf("org A must not read org B's export: %v", err)
	}
	var n int
	if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM file_exports`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("unbound session must see nothing: %d %v", n, err)
	}

	// The purge sweep is per club: sweeping A expires only A's export.
	future := time.Now().Add(365 * 24 * time.Hour)
	got, err := other.ExpireArtifactsForOrg(orgA, future)
	if err != nil || len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("sweep for A expires only A's export: %+v %v", got, err)
	}
	if _, err := other.ExpireArtifactsForOrg("", future); err == nil {
		t.Fatal("a sweep with no club must be refused in database mode")
	}
	if got, err := other.ExpireArtifactsForOrg(orgB, future); err != nil || len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("B's export is still pending for B's own sweep: %+v %v", got, err)
	}
}
