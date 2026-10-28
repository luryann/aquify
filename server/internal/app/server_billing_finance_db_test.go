package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// TestDBRepoBillingFinance proves the finance tables round-trip, and that two
// service instances sharing Postgres (two API pods / API + worker) committing
// the same billing run concurrently never double-bill, then re-running bills
// nothing. Credits and late fees are verified for cross-instance visibility.
func TestDBRepoBillingFinance(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	orgID := uuid.NewString()
	hhA, hhB := uuid.NewString(), uuid.NewString()
	sw1, sw2, sw3 := uuid.NewString(), uuid.NewString(), uuid.NewString()
	t.Cleanup(func() { cleanupBillingFinanceFixtures(t, pool, orgID) })
	for _, h := range [][2]string{{hhA, "Alpha"}, {hhB, "Bravo"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, $3)`, h[0], orgID, h[1]); err != nil {
			t.Fatalf("seed household: %v", err)
		}
	}
	for _, s := range [][2]string{{sw1, hhA}, {sw2, hhA}, {sw3, hhB}} {
		if _, err := pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender) VALUES ($1, $2, $3, 'Test', 'Swimmer', '2013-01-01', 'female')`, s[0], orgID, s[1]); err != nil {
			t.Fatalf("seed swimmer: %v", err)
		}
	}
	joined := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	roster := func(string) (billing.RosterSnapshot, error) {
		return billing.RosterSnapshot{
			Households: []billing.RosterHousehold{{ID: hhA, Name: "Alpha", Status: "active"}, {ID: hhB, Name: "Bravo", Status: "active"}},
			Swimmers: []billing.RosterSwimmer{
				{ID: sw1, HouseholdID: hhA, FirstName: "A", LastName: "One", DateOfBirth: "2012-01-01", Status: "active"},
				{ID: sw2, HouseholdID: hhA, FirstName: "B", LastName: "Two", DateOfBirth: "2014-01-01", Status: "active"},
				{ID: sw3, HouseholdID: hhB, FirstName: "C", LastName: "Three", DateOfBirth: "2013-01-01", Status: "active"},
			},
			GroupLinks: []billing.RosterGroupLink{
				{SwimmerID: sw1, GroupCode: "GOLD", AssignedAt: joined},
				{SwimmerID: sw2, GroupCode: "GOLD", AssignedAt: joined},
				{SwimmerID: sw3, GroupCode: "GOLD", AssignedAt: joined},
			},
		}, nil
	}
	admin := &appmiddleware.Principal{UserID: "admin", OrgID: orgID, Roles: []string{"admin"}, MFAAuthenticated: true}

	svc1 := billing.NewServiceWithPool(pool)
	svc1.SetRosterSource(roster)
	if _, err := svc1.CreateFeeSchedule(orgID, "admin", admin, billing.FeeScheduleInput{Name: "Gold", Kind: "dues", GroupCode: "GOLD", Frequency: "monthly", AmountCents: 10000, StartDate: "2026-09-01"}); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if _, err := svc1.CreateDiscountRule(orgID, "admin", admin, billing.DiscountRuleInput{Name: "Sibling", AppliesTo: "dues", MinPosition: 2, DiscountType: "fixed", AmountCents: 1500}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, _, err := svc1.AddManualCredit(orgID, "admin", admin, hhB, billing.ManualCreditInput{Direction: "credit", AmountCents: 4000, Reason: "Carry-over"}); err != nil {
		t.Fatalf("manual credit: %v", err)
	}

	// Second instance loads state written by the first.
	svc2 := billing.NewServiceWithPool(pool)
	svc2.SetRosterSource(roster)
	if schedules, err := svc2.ListFeeSchedules(orgID, admin); err != nil || len(schedules) != 1 {
		t.Fatalf("instance 2 must see schedule: %v %v", schedules, err)
	}

	input := billing.RunInput{AsOf: "2026-11-05", InvoiceStatus: "issued", DueDate: "2026-11-20"}
	var wg sync.WaitGroup
	invoices := make(chan int, 6)
	for i := 0; i < 6; i++ {
		svc := svc1
		if i%2 == 1 {
			svc = svc2
		}
		wg.Add(1)
		go func(svc *billing.Service) {
			defer wg.Done()
			if res, err := svc.CommitBillingRun(orgID, "admin", admin, input, billing.RunTriggerManual); err == nil {
				invoices <- res.Run.InvoiceCount
			}
		}(svc)
	}
	wg.Wait()
	close(invoices)
	total := 0
	for n := range invoices {
		total += n
	}
	var dbInvoices, dbCharges int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM invoices WHERE org_id = $1`, orgID).Scan(&dbInvoices); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_charges WHERE org_id = $1`, orgID).Scan(&dbCharges); err != nil {
		t.Fatal(err)
	}
	if total != 2 || dbInvoices != 2 || dbCharges != 3 {
		t.Fatalf("concurrent cross-instance commits double-billed: reported=%d invoices=%d charges=%d", total, dbInvoices, dbCharges)
	}

	// A fresh instance (restart) sees the billed keys and the credit applied.
	svc3 := billing.NewServiceWithPool(pool)
	svc3.SetRosterSource(roster)
	preview, err := svc3.PreviewBillingRun(orgID, admin, input)
	if err != nil || preview.HouseholdCount != 0 {
		t.Fatalf("rerun after restart must bill nothing: %+v %v", preview, err)
	}
	credit, err := svc3.GetHouseholdCredit(orgID, admin, hhB)
	if err != nil || credit.BalanceCents != 0 {
		t.Fatalf("bravo credit should be fully applied (10000 invoice, 4000 credit): %+v %v", credit, err)
	}
	bravo, _ := svc3.ListInvoices(orgID, admin, billing.ListInvoicesFilters{HouseholdID: hhB})
	if len(bravo) != 1 || bravo[0].AmountPaidCents != 4000 || bravo[0].AmountDueCents != 6000 || bravo[0].Status != "partially_paid" {
		t.Fatalf("bravo invoice after credit (persisted amounts): %+v", bravo)
	}
	alpha, _ := svc3.ListInvoices(orgID, admin, billing.ListInvoicesFilters{HouseholdID: hhA})
	if len(alpha) != 1 || alpha[0].TotalCents != 18500 {
		t.Fatalf("alpha invoice must include sibling discount (200.00 - 15.00): %+v", alpha)
	}

	// Late fees: concurrent sweeps on two instances apply exactly one per invoice.
	if _, _, err := svc1.UpdateBillingSettings(orgID, "admin", admin, billing.BillingSettingsInput{LateFeeEnabled: true, LateFeeGraceDays: 3, LateFeeType: "percent", LateFeePercentBps: 1000, LateFeeMaxCents: 1500, AutoRunDay: 1, AutoRunInvoiceStatus: "draft"}); err != nil {
		t.Fatal(err)
	}
	sweepDay := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	var wg2 sync.WaitGroup
	for _, svc := range []*billing.Service{svc1, svc2} {
		wg2.Add(1)
		go func(svc *billing.Service) {
			defer wg2.Done()
			_, _ = svc.ApplyLateFees(orgID, "admin", admin, sweepDay)
		}(svc)
	}
	wg2.Wait()
	var fees, feeCents int
	if err := pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(amount_cents),0) FROM billing_late_fee_applications WHERE org_id = $1`, orgID).Scan(&fees, &feeCents); err != nil {
		t.Fatal(err)
	}
	// Alpha 10% of 185.00 capped at 15.00; Bravo 10% of 60.00 = 6.00.
	if fees != 2 || feeCents != 1500+600 {
		t.Fatalf("late fees must be once per invoice: count=%d cents=%d", fees, feeCents)
	}
	st, err := svc2.HouseholdStatement(orgID, admin, hhB, time.Time{}, sweepDay)
	if err != nil || st.ClosingBalanceCents != 6600 {
		t.Fatalf("bravo statement closing balance: %+v %v", st, err)
	}
}

func cleanupBillingFinanceFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`DELETE FROM billing_late_fee_applications WHERE org_id = $1`,
		`DELETE FROM household_credit_entries WHERE org_id = $1`,
		`DELETE FROM household_credit_balances WHERE org_id = $1`,
		`DELETE FROM billing_charges WHERE org_id = $1`,
		`DELETE FROM billing_adjustments WHERE org_id = $1`,
		`DELETE FROM billing_runs WHERE org_id = $1`,
		`DELETE FROM billing_discount_rules WHERE org_id = $1`,
		`DELETE FROM billing_fee_schedules WHERE org_id = $1`,
		`DELETE FROM billing_settings WHERE org_id = $1`,
		`DELETE FROM ledger_events WHERE org_id = $1`,
		`DELETE FROM refunds WHERE org_id = $1`,
		`DELETE FROM payments WHERE org_id = $1`,
		`DELETE FROM invoice_items WHERE org_id = $1`,
		`DELETE FROM invoices WHERE org_id = $1`,
		`DELETE FROM swimmers WHERE org_id = $1`,
		`DELETE FROM households WHERE org_id = $1`,
	} {
		if _, err := pool.Exec(ctx, stmt, orgID); err != nil {
			t.Logf("cleanup %q: %v", stmt, err)
		}
	}
}
