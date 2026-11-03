package handlers

import (
	"context"
	"testing"

	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
	"github.com/google/uuid"
)

// A household created on the API instance after the worker started must be
// included when the worker resolves campaign recipients.
func TestResolveCampaignRecipientsObservesOtherInstanceWrites(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	apiMembers := members.NewServiceWithPool(pool)
	workerMembers := members.NewServiceWithPool(pool)
	workerGroups := groups.NewServiceWithPool(pool)
	worker := NewCommsHandlers(comms.NewService(), workerMembers, workerGroups)
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: org, Roles: []string{"admin"}}

	// Worker has already loaded its (empty) caches.
	if err := workerMembers.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := apiMembers.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Late", PrimaryEmail: "late@example.com"}); err != nil {
		t.Fatal(err)
	}
	got, err := worker.resolveCampaignRecipients(context.Background(), org, &comms.EmailCampaign{OrgID: org, Audience: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Email != "late@example.com" {
		t.Fatalf("recipients = %+v, want late@example.com", got)
	}
}
