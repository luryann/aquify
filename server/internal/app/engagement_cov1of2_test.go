package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/events"
	"github.com/luryann/aquify/server/internal/app/training"
	"github.com/luryann/aquify/server/internal/testutil"
)

const cov1xOrg = "org-cov1x-engagement"

func cov1xCharge() events.Charge {
	return events.Charge{
		EventID: "abcdef12-0000-0000-0000-000000000001", HouseholdID: "hh-cov1x", SwimmerID: "sw-cov1x",
		Description: "Pizza night - RSVP fee", DueDate: "2027-01-15", Quantity: 3, UnitAmountCents: 1200, ActorUserID: "parent-cov1x",
	}
}

func cov1xChargeRef(c events.Charge) string {
	return fmt.Sprintf("club_event:%s:%s:%s:q%d", c.EventID, c.HouseholdID, c.SwimmerID, c.Quantity)
}

func TestCov1xTrainingPracticeSourceAdaptsTrainingService(t *testing.T) {
	t.Parallel()
	svc := training.NewService()
	src := trainingPracticeSource{training: svc}

	if got, err := src.Practices(cov1xOrg); err != nil || len(got) != 0 {
		t.Fatalf("empty org: %v %v", got, err)
	}
	team, err := svc.Create(cov1xOrg, "2027-02-01", "", "water", "Team practice", 0, nil, training.PracticeStrokes{}, "", "coach-1")
	if err != nil {
		t.Fatal(err)
	}
	grp, err := svc.Create(cov1xOrg, "2027-02-02", "grp-gold", "dryland", "Gold dryland", 0, nil, training.PracticeStrokes{}, "", "coach-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create("org-other", "2027-02-03", "", "water", "Elsewhere", 0, nil, training.PracticeStrokes{}, "", "coach-1"); err != nil {
		t.Fatal(err)
	}
	for sw, status := range map[string]string{"sw-1": "present", "sw-2": "absent"} {
		if _, err := svc.SetAttendance(cov1xOrg, team.ID, sw, status, "coach-1"); err != nil {
			t.Fatal(err)
		}
	}

	practices, err := src.Practices(cov1xOrg)
	if err != nil || len(practices) != 2 {
		t.Fatalf("practices: %v %v", practices, err)
	}
	byID := map[string]string{}
	for _, p := range practices {
		byID[p.ID] = p.Date + "|" + p.GroupID + "|" + p.Type
	}
	if byID[team.ID] != "2027-02-01||water" || byID[grp.ID] != "2027-02-02|grp-gold|dryland" {
		t.Fatalf("fields not carried over: %v", byID)
	}

	marks, err := src.AttendanceForSwimmer(cov1xOrg, "sw-1", []string{team.ID, grp.ID})
	if err != nil || len(marks) != 1 || marks[team.ID] != "present" {
		t.Fatalf("AttendanceForSwimmer: %v %v", marks, err)
	}
	records, err := src.Attendance(cov1xOrg, team.ID)
	if err != nil || len(records) != 2 {
		t.Fatalf("Attendance: %v %v", records, err)
	}
	got := map[string]string{}
	for _, r := range records {
		got[r.SwimmerID] = r.Status
	}
	if got["sw-1"] != "present" || got["sw-2"] != "absent" {
		t.Fatalf("records: %v", got)
	}
}

func TestCov1xChargeRSVPSkipsVoidedInvoicesAndIssuesAReplacement(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	biller := rsvpFeeBiller{billing: svc}
	sys := engagementSystemPrincipal(cov1xOrg)
	c := cov1xCharge()

	first, err := biller.ChargeRSVP(cov1xOrg, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.VoidInvoice(cov1xOrg, "admin-1", sys, first); err != nil {
		t.Fatal(err)
	}
	second, err := biller.ChargeRSVP(cov1xOrg, c)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("a voided invoice must not be reused")
	}
	invs, _ := svc.InvoicesByExternalRef(cov1xOrg, sys, cov1xChargeRef(c))
	if len(invs) != 2 || invs[0].Status != "voided" || invs[1].Status != "issued" || invs[1].TotalCents != 3600 {
		t.Fatalf("expected a voided invoice and an issued replacement, got %+v", invs)
	}
	if !strings.HasSuffix(invs[1].InvoiceNumber, "-2") {
		t.Fatalf("replacement should take the next attempt number, got %q", invs[1].InvoiceNumber)
	}
}

func TestCov1xChargeRSVPFailureModes(t *testing.T) {
	t.Parallel()
	sys := engagementSystemPrincipal(cov1xOrg)

	t.Run("invalid due date is reported and nothing is created", func(t *testing.T) {
		t.Parallel()
		svc := billing.NewService()
		c := cov1xCharge()
		c.DueDate = "next-friday"
		_, err := rsvpFeeBiller{billing: svc}.ChargeRSVP(cov1xOrg, c)
		if !errors.Is(err, billing.ErrInvalidFinanceInput) || !strings.Contains(err.Error(), "create rsvp invoice") {
			t.Fatalf("got %v", err)
		}
		if n := len(svc.SnapshotInvoices(cov1xOrg)); n != 0 {
			t.Fatalf("%d invoices created", n)
		}
	})

	t.Run("a number collision that never clears gives up", func(t *testing.T) {
		t.Parallel()
		svc := billing.NewService()
		c := cov1xCharge()
		// Another writer owns the invoice number this charge would take, under a
		// different reference, so the lookup never finds it and creation keeps colliding.
		sum := sha256.Sum256([]byte(cov1xChargeRef(c)))
		number := fmt.Sprintf("EVT-%s-%s-1", strings.ToUpper(c.EventID[:8]), strings.ToUpper(hex.EncodeToString(sum[:4])))
		if _, err := svc.CreateInvoice(cov1xOrg, "other", sys, billing.CreateInvoiceInput{HouseholdID: "hh-x", InvoiceNumber: number, ExternalRef: "someone-else"}); err != nil {
			t.Fatal(err)
		}
		_, err := rsvpFeeBiller{billing: svc}.ChargeRSVP(cov1xOrg, c)
		if err == nil || !strings.Contains(err.Error(), "concurrent creation did not settle") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("an unissuable fee voids the draft it built", func(t *testing.T) {
		t.Parallel()
		svc := billing.NewService()
		c := cov1xCharge()
		c.UnitAmountCents = 0 // a zero-total invoice cannot be issued
		_, err := rsvpFeeBiller{billing: svc}.ChargeRSVP(cov1xOrg, c)
		if !errors.Is(err, billing.ErrInvoiceEmpty) || !strings.Contains(err.Error(), "issue rsvp invoice") {
			t.Fatalf("got %v", err)
		}
		invs, _ := svc.InvoicesByExternalRef(cov1xOrg, sys, cov1xChargeRef(c))
		if len(invs) != 1 || invs[0].Status != "voided" {
			t.Fatalf("the failed draft must be voided: %+v", invs)
		}
	})

	t.Run("an oversized line voids the draft", func(t *testing.T) {
		t.Parallel()
		svc := billing.NewService()
		c := cov1xCharge()
		c.Quantity = 20000
		_, err := rsvpFeeBiller{billing: svc}.ChargeRSVP(cov1xOrg, c)
		if !errors.Is(err, billing.ErrInvalidFinanceInput) || !strings.Contains(err.Error(), "add rsvp invoice item") {
			t.Fatalf("got %v", err)
		}
		invs, _ := svc.InvoicesByExternalRef(cov1xOrg, sys, cov1xChargeRef(c))
		if len(invs) != 1 || invs[0].Status != "voided" {
			t.Fatalf("the failed draft must be voided: %+v", invs)
		}
	})
}

func TestCov1xChargeRSVPDatabaseFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	hh := uuid.NewString()
	sw := uuid.NewString()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'RSVP Family')`, []any{hh, org}},
		{`INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender) VALUES ($1, $2, $3, 'R', 'S', '2013-01-01', 'female')`, []any{sw, org, hh}},
	} {
		if _, err := pool.Exec(t.Context(), q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	c := cov1xCharge()
	c.EventID, c.HouseholdID, c.SwimmerID = uuid.NewString(), hh, sw

	// Lookup failure: the billing database is unreachable.
	closed := cov1xClosedCopy(t, pool)
	_, err := rsvpFeeBiller{billing: billing.NewServiceWithPool(closed)}.ChargeRSVP(org, c)
	if err == nil || !strings.Contains(err.Error(), "lookup rsvp invoice") {
		t.Fatalf("lookup failure: %v", err)
	}

	// Once the draft exists every update to it fails, so adding the fee line fails
	// and so does the compensating void: both failures are reported, never hidden.
	for _, q := range []string{
		`CREATE FUNCTION cov1x_block_invoice_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'invoice is frozen'; END $$`,
		`CREATE TRIGGER cov1x_freeze BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION cov1x_block_invoice_update()`,
	} {
		if _, err := pool.Exec(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	svc := billing.NewServiceWithPool(pool)
	_, err = rsvpFeeBiller{billing: svc}.ChargeRSVP(org, c)
	if err == nil || !strings.Contains(err.Error(), "add rsvp invoice item") || !strings.Contains(err.Error(), "void rsvp invoice") {
		t.Fatalf("expected both the add-item and the void failure, got %v", err)
	}
}

func TestCov1xReleaseCharge(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	biller := rsvpFeeBiller{billing: svc}
	sys := engagementSystemPrincipal(cov1xOrg)

	if _, err := biller.ReleaseCharge(cov1xOrg, "missing-invoice", "parent-1"); !errors.Is(err, billing.ErrInvoiceNotFound) {
		t.Fatalf("unknown invoice: %v", err)
	}

	// Unpaid and issued: released by voiding.
	unpaid, err := biller.ChargeRSVP(cov1xOrg, cov1xCharge())
	if err != nil {
		t.Fatal(err)
	}
	released, err := biller.ReleaseCharge(cov1xOrg, unpaid, "parent-1")
	if err != nil || !released {
		t.Fatalf("unpaid release: %v %v", released, err)
	}
	if inv, _ := svc.GetInvoice(cov1xOrg, sys, unpaid); inv.Status != "voided" {
		t.Fatalf("invoice should be voided, is %s", inv.Status)
	}
	// Releasing an already-void invoice is an idempotent success.
	if released, err := biller.ReleaseCharge(cov1xOrg, unpaid, "parent-1"); err != nil || !released {
		t.Fatalf("second release: %v %v", released, err)
	}

	// A draft that never got issued is voided too.
	draft, err := svc.CreateInvoice(cov1xOrg, "a", sys, billing.CreateInvoiceInput{HouseholdID: "hh-d", InvoiceNumber: "EVT-DRAFT"})
	if err != nil {
		t.Fatal(err)
	}
	if released, err := biller.ReleaseCharge(cov1xOrg, draft.ID, "parent-1"); err != nil || !released {
		t.Fatalf("draft release: %v %v", released, err)
	}

	// Money already collected keeps the ledger authoritative: nothing is voided.
	paidCharge := cov1xCharge()
	paidCharge.SwimmerID = "sw-paid"
	paidID, err := biller.ChargeRSVP(cov1xOrg, paidCharge)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RecordPayment(cov1xOrg, "admin-1", sys, paidID, billing.RecordPaymentInput{AmountCents: 1000, Method: "cash"}); err != nil {
		t.Fatal(err)
	}
	if released, err := biller.ReleaseCharge(cov1xOrg, paidID, "parent-1"); err != nil || released {
		t.Fatalf("partially paid invoice must be kept: released=%v err=%v", released, err)
	}
	if _, _, err := svc.RecordPayment(cov1xOrg, "admin-1", sys, paidID, billing.RecordPaymentInput{AmountCents: 2600, Method: "cash"}); err != nil {
		t.Fatal(err)
	}
	if released, err := biller.ReleaseCharge(cov1xOrg, paidID, "parent-1"); err != nil || released {
		t.Fatalf("fully paid invoice must be kept: released=%v err=%v", released, err)
	}
	if inv, _ := svc.GetInvoice(cov1xOrg, sys, paidID); inv.Status != "paid" {
		t.Fatalf("paid invoice status changed to %s", inv.Status)
	}
}
