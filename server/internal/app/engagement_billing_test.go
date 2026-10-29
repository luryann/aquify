package app

import (
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/events"
)

func rsvpCharge() events.Charge {
	return events.Charge{
		EventID: "11111111-2222-3333-4444-555555555555", HouseholdID: "hh-1", SwimmerID: "sw-1",
		Description: "Banquet - RSVP fee", DueDate: "2026-12-01", Quantity: 2, UnitAmountCents: 2500, ActorUserID: "parent-1",
	}
}

func TestChargeRSVPIsIdempotentAcrossRetries(t *testing.T) {
	svc := billing.NewService()
	biller := rsvpFeeBiller{billing: svc}
	first, err := biller.ChargeRSVP("org-1", rsvpCharge())
	if err != nil {
		t.Fatal(err)
	}
	again, err := biller.ChargeRSVP("org-1", rsvpCharge()) // retry after a lost response
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatalf("retry created a second invoice: %s vs %s", first, again)
	}
	invs, _ := svc.InvoicesByExternalRef("org-1", engagementSystemPrincipal("org-1"), "club_event:11111111-2222-3333-4444-555555555555:hh-1:sw-1:q2")
	if len(invs) != 1 || invs[0].Status != "issued" || invs[0].TotalCents != 5000 {
		t.Fatalf("expected one issued 5000c invoice, got %+v", invs)
	}
}

func TestChargeRSVPResumesOrphanDraft(t *testing.T) {
	svc := billing.NewService()
	sys := engagementSystemPrincipal("org-1")
	draft, err := svc.CreateInvoice("org-1", "parent-1", sys, billing.CreateInvoiceInput{
		HouseholdID: "hh-1", InvoiceNumber: "EVT-ORPHAN",
		ExternalRef: "club_event:11111111-2222-3333-4444-555555555555:hh-1:sw-1:q2",
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := rsvpFeeBiller{billing: svc}.ChargeRSVP("org-1", rsvpCharge())
	if err != nil {
		t.Fatal(err)
	}
	if id != draft.ID {
		t.Fatalf("orphan draft was not resumed: got %s want %s", id, draft.ID)
	}
	if got, _ := svc.GetInvoice("org-1", sys, id); got.Status != "issued" {
		t.Fatalf("draft not issued: %s", got.Status)
	}
}

func TestEngagementSystemPrincipalIsFixedMarker(t *testing.T) {
	p := engagementSystemPrincipal("org-1")
	if p.UserID != engagementSystemUserID || p.OrgID != "org-1" {
		t.Fatalf("unexpected system principal %+v", p)
	}
}
