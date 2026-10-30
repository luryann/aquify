package app

import (
	"context"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/volunteer"
)

func TestVolunteerBillerFailedIssueLeavesNoDraft(t *testing.T) {
	svc := billing.NewService()
	p := &appmiddleware.Principal{UserID: "admin-1", OrgID: "org-1", Roles: []string{"admin"}}
	b := volunteerBiller{billing: svc}
	// A zero-amount invoice cannot be issued, so the failure lands after CreateInvoice/AddInvoiceItem.
	_, err := b.CreateBuyoutInvoice(context.Background(), volunteer.BuyoutInvoiceRequest{
		OrgID: "org-1", HouseholdID: "hh-1", Description: "x", AmountCents: 0, DueDate: "2026-12-01", Issue: true, Principal: p,
	})
	if err == nil {
		t.Fatal("expected issue failure")
	}
	for _, inv := range svc.SnapshotInvoices("org-1") {
		if inv.Status == "draft" {
			t.Fatalf("orphan draft invoice: %+v", inv)
		}
	}
}
