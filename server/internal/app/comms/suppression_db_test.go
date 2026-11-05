package comms

import (
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

// System actors such as "postmark-webhook" must not reach the uuid created_by column.
func TestApplyPostmarkBouncePersistsSuppression(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	svc := NewServiceWithPool(pool)

	if err := svc.ApplyPostmarkEvent(PostmarkEventInput{OrgID: org, RecipientEmail: "Bounce@Example.com", RecordType: "Bounce"}); err != nil {
		t.Fatalf("bounce: %v", err)
	}
	if err := svc.ApplyPostmarkEvent(PostmarkEventInput{OrgID: org, RecipientEmail: "sub@example.com", RecordType: "SubscriptionChange", SuppressSending: true, SuppressionReason: "manual"}); err != nil {
		t.Fatalf("subscription change: %v", err)
	}
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM suppression_list WHERE org_id = $1::uuid AND created_by IS NULL`, org).Scan(&n); err != nil || n != 2 {
		t.Fatalf("persisted suppression rows = %d err=%v", n, err)
	}
	reloaded := NewServiceWithPool(pool)
	if err := reloaded.RefreshOrg(t.Context(), org); err != nil { // clubs load on demand, not at startup
		t.Fatal(err)
	}
	if len(reloaded.suppression) != 2 {
		t.Fatalf("reload suppression = %d", len(reloaded.suppression))
	}
}
