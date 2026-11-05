package billing

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// Tenancy tracker 4.3 for billing: a billing request reloads only its own org's
// invoices and finance rows, replaces only those cache entries, and still sees a
// write another instance made to that org.
func TestBillingRequestReloadsOnlyTheCallersOrg(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	adminOf := func(org string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
	}
	writer := NewServiceWithPool(pool)
	invoice := func(org, number string) {
		hh := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'H')`, hh, org); err != nil {
			t.Fatal(err)
		}
		p := adminOf(org)
		inv, err := writer.CreateInvoice(org, p.UserID, p, CreateInvoiceInput{HouseholdID: hh, InvoiceNumber: number})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.AddInvoiceItem(org, p.UserID, p, inv.ID, AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: 5000, Category: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	invoice(orgA, "A-1")
	invoice(orgB, "B-1")

	reader := NewServiceWithPool(pool) // another instance; starts with a full load
	reader.mu.Lock()
	reader.invoices = map[string]*Invoice{} // as if it started before the data existed
	reader.mu.Unlock()
	cached := func() int {
		reader.mu.RLock()
		defer reader.mu.RUnlock()
		return len(reader.invoices)
	}

	got, err := reader.ListInvoices(orgA, adminOf(orgA), ListInvoicesFilters{})
	if err != nil || len(got) != 1 || got[0].OrgID != orgA {
		t.Fatalf("org A list: %+v %v", got, err)
	}
	if n := cached(); n != 1 {
		t.Fatalf("serving org A must not load org B's invoices into memory: %d cached", n)
	}
	if got, err := reader.ListInvoices(orgB, adminOf(orgB), ListInvoicesFilters{}); err != nil || len(got) != 1 || got[0].OrgID != orgB {
		t.Fatalf("org B list: %+v %v", got, err)
	}
	if n := cached(); n != 2 {
		t.Fatalf("rows of both orgs are kept once loaded, got %d", n)
	}

	invoice(orgA, "A-2") // written by the other instance
	if got, _ := reader.ListInvoices(orgA, adminOf(orgA), ListInvoicesFilters{}); len(got) != 2 {
		t.Fatalf("a cross-instance write to org A must be visible: %d", len(got))
	}
	if n := cached(); n != 3 {
		t.Fatalf("reloading org A replaces only org A's rows: %d cached", n)
	}
}

// A slow billing read for one club must not block another club's billing request: the
// database read runs outside the service lock.
func TestSlowBillingReadForOneClubDoesNotBlockAnother(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, pool, orgA, orgB)
	svc := NewServiceWithPool(pool)
	var calls atomic.Int32
	const stall = 400 * time.Millisecond
	svc.SetSyncHookForTest(func() {
		if calls.Add(1) == 1 { // only the first read (club A's) stalls
			time.Sleep(stall)
		}
	})
	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		if err := svc.refreshForRead(orgA); err != nil {
			t.Errorf("club A: %v", err)
		}
	}()
	time.Sleep(60 * time.Millisecond) // A is now stalled between its read and its swap
	start := time.Now()
	if err := svc.refreshForRead(orgB); err != nil {
		t.Fatalf("club B: %v", err)
	}
	if took := time.Since(start); took > stall/2 {
		t.Fatalf("club B waited %v behind club A's stalled read (stall %v); reads must not hold the service lock", took, stall)
	}
	<-aDone
}

// The per-club change counters let a billing instance skip the full reload when no OTHER
// process has written: its own write-through changes do not count, another instance's do.
func TestBillingSkipsReloadUntilAnotherInstanceWrites(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org := uuid.NewString()
	testutil.SeedOrgs(t, pool, org)
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
	newHousehold := func() string {
		hh := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'H')`, hh, org); err != nil {
			t.Fatal(err)
		}
		return hh
	}
	a, b := NewServiceWithPool(pool), NewServiceWithPool(pool) // two processes: distinct writer ids
	var loads atomic.Int32
	a.SetSyncHookForTest(func() { loads.Add(1) })

	if err := a.refreshOrg(ctx, org); err != nil || loads.Load() != 1 {
		t.Fatalf("first refresh loads the club: loads=%d err=%v", loads.Load(), err)
	}
	if _, err := a.CreateInvoice(org, admin.UserID, admin, CreateInvoiceInput{HouseholdID: newHousehold(), InvoiceNumber: "A-1"}); err != nil {
		t.Fatal(err)
	}
	if err := a.refreshOrg(ctx, org); err != nil || loads.Load() != 1 {
		t.Fatalf("A's own write must not force a reload: loads=%d err=%v", loads.Load(), err)
	}
	if _, err := b.CreateInvoice(org, admin.UserID, admin, CreateInvoiceInput{HouseholdID: newHousehold(), InvoiceNumber: "B-1"}); err != nil {
		t.Fatal(err)
	}
	if err := a.refreshOrg(ctx, org); err != nil || loads.Load() != 2 {
		t.Fatalf("B's write must force exactly one reload: loads=%d err=%v", loads.Load(), err)
	}
	if got, err := a.ListInvoices(org, admin, ListInvoicesFilters{}); err != nil || len(got) != 2 {
		t.Fatalf("A sees both invoices after the reload: %d %v", len(got), err)
	}
	if err := a.refreshOrg(ctx, org); err != nil || loads.Load() != 2 {
		t.Fatalf("nothing new, so no reload: loads=%d err=%v", loads.Load(), err)
	}
}
