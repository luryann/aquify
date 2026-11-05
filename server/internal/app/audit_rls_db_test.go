package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/testutil"
)

// the audit trail is isolated by the database, and an unbound
// session (no club) sees no events at all.
func TestAuditRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := audit.NewService()
	svc.SetPool(appPool)
	for _, org := range []string{orgA, orgB} {
		if _, err := svc.Append(audit.AppendRequest{OrgID: org, ActorUserID: "u", Action: "test.action", ResourceType: "thing", ResourceID: "1"}); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := svc.Query(audit.Filter{OrgID: orgA}); err != nil || len(got) != 1 || got[0].OrgID != orgA {
		t.Fatalf("org A sees only its own events: %+v %v", got, err)
	}
	if got, err := svc.Query(audit.Filter{}); err != nil || len(got) != 0 {
		t.Fatalf("a query with no club must see nothing: %+v %v", got, err)
	}
	var n int
	if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("unbound session must see nothing: %d %v", n, err)
	}
}
