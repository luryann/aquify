package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/documents"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// The API and the worker are separate processes sharing Postgres. A household
// created through the API after the worker started must be visible to the
// worker-side export collector and erasure steps without a restart.
func TestDBWorkerCollectsHouseholdCreatedAfterStartup(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	testutil.SeedOrgs(t, pool, org)
	admin := &appmiddleware.Principal{UserID: "admin-1", OrgID: org, Roles: []string{"admin"}}

	apiMembers := members.NewServiceWithPool(pool)
	workerMembers := members.NewServiceWithPool(pool) // started before the household exists
	workerComms := comms.NewServiceWithPool(pool)
	workerDocs := documents.NewServiceWithPool(pool)

	hh, err := apiMembers.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Late Family", PrimaryEmail: "late@example.test"})
	if err != nil {
		t.Fatal(err)
	}

	sources := buildHouseholdExportSources(householdExportDeps{comms: workerComms, documents: workerDocs})
	data, err := collectHouseholdExportData(org, hh.ID, workerMembers, nil, nil, sources...)
	if err != nil {
		t.Fatalf("worker collector: %v", err)
	}
	got, ok := data["household"].(*members.Household)
	if !ok || got.ID != hh.ID {
		t.Fatalf("household = %#v", data["household"])
	}

	if _, err := workerMembers.GetHouseholdErasureContext(org, hh.ID); err != nil {
		t.Fatalf("worker erasure context: %v", err)
	}
	if _, err := workerComms.EraseHouseholdData(context.Background(), org, hh.ID, nil); err != nil {
		t.Fatalf("worker comms erase: %v", err)
	}
	if _, err := workerDocs.EraseHouseholdData(context.Background(), org, hh.ID); err != nil {
		t.Fatalf("worker docs erase: %v", err)
	}
}
