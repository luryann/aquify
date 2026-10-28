package events

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeBiller struct {
	charges  []Charge
	released []string
	paid     map[string]bool
	failNext bool
}

func (f *fakeBiller) ChargeRSVP(orgID string, c Charge) (string, error) {
	if f.failNext {
		f.failNext = false
		return "", errors.New("billing down")
	}
	f.charges = append(f.charges, c)
	return fmt.Sprintf("inv-%d", len(f.charges)), nil
}

func (f *fakeBiller) ReleaseCharge(orgID, invoiceID, actor string) (bool, error) {
	if f.paid[invoiceID] {
		return false, nil
	}
	f.released = append(f.released, invoiceID)
	return true, nil
}

var baseNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func newTestService() *Service {
	s := NewService()
	s.SetClock(func() time.Time { return baseNow })
	return s
}

func intPtr(v int) *int { return &v }

func validInput() EventInput {
	start := baseNow.Add(10 * 24 * time.Hour)
	end := start.Add(2 * time.Hour)
	return EventInput{
		Title:       "Team Banquet",
		Category:    "banquet",
		StartsAt:    start,
		EndsAt:      &end,
		RSVPEnabled: true,
		RSVPMode:    "household",
	}
}

func TestCreateValidation(t *testing.T) {
	s := newTestService()
	cases := map[string]func(*EventInput){
		"title":       func(in *EventInput) { in.Title = " " },
		"category":    func(in *EventInput) { in.Category = "party" },
		"ends_before": func(in *EventInput) { e := in.StartsAt.Add(-time.Hour); in.EndsAt = &e },
		"groups":      func(in *EventInput) { in.Audience = "groups" },
		"fee_no_rsvp": func(in *EventInput) { in.RSVPEnabled = false; in.FeeCents = 500 },
		"capacity":    func(in *EventInput) { in.Capacity = intPtr(0) },
		"deadline":    func(in *EventInput) { d := in.StartsAt.Add(time.Hour); in.RSVPDeadline = &d },
		"mode":        func(in *EventInput) { in.RSVPMode = "team" },
	}
	for name, mutate := range cases {
		in := validInput()
		mutate(&in)
		if _, err := s.Create("org", "u1", in); !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("%s: expected ErrInvalidEvent, got %v", name, err)
		}
	}
	in := validInput()
	in.Audience = "groups"
	in.GroupCodes = []string{" senior ", "SENIOR", "age"}
	e, err := s.Create("org", "u1", in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if strings.Join(e.GroupCodes, ",") != "AGE,SENIOR" {
		t.Fatalf("group codes not canonicalised: %v", e.GroupCodes)
	}
	if _, err := s.Get("other-org", e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected cross-org get to be not found, got %v", err)
	}
}

func TestRSVPCapacityDeadlineAndModeLock(t *testing.T) {
	s := newTestService()
	in := validInput()
	in.Capacity = intPtr(5)
	deadline := baseNow.Add(24 * time.Hour)
	in.RSVPDeadline = &deadline
	e, err := s.Create("org", "admin", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes", GuestCount: 2}, true); err != nil {
		t.Fatalf("first rsvp: %v", err)
	}
	// 3 taken; a party of 3 would exceed capacity 5.
	if _, err := s.Respond("org", e.ID, "p2", RSVPInput{HouseholdID: "h2", Response: "yes", GuestCount: 2}, true); !errors.Is(err, ErrEventFull) {
		t.Fatalf("expected ErrEventFull, got %v", err)
	}
	// Maybe does not consume capacity.
	if _, err := s.Respond("org", e.ID, "p2", RSVPInput{HouseholdID: "h2", Response: "maybe", GuestCount: 4}, true); err != nil {
		t.Fatalf("maybe rsvp: %v", err)
	}
	// Updating own RSVP excludes itself from the headcount.
	if _, err := s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes", GuestCount: 4}, true); err != nil {
		t.Fatalf("resize own rsvp: %v", err)
	}
	rsvps, _ := s.ListRSVPs("org", e.ID)
	sum := Summarize(e, rsvps)
	if sum.Yes != 1 || sum.Maybe != 1 || sum.YesHeadcount != 5 || sum.SpotsLeft == nil || *sum.SpotsLeft != 0 {
		t.Fatalf("unexpected summary %+v", sum)
	}

	// Mode cannot change once RSVPs exist.
	in.RSVPMode = "swimmer"
	if _, err := s.Update("org", e.ID, "admin", in); !errors.Is(err, ErrRSVPModeLocked) {
		t.Fatalf("expected ErrRSVPModeLocked, got %v", err)
	}

	// After the deadline families are locked out but staff are not.
	s.SetClock(func() time.Time { return deadline.Add(time.Minute) })
	if _, err := s.Respond("org", e.ID, "p2", RSVPInput{HouseholdID: "h2", Response: "no"}, true); !errors.Is(err, ErrRSVPClosed) {
		t.Fatalf("expected ErrRSVPClosed, got %v", err)
	}
	if _, err := s.Respond("org", e.ID, "coach", RSVPInput{HouseholdID: "h2", Response: "no"}, false); err != nil {
		t.Fatalf("staff override after deadline: %v", err)
	}
}

func TestSwimmerModeRequiresSwimmer(t *testing.T) {
	s := newTestService()
	in := validInput()
	in.RSVPMode = "swimmer"
	e, _ := s.Create("org", "admin", in)
	if _, err := s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes"}, true); !errors.Is(err, ErrInvalidRSVP) {
		t.Fatalf("expected ErrInvalidRSVP, got %v", err)
	}
	a, err := s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", SwimmerID: "s1", Response: "yes"}, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", SwimmerID: "s2", Response: "no", GuestCount: 3}, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.RSVP.ID == b.RSVP.ID || b.RSVP.GuestCount != 0 {
		t.Fatalf("expected separate per-swimmer rows and guests zeroed on no: %+v %+v", a.RSVP, b.RSVP)
	}
}

func TestRSVPDisabledAndCancelled(t *testing.T) {
	s := newTestService()
	in := validInput()
	in.RSVPEnabled = false
	e, _ := s.Create("org", "admin", in)
	if _, err := s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes"}, true); !errors.Is(err, ErrRSVPDisabled) {
		t.Fatalf("expected ErrRSVPDisabled, got %v", err)
	}
	in = validInput()
	in.Status = "cancelled"
	c, _ := s.Create("org", "admin", in)
	if _, err := s.Respond("org", c.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes"}, false); !errors.Is(err, ErrEventCancelled) {
		t.Fatalf("expected ErrEventCancelled, got %v", err)
	}
}

func TestRSVPFeeBillingLifecycle(t *testing.T) {
	s := newTestService()
	biller := &fakeBiller{paid: map[string]bool{}}
	s.SetBiller(biller)
	in := validInput()
	in.FeeCents = 2500
	e, _ := s.Create("org", "admin", in)

	res, err := s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes", GuestCount: 1}, true)
	if err != nil || res.Billing != BillingCharged {
		t.Fatalf("expected charge, got %v %v", res, err)
	}
	if len(biller.charges) != 1 || biller.charges[0].Quantity != 2 || biller.charges[0].UnitAmountCents != 2500 || biller.charges[0].DueDate != e.StartsAt.Format("2006-01-02") {
		t.Fatalf("unexpected charge %+v", biller.charges)
	}
	// Same answer again: no double billing.
	res, _ = s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes", GuestCount: 1, Note: "veg"}, true)
	if res.Billing != BillingUnchanged || len(biller.charges) != 1 {
		t.Fatalf("expected unchanged billing, got %s with %d charges", res.Billing, len(biller.charges))
	}
	// Party size change re-bills: release then charge.
	res, _ = s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes", GuestCount: 3}, true)
	if res.Billing != BillingCharged || len(biller.released) != 1 || res.RSVP.InvoiceID != "inv-2" || res.RSVP.BilledQuantity != 4 {
		t.Fatalf("expected rebill, got %+v released=%v", res, biller.released)
	}
	// Paid invoice is retained when switching to no.
	biller.paid["inv-2"] = true
	res, _ = s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "no"}, true)
	if res.Billing != BillingRetainedPaid || res.RSVP.InvoiceID != "inv-2" {
		t.Fatalf("expected retained paid invoice, got %+v", res)
	}
	// Unpaid invoice is released on no.
	res, _ = s.Respond("org", e.ID, "p2", RSVPInput{HouseholdID: "h2", Response: "yes"}, true)
	if res.Billing != BillingCharged {
		t.Fatal("expected charge for h2")
	}
	res, _ = s.Respond("org", e.ID, "p2", RSVPInput{HouseholdID: "h2", Response: "maybe"}, true)
	if res.Billing != BillingReleased || res.RSVP.InvoiceID != "" {
		t.Fatalf("expected release, got %+v", res)
	}
	// Billing failure is surfaced, RSVP still stored.
	biller.failNext = true
	res, err = s.Respond("org", e.ID, "p3", RSVPInput{HouseholdID: "h3", Response: "yes"}, true)
	if err != nil || res.Billing != BillingFailed || res.RSVP.Response != "yes" {
		t.Fatalf("expected stored rsvp with failed billing, got %+v %v", res, err)
	}
}

func TestCancelReleasesUnpaidInvoices(t *testing.T) {
	s := newTestService()
	biller := &fakeBiller{paid: map[string]bool{}}
	s.SetBiller(biller)
	in := validInput()
	in.FeeCents = 1000
	e, _ := s.Create("org", "admin", in)
	_, _ = s.Respond("org", e.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes"}, true)
	_, _ = s.Respond("org", e.ID, "p2", RSVPInput{HouseholdID: "h2", Response: "yes"}, true)
	biller.paid["inv-2"] = true
	in.Status = "cancelled"
	if _, err := s.Update("org", e.ID, "admin", in); err != nil {
		t.Fatal(err)
	}
	if len(biller.released) != 1 || biller.released[0] != "inv-1" {
		t.Fatalf("expected only unpaid inv-1 released, got %v", biller.released)
	}
	if err := s.Delete("org", e.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if rsvps, _ := s.ListRSVPs("org", e.ID); len(rsvps) != 0 {
		t.Fatalf("expected rsvps removed with event, got %d", len(rsvps))
	}
}

func TestListFilterAndOrder(t *testing.T) {
	s := newTestService()
	a := validInput()
	a.Title = "Later"
	a.StartsAt = baseNow.Add(48 * time.Hour)
	a.EndsAt = nil
	b := validInput()
	b.Title = "Past"
	b.StartsAt = baseNow.Add(-48 * time.Hour)
	b.EndsAt = nil
	_, _ = s.Create("org", "u", a)
	_, _ = s.Create("org", "u", b)
	_, _ = s.Create("org2", "u", a)
	from := baseNow
	list, _ := s.List("org", ListFilter{From: &from})
	if len(list) != 1 || list[0].Title != "Later" {
		t.Fatalf("unexpected filtered list %+v", list)
	}
	all, _ := s.List("org", ListFilter{})
	if len(all) != 2 || all[0].Title != "Past" {
		t.Fatalf("expected chronological order, got %v", all)
	}
}

func TestFeedTokenLifecycle(t *testing.T) {
	s := newTestService()
	secret, tok, err := s.RotateFeedToken("org", "h1", "parent")
	if err != nil {
		t.Fatal(err)
	}
	if tok.TokenHash == secret || len(secret) < 40 {
		t.Fatalf("secret must not equal stored hash: %q", secret)
	}
	got, err := s.ResolveFeedToken(secret)
	if err != nil || got.HouseholdID != "h1" || got.LastUsedAt == nil {
		t.Fatalf("resolve: %+v %v", got, err)
	}
	secret2, _, _ := s.RotateFeedToken("org", "h1", "parent")
	if _, err := s.ResolveFeedToken(secret); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatal("rotated token should be revoked")
	}
	if active, _ := s.ActiveFeedToken("org", "h1"); active == nil {
		t.Fatal("expected active token")
	}
	if err := s.RevokeFeedToken("org", "h1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveFeedToken(secret2); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatal("revoked token should not resolve")
	}
	if err := s.RevokeFeedToken("org", "h1"); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatalf("expected missing on double revoke, got %v", err)
	}
	if _, err := s.ResolveFeedToken("short"); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatal("short token should not resolve")
	}
}

func TestRenderICS(t *testing.T) {
	end := baseNow.Add(90 * time.Minute)
	out := RenderICS("Club", []CalendarItem{
		{UID: "a@x", Title: "Meet; finals, day 1", AllDay: true, StartDate: "2026-10-10", EndDate: "2026-10-11"},
		{UID: "b@x", Title: "Banquet", Start: baseNow, End: &end, Location: "Hall\nRoom 2", Cancelled: true,
			Description: strings.Repeat("long description ", 10)},
	}, baseNow)
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n", "SUMMARY:Meet\\; finals\\, day 1\r\n",
		"DTSTART;VALUE=DATE:20261010\r\n", "DTEND;VALUE=DATE:20261012\r\n",
		"DTSTART:20261001T120000Z\r\n", "DTEND:20261001T133000Z\r\n",
		"LOCATION:Hall\\nRoom 2\r\n", "STATUS:CANCELLED\r\n", "END:VCALENDAR\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ics missing %q", want)
		}
	}
	for _, line := range strings.Split(out, "\r\n") {
		if len(line) > 75 {
			t.Errorf("line exceeds 75 octets: %q", line)
		}
	}
}
