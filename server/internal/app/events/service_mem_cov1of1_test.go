package events

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type cov1xBiller struct {
	charges    []Charge
	releaseErr error
	chargeErr  error
	retain     bool
}

func (b *cov1xBiller) ChargeRSVP(orgID string, c Charge) (string, error) {
	if b.chargeErr != nil {
		return "", b.chargeErr
	}
	b.charges = append(b.charges, c)
	return "cov1x-inv", nil
}

func (b *cov1xBiller) ReleaseCharge(orgID, invoiceID, actor string) (bool, error) {
	if b.releaseErr != nil {
		return false, b.releaseErr
	}
	return !b.retain, nil
}

func TestCov1xNormalizeValidationMessages(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }
	neg := -1
	cases := []struct {
		name   string
		mutate func(*EventInput)
		field  string
	}{
		{"long title", func(in *EventInput) { in.Title = long(maxTitleLen + 1) }, "title"},
		{"long description", func(in *EventInput) { in.Description = long(maxDescriptionLen + 1) }, "description"},
		{"long location", func(in *EventInput) { in.Location = long(maxLocationLen + 1) }, "location"},
		{"zero start", func(in *EventInput) { in.StartsAt = time.Time{} }, "starts_at"},
		{"bad audience", func(in *EventInput) { in.Audience = "everyone" }, "audience"},
		{"bad status", func(in *EventInput) { in.Status = "postponed" }, "status"},
		{"negative fee", func(in *EventInput) { in.FeeCents = -5 }, "fee_cents"},
		{"huge fee", func(in *EventInput) { in.FeeCents = maxFeeCents + 1 }, "fee_cents"},
		{"negative capacity", func(in *EventInput) { in.Capacity = &neg }, "capacity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mutate(&in)
			_, err := normalize(in)
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("expected ValidationError on %s, got %v", tc.field, err)
			}
			if !errors.Is(err, ErrInvalidEvent) || !strings.HasPrefix(err.Error(), tc.field+": ") {
				t.Fatalf("unexpected error text %q", err.Error())
			}
		})
	}
}

func TestCov1xNormalizeCanonicalises(t *testing.T) {
	local := time.FixedZone("x", -7*3600)
	start := time.Date(2026, 12, 1, 9, 0, 0, 0, local)
	end := start.Add(time.Hour)
	dl := start.Add(-time.Hour)
	got, err := normalize(EventInput{
		Title: "  Dryland  ", Category: " DRYLAND ", StartsAt: start, EndsAt: &end,
		RSVPEnabled: true, RSVPDeadline: &dl, Capacity: intPtr(10),
		Audience: "ALL", GroupCodes: []string{"x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Dryland" || got.Category != "dryland" || got.Status != "scheduled" || got.RSVPMode != "household" {
		t.Fatalf("defaults not applied: %+v", got)
	}
	if got.StartsAt.Location() != time.UTC || got.EndsAt.Location() != time.UTC || got.RSVPDeadline.Location() != time.UTC {
		t.Fatal("times must be normalised to UTC")
	}
	if len(got.GroupCodes) != 0 {
		t.Fatalf("audience all must clear group codes, got %v", got.GroupCodes)
	}
	// RSVP disabled clears deadline and capacity.
	got, err = normalize(EventInput{Title: "t", Category: "social", StartsAt: start, RSVPDeadline: &dl, Capacity: intPtr(3)})
	if err != nil || got.RSVPDeadline != nil || got.Capacity != nil {
		t.Fatalf("expected cleared rsvp settings, got %+v %v", got, err)
	}
}

func TestCov1xClockAndLogger(t *testing.T) {
	s := NewService()
	s.SetClock(func() time.Time { return baseNow })
	if !s.Now().Equal(baseNow) {
		t.Fatalf("Now() = %v", s.Now())
	}
	var buf bytes.Buffer
	s.SetLogger(nil) // nil keeps the existing logger
	s.SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	s.logger.Warn("hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Fatal("custom logger not installed")
	}
	if NewServiceWithPool(nil).repo != nil {
		t.Fatal("nil pool must stay in-memory")
	}
	if real := NewService(); real.Now().Location() != time.UTC {
		t.Fatal("default clock should be UTC")
	}
}

func TestCov1xRSVPOpenBranches(t *testing.T) {
	var nilEvent *Event
	if nilEvent.RSVPOpen(baseNow) {
		t.Fatal("nil event is never open")
	}
	start := baseNow.Add(48 * time.Hour)
	dl := baseNow.Add(time.Hour)
	e := &Event{RSVPEnabled: true, Status: "scheduled", StartsAt: start, RSVPDeadline: &dl}
	if !e.RSVPOpen(baseNow) || e.RSVPOpen(dl.Add(time.Second)) {
		t.Fatal("deadline must close RSVPs")
	}
	disabled := &Event{RSVPEnabled: false, Status: "scheduled", StartsAt: start}
	cancelled := &Event{RSVPEnabled: true, Status: "cancelled", StartsAt: start}
	if disabled.RSVPOpen(baseNow) || cancelled.RSVPOpen(baseNow) {
		t.Fatal("disabled/cancelled events are closed")
	}
	end := start.Add(time.Hour)
	e = &Event{RSVPEnabled: true, Status: "scheduled", StartsAt: start, EndsAt: &end}
	if !e.RSVPOpen(start.Add(30*time.Minute)) || e.RSVPOpen(end.Add(time.Second)) {
		t.Fatal("ends_at bounds RSVPs")
	}
}

func TestCov1xSummarizeIgnoresUnknownAndNilEvent(t *testing.T) {
	sum := Summarize(nil, []*RSVP{{Response: "yes", GuestCount: 1}, {Response: "bogus"}, {Response: "no"}})
	if sum.Yes != 1 || sum.YesHeadcount != 2 || sum.No != 1 || sum.Maybe != 0 || sum.SpotsLeft != nil {
		t.Fatalf("unexpected %+v", sum)
	}
	over := Summarize(&Event{Capacity: intPtr(1)}, []*RSVP{{Response: "yes", GuestCount: 3}})
	if over.SpotsLeft == nil || *over.SpotsLeft != 0 {
		t.Fatalf("spots left must clamp at zero: %+v", over)
	}
}

func TestCov1xEventEnd(t *testing.T) {
	start := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	if got := eventEnd(&Event{StartsAt: start}); !got.Equal(start) {
		t.Fatalf("no end: %v", got)
	}
	end := start.Add(3 * time.Hour)
	if got := eventEnd(&Event{StartsAt: start, EndsAt: &end}); !got.Equal(end) {
		t.Fatalf("explicit end: %v", got)
	}
	got := eventEnd(&Event{StartsAt: start, AllDay: true})
	if got.Day() != 5 || got.Hour() != 23 || got.Minute() != 59 {
		t.Fatalf("all-day end: %v", got)
	}
}

func TestCov1xListToFilterAndTieBreak(t *testing.T) {
	s := newTestService()
	in := validInput()
	in.EndsAt = nil
	in.StartsAt = baseNow.Add(24 * time.Hour)
	a, _ := s.Create("org", "u", in)
	b, _ := s.Create("org", "u", in)
	in.StartsAt = baseNow.Add(72 * time.Hour)
	_, _ = s.Create("org", "u", in)
	to := baseNow.Add(48 * time.Hour)
	list, err := s.List("org", ListFilter{To: &to})
	if err != nil || len(list) != 2 {
		t.Fatalf("to filter: %v %v", list, err)
	}
	if list[0].ID >= list[1].ID || (list[0].ID != a.ID && list[0].ID != b.ID) {
		t.Fatalf("equal start times must order by ID: %v %v", list[0].ID, list[1].ID)
	}
}

func TestCov1xUpdateDeleteNotFound(t *testing.T) {
	s := newTestService()
	if _, err := s.Update("org", "missing", "u", validInput()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update: %v", err)
	}
	e, _ := s.Create("org", "u", validInput())
	if _, err := s.Update("other", e.ID, "u", validInput()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org update: %v", err)
	}
	bad := validInput()
	bad.Title = ""
	if _, err := s.Update("org", e.ID, "u", bad); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("invalid update: %v", err)
	}
	if err := s.Delete("org", "missing", "u"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete: %v", err)
	}
	if err := s.Delete("other", e.ID, "u"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org delete: %v", err)
	}
	if _, err := s.Get("org", e.ID); err != nil {
		t.Fatalf("event must survive failed deletes: %v", err)
	}
}

func TestCov1xListRSVPsForEventsMemory(t *testing.T) {
	s := newTestService()
	got, err := s.ListRSVPsForEvents("org", nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty ids: %v %v", got, err)
	}
	e1, _ := s.Create("org", "u", validInput())
	e2, _ := s.Create("org", "u", validInput())
	e3, _ := s.Create("org", "u", validInput())
	for _, hh := range []string{"h2", "h1"} {
		if _, err := s.Respond("org", e1.ID, "p", RSVPInput{HouseholdID: hh, Response: "yes"}, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Respond("org", e2.ID, "p", RSVPInput{HouseholdID: "h1", Response: "no"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Respond("org", e3.ID, "p", RSVPInput{HouseholdID: "h1", Response: "no"}, true); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListRSVPsForEvents("org", []string{e1.ID, e2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got[e1.ID]) != 2 || len(got[e2.ID]) != 1 || got[e3.ID] != nil {
		t.Fatalf("unexpected grouping: %v", got)
	}
	if got[e1.ID][0].HouseholdID != "h1" || got[e1.ID][1].HouseholdID != "h2" {
		t.Fatal("rsvps must be sorted by household")
	}
	if other, _ := s.ListRSVPsForEvents("other", []string{e1.ID}); len(other) != 0 {
		t.Fatal("cross-org rsvps leaked")
	}
}

func TestCov1xRespondValidation(t *testing.T) {
	s := newTestService()
	e, _ := s.Create("org", "u", validInput())
	if _, err := s.Respond("org", "missing", "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing event: %v", err)
	}
	cases := map[string]RSVPInput{
		"household_id": {Response: "yes"},
		"response":     {HouseholdID: "h", Response: "perhaps"},
		"guest_count":  {HouseholdID: "h", Response: "yes", GuestCount: maxGuestCount + 1},
		"note":         {HouseholdID: "h", Response: "yes", Note: strings.Repeat("n", maxNoteLen+1)},
	}
	for field, in := range cases {
		_, err := s.Respond("org", e.ID, "p", in, true)
		if !errors.Is(err, ErrInvalidRSVP) || !strings.Contains(err.Error(), field+":") {
			t.Errorf("%s: got %v", field, err)
		}
	}
	in := RSVPInput{HouseholdID: "h", Response: "yes", GuestCount: -1}
	if _, err := s.Respond("org", e.ID, "p", in, true); !errors.Is(err, ErrInvalidRSVP) {
		t.Fatalf("negative guests: %v", err)
	}
	// Household-mode events drop any swimmer id.
	res, err := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h", SwimmerID: "s1", Response: "YES", Note: "  hi  "}, true)
	if err != nil || res.RSVP.SwimmerID != "" || res.RSVP.Note != "hi" || res.RSVP.Response != "yes" {
		t.Fatalf("unexpected %+v %v", res, err)
	}
	if res.Billing != BillingNone {
		t.Fatalf("no biller => none, got %s", res.Billing)
	}
}

func TestCov1xReconcileFeeBranches(t *testing.T) {
	newEnv := func(fee int) (*Service, *cov1xBiller, *Event) {
		s := newTestService()
		b := &cov1xBiller{}
		s.SetBiller(b)
		in := validInput()
		in.FeeCents = fee
		e, err := s.Create("org", "admin", in)
		if err != nil {
			t.Fatal(err)
		}
		return s, b, e
	}

	t.Run("free event never bills", func(t *testing.T) {
		s, b, e := newEnv(0)
		res, err := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true)
		if err != nil || res.Billing != BillingNone || len(b.charges) != 0 {
			t.Fatalf("%+v %v", res, err)
		}
	})
	t.Run("maybe on paid event is not charged", func(t *testing.T) {
		s, b, e := newEnv(500)
		res, _ := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h", Response: "maybe"}, true)
		if res.Billing != BillingNone || len(b.charges) != 0 {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("release error surfaces as failed and keeps invoice", func(t *testing.T) {
		s, b, e := newEnv(500)
		if res, _ := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true); res.Billing != BillingCharged {
			t.Fatalf("setup: %+v", res)
		}
		b.releaseErr = errors.New("ledger down")
		res, err := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h", Response: "no"}, true)
		if err != nil || res.Billing != BillingFailed || res.RSVP.InvoiceID != "cov1x-inv" {
			t.Fatalf("%+v %v", res, err)
		}
	})
	t.Run("paid invoice retained", func(t *testing.T) {
		s, b, e := newEnv(500)
		_, _ = s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true)
		b.retain = true
		res, _ := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h", Response: "no"}, true)
		if res.Billing != BillingRetainedPaid {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("charge failure logged and reported", func(t *testing.T) {
		s, b, e := newEnv(500)
		b.chargeErr = errors.New("nope")
		var buf bytes.Buffer
		s.SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))
		res, _ := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true)
		if res.Billing != BillingFailed || res.RSVP.InvoiceID != "" || !strings.Contains(buf.String(), "charge rsvp fee failed") {
			t.Fatalf("%+v log=%q", res, buf.String())
		}
	})
	t.Run("force rebill with release then charge", func(t *testing.T) {
		s, b, e := newEnv(500)
		res, _ := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true)
		if got := s.reconcileFee(e, res.RSVP, "admin", true); got != BillingCharged || len(b.charges) != 2 {
			t.Fatalf("got %s charges=%d", got, len(b.charges))
		}
	})
	t.Run("force with zero quantity and no invoice is none", func(t *testing.T) {
		s, _, e := newEnv(500)
		r := &RSVP{Response: "no", HouseholdID: "h"}
		// invoice id empty, fee>0, wantQty 0: nothing to bill or release.
		if got := s.reconcileFee(e, r, "a", true); got != BillingNone {
			t.Fatalf("got %s", got)
		}
	})
}

func TestCov1xReleaseAllLogsFailuresAndKeepsOthers(t *testing.T) {
	s := newTestService()
	b := &cov1xBiller{}
	s.SetBiller(b)
	in := validInput()
	in.FeeCents = 100
	e, _ := s.Create("org", "admin", in)
	for _, hh := range []string{"h1", "h2"} {
		if _, err := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: hh, Response: "yes"}, true); err != nil {
			t.Fatal(err)
		}
	}
	// A no-invoice rsvp is skipped.
	if _, err := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h3", Response: "no"}, true); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	s.SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	b.releaseErr = errors.New("boom")
	in.Status = "cancelled"
	if _, err := s.Update("org", e.ID, "admin", in); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "release rsvp invoice failed") != 2 {
		t.Fatalf("expected two warnings, got %q", buf.String())
	}
	rs, _ := s.ListRSVPs("org", e.ID)
	for _, r := range rs {
		if r.Response == "yes" && r.InvoiceID == "" {
			t.Fatal("failed release must keep the invoice link")
		}
	}
	// No biller => releaseAll is a no-op.
	s.SetBiller(nil)
	s.releaseAll("org", "a", rs)
}

func TestCov1xEraseHouseholdDataBlankAndTokens(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	if n, err := s.EraseHouseholdData(ctx, " ", "h"); n != 0 || err != nil {
		t.Fatal("blank org must be a no-op")
	}
	if n, err := s.EraseHouseholdData(ctx, "org", ""); n != 0 || err != nil {
		t.Fatal("blank household must be a no-op")
	}
	secret, _, _ := s.RotateFeedToken("org", "h1", "p")
	otherSecret, _, _ := s.RotateFeedToken("org", "h2", "p")
	if _, err := s.EraseHouseholdData(ctx, "org", "h1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveFeedToken(secret); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatal("erased household token must not resolve")
	}
	if _, err := s.ResolveFeedToken(otherSecret); err != nil {
		t.Fatalf("other household token must survive: %v", err)
	}
	if n, _ := s.EraseHouseholdData(ctx, "org", "h1"); n != 0 {
		t.Fatal("erase must be idempotent")
	}
}

func TestCov1xFeedTokenMemoryEdges(t *testing.T) {
	s := newTestService()
	if tok, err := s.ActiveFeedToken("org", "h1"); tok != nil || err != nil {
		t.Fatalf("no token yet: %v %v", tok, err)
	}
	unknown := strings.Repeat("a", 43)
	if _, err := s.ResolveFeedToken(unknown); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatalf("unknown token: %v", err)
	}
	secret, tok, _ := s.RotateFeedToken("org", "h1", "p")
	// Simulate a revoked token still present in the hash index.
	s.mu.Lock()
	now := baseNow
	s.tokens[tok.ID].RevokedAt = &now
	s.mu.Unlock()
	if _, err := s.ResolveFeedToken(secret); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatalf("revoked: %v", err)
	}
	// Dangling index entry (token record gone) also resolves to missing.
	s.mu.Lock()
	delete(s.tokens, tok.ID)
	s.mu.Unlock()
	if _, err := s.ResolveFeedToken(secret); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatalf("dangling: %v", err)
	}
	if got := HashFeedToken("  " + secret + " "); got != HashFeedToken(secret) {
		t.Fatal("hash must ignore surrounding whitespace")
	}
	sec, err := newFeedSecret()
	if err != nil || len(sec) != 43 {
		t.Fatalf("secret %q %v", sec, err)
	}
}

func TestCov1xRenderICSBranches(t *testing.T) {
	end := baseNow.Add(-time.Hour) // before start: falls back to +1h
	out := RenderICS("Club", "", []CalendarItem{
		{UID: "t@x", Title: "Timed", Start: baseNow, End: &end, Categories: "social", Updated: baseNow.Add(time.Minute)},
		{UID: "a@x", Title: "Bad range", AllDay: true, StartDate: "2026-10-10", EndDate: "2026-10-01"},
		{UID: "b@x", Title: "Bad date", AllDay: true, StartDate: "garbage"},
		{UID: "c@x", Title: "No end", AllDay: true, StartDate: "2026-12-31"},
	}, baseNow)
	for _, want := range []string{
		"DTEND:20261001T130000Z\r\n", "CATEGORIES:social\r\n", "LAST-MODIFIED:20261001T120100Z\r\n",
		"STATUS:CONFIRMED\r\n", "DTSTART;VALUE=DATE:20261010\r\n", "DTEND;VALUE=DATE:20261011\r\n",
		"DTSTART;VALUE=DATE:garbage\r\n", "DTEND;VALUE=DATE:garbage\r\n", "DTEND;VALUE=DATE:20270101\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "X-WR-TIMEZONE") {
		t.Fatal("empty timezone must not be advertised")
	}
}

func TestCov1xFoldLineKeepsMultibyteRunes(t *testing.T) {
	line := "SUMMARY:" + strings.Repeat("é", 60)
	folded := foldLine(line)
	for _, part := range strings.Split(folded, "\r\n") {
		if len(part) > 75 {
			t.Fatalf("part too long: %d", len(part))
		}
	}
	if strings.ReplaceAll(folded, "\r\n ", "") != line {
		t.Fatal("unfolding must reproduce the original line")
	}
}

func TestCov1xSortRSVPsBySwimmerWithinHousehold(t *testing.T) {
	s := newTestService()
	in := validInput()
	in.RSVPMode = "swimmer"
	e, _ := s.Create("org", "u", in)
	for _, sw := range []string{"s2", "s3", "s1"} {
		if _, err := s.Respond("org", e.ID, "p", RSVPInput{HouseholdID: "h", SwimmerID: sw, Response: "yes"}, true); err != nil {
			t.Fatal(err)
		}
	}
	rs, _ := s.ListRSVPs("org", e.ID)
	var got []string
	for _, r := range rs {
		got = append(got, r.SwimmerID)
	}
	if strings.Join(got, ",") != "s1,s2,s3" {
		t.Fatalf("expected swimmer order within household, got %v", got)
	}
}
