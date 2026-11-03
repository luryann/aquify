package billing

import (
	"context"
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/google/uuid"
)

// An invoice created and issued by one Service must be visible, listable and
// payable on another Service sharing the same Postgres (second API replica or
// the worker process), in both directions.
func TestInvoicesVisibleAcrossInstances(t *testing.T) {
	pool := stripeTestPool(t)
	ctx := context.Background()
	org, household := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id,org_id,household_name) VALUES ($1,$2,'Cross instance')`, household, org); err != nil {
		t.Fatal(err)
	}
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
	a, b := NewServiceWithPool(pool), NewServiceWithPool(pool)

	mk := func(src *Service, num string) string {
		inv, err := src.CreateInvoice(org, admin.UserID, admin, CreateInvoiceInput{HouseholdID: household, InvoiceNumber: num})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := src.AddInvoiceItem(org, admin.UserID, admin, inv.ID, AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: 10000, Category: "manual"}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := src.IssueInvoice(org, admin.UserID, admin, inv.ID); err != nil {
			t.Fatal(err)
		}
		return inv.ID
	}

	for _, dir := range []struct {
		name     string
		src, dst *Service
	}{{"a-to-b", a, b}, {"b-to-a", b, a}} {
		id := mk(dir.src, "X-"+dir.name)
		got, err := dir.dst.GetInvoice(org, admin, id)
		if err != nil || got.Status != "issued" || got.AmountDueCents != 10000 {
			t.Fatalf("%s: get invoice: %+v %v", dir.name, got, err)
		}
		list, err := dir.dst.ListInvoices(org, admin, ListInvoicesFilters{})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, inv := range list {
			found = found || inv.ID == id
		}
		if !found {
			t.Fatalf("%s: invoice missing from list", dir.name)
		}
		items, err := dir.dst.ListInvoiceItems(org, admin, id)
		if err != nil || len(items) != 1 {
			t.Fatalf("%s: items: %d %v", dir.name, len(items), err)
		}
		if _, _, err := dir.dst.RecordPayment(org, admin.UserID, admin, id, RecordPaymentInput{AmountCents: 4000, Method: "cash"}); err != nil {
			t.Fatalf("%s: record payment: %v", dir.name, err)
		}
		// The originating instance observes the payment made elsewhere.
		back, err := dir.src.GetInvoice(org, admin, id)
		if err != nil || back.AmountDueCents != 6000 {
			t.Fatalf("%s: source view: %+v %v", dir.name, back, err)
		}
		if _, _, err := dir.dst.VoidInvoice(org, admin.UserID, admin, id); err == nil {
			// void of a partially paid invoice may be rejected; either way no 404.
			continue
		} else if err == ErrInvoiceNotFound {
			t.Fatalf("%s: void not found", dir.name)
		}
	}
}
