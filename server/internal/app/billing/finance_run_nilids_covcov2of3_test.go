package billing

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCov2yDBUpsertRunStoresMissingScheduleIDsAsEmptyList(t *testing.T) {
	env := cov2xNewDBEnv(t)
	run := &BillingRun{
		ID: uuid.NewString(), OrgID: env.org, AsOf: "2026-11-01", InvoiceStatus: "draft",
		Trigger: RunTriggerManual, CreatedAt: time.Now().UTC(),
	}
	if err := upsertRun(context.Background(), env.pool, run); err != nil {
		t.Fatalf("a run without schedule ids must persist: %v", err)
	}
	if n := env.count(`SELECT cardinality(schedule_ids) FROM billing_runs WHERE id = $1`, run.ID); n != 0 {
		t.Fatalf("schedule ids should be an empty array, cardinality %d", n)
	}
	st, err := env.svc.repo.loadFinanceState(context.Background(), env.org)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.runs[run.ID]; got == nil || len(got.ScheduleIDs) != 0 {
		t.Fatalf("round trip: %+v", got)
	}
}
