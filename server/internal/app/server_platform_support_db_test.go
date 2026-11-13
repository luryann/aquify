package app

import (
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/testutil"
)

// The operator log and the club's own log both receive a support action in Postgres, the
// operator is not a user of the club (no actor foreign key may reject the mirror), and the
// activity read finds the operator entries among hundreds of the club's own events.
func TestPlatformSupportAuditMirrorAndPrefixQueryInPostgres(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	platformOrg, club := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, platformOrg, club)
	svc := audit.NewService()
	svc.SetPool(appPool)
	operator := uuid.NewString() // exists in the platform org only

	for _, org := range []string{platformOrg, club} {
		if _, err := svc.Append(audit.AppendRequest{OrgID: org, ActorUserID: operator, ActorRoles: []string{"platform_admin"},
			Action: audit.ActionPlatformUserMFAReset, ResourceType: "keycloak_user", ResourceID: uuid.NewString(),
			Metadata: map[string]string{"reason": "lost phone", "target_org_id": club}}); err != nil {
			t.Fatalf("append to %s: %v", org, err)
		}
	}
	for i := 0; i < 250; i++ {
		if _, err := svc.Append(audit.AppendRequest{OrgID: club, ActorUserID: "club-admin", Action: "billing.invoice.issue", ResourceType: "invoice", ResourceID: uuid.NewString()}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.Query(audit.Filter{OrgID: club, ActionPrefix: "platform.", Limit: 50})
	if err != nil || len(got) != 1 || got[0].Action != audit.ActionPlatformUserMFAReset || got[0].Metadata["reason"] != "lost phone" {
		t.Fatalf("club activity: %+v %v", got, err)
	}
	if got, _ := svc.Query(audit.Filter{OrgID: platformOrg, ActionPrefix: "platform."}); len(got) != 1 {
		t.Fatalf("operator log: %+v", got)
	}
}
