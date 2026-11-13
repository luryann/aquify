package billing

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func cov1xDBOrgCacheEviction(t *testing.T, e *cov1xEnv) {
	other := cov1xEnvOn(t, e.pool)
	first := e.issued(t, "EVICT-1", 1000)
	second := other.issued(t, "EVICT-2", 1000)

	// A service that may cache only one club at a time evicts the club served least recently.
	svc := NewServiceWithPool(e.pool)
	svc.SetOrgCacheLimits(time.Hour, 1)
	if _, err := svc.GetInvoice(e.org, e.admin, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetInvoice(other.org, other.admin, second.ID); err != nil {
		t.Fatal(err)
	}
	svc.mu.RLock()
	_, firstCached := svc.invoices[first.ID]
	_, secondCached := svc.invoices[second.ID]
	svc.mu.RUnlock()
	if firstCached || !secondCached {
		t.Fatalf("expected only the most recent club cached, first=%v second=%v", firstCached, secondCached)
	}
	// The evicted club is transparently reloaded on its next request.
	if got, err := svc.GetInvoice(e.org, e.admin, first.ID); err != nil || got.ID != first.ID {
		t.Fatalf("evicted club must reload: %+v %v", got, err)
	}
}

func cov1xDBBillingRunsSurviveReload(t *testing.T, e *cov1xEnv) {
	swimmer := uuid.NewString()
	e.mustExec(t, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'Ann', 'Cov', '2012-01-01', 'undisclosed')`, swimmer, e.org, e.hh)
	e.svc.SetRosterSource(func(string) (RosterSnapshot, error) {
		return RosterSnapshot{
			Households: []RosterHousehold{{ID: e.hh, Name: "Cov1x household", Status: "active"}},
			Swimmers:   []RosterSwimmer{{ID: swimmer, HouseholdID: e.hh, FirstName: "Ann", LastName: "Cov", DateOfBirth: "2012-01-01", Status: "active"}},
			GroupLinks: []RosterGroupLink{{SwimmerID: swimmer, GroupCode: "GOLD", IsPrimary: true, AssignedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}},
			Registrations: []RosterRegistration{
				{ID: "r1", HouseholdID: e.hh, SwimmerID: swimmer, GroupCode: "GOLD", Season: "2026-27", Status: "active"},
			},
			Groups: []RosterGroup{{Code: "GOLD", Name: "Gold"}},
		}, nil
	})
	sched, err := e.svc.CreateFeeSchedule(e.org, "a", e.admin, FeeScheduleInput{
		Name: "Gold dues", Kind: ScheduleKindDues, GroupCode: "gold", Frequency: FrequencyMonthly, AmountCents: 10000, StartDate: "2026-09-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.CommitBillingRun(e.org, "a", e.admin, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}, RunTriggerManual)
	if err != nil || len(res.Invoices) != 1 {
		t.Fatalf("run: %+v %v", res, err)
	}

	// Another instance loads the run and its charges from Postgres.
	other := NewServiceWithPool(e.pool)
	runs, err := other.ListBillingRuns(e.org, e.admin)
	if err != nil || len(runs) != 1 || runs[0].ID != res.Run.ID {
		t.Fatalf("runs: %+v %v", runs, err)
	}
	other.mu.Lock()
	billed := other.scheduleHasChargesLocked(sched.ID)
	other.mu.Unlock()
	if !billed {
		t.Fatal("charges must be loaded with the finance state")
	}
	frozen := FeeScheduleInput{Name: "Gold dues", Kind: ScheduleKindDues, GroupCode: "GOLD", Frequency: FrequencyQuarterly, AmountCents: 10000, StartDate: "2026-09-01"}
	if _, _, err := other.UpdateFeeSchedule(e.org, "a", e.admin, sched.ID, frozen); !errors.Is(err, ErrScheduleLocked) {
		t.Fatalf("the billed schedule must be frozen on every instance: %v", err)
	}

	// Voiding the run's invoice releases its charges in the database and in memory.
	invoiceID := res.Invoices[0].InvoiceID
	if _, _, err := e.svc.VoidInvoice(e.org, "a", e.admin, invoiceID); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM billing_charges WHERE invoice_id = $1::uuid AND released_at IS NULL`, invoiceID); n != 0 {
		t.Fatalf("voiding must release the invoice's charges, %d still active", n)
	}
}
