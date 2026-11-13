package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/seasons"
	"github.com/luryann/aquify/server/internal/app/volunteer"
	"github.com/luryann/aquify/server/internal/postmark"
	"github.com/luryann/aquify/server/internal/testutil"
	"github.com/luryann/aquify/server/internal/workers"
)

func cov1xAdmin(org string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "cov1x-admin", OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
}

// TestCov1xWireVolunteerRemindersEndToEnd wires the real members, meets,
// seasons, billing, email and queue dependencies into the volunteer domain and
// follows a signup through to the delivered reminder email.
func TestCov1xWireVolunteerRemindersEndToEnd(t *testing.T) {
	const org = "org-cov1x-vol"
	admin := cov1xAdmin(org)
	ctx := context.Background()

	membersSvc := members.NewService()
	hh, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Reminder Family", PrimaryEmail: "family@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := membersSvc.CreateSwimmer(admin, org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "Ann", LastName: "Reminder", DateOfBirth: "2010-04-05", Gender: "female"})
	if err != nil {
		t.Fatal(err)
	}
	otherHH, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Other Family"})
	if err != nil {
		t.Fatal(err)
	}

	meetsSvc := meets.NewService()
	meet, _, err := meetsSvc.CreateMeet(admin, org, meets.CreateMeetInput{Name: "Fall Invitational", StartDate: "2027-03-01", EndDate: "2027-03-02"})
	if err != nil {
		t.Fatal(err)
	}
	seasonsSvc := seasons.NewService()
	season, err := seasonsSvc.Create(org, "2026-27", "2026-09-01", "2027-08-31", "active", "", "cov1x")
	if err != nil {
		t.Fatal(err)
	}

	mail := postmark.NewStubClient()
	queue := workers.NewServer(workers.ServerConfig{})
	h, svc := wireVolunteer(volunteerDeps{
		logger: nil, members: membersSvc, meets: meetsSvc, seasons: seasonsSvc,
		billing: billing.NewService(), postmark: mail, workerBack: queue,
	})
	if h == nil || svc == nil {
		t.Fatal("wireVolunteer must return handlers and service")
	}
	now := time.Date(2027, 2, 20, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })

	start := now.Add(72 * time.Hour)
	job, err := svc.CreateJob(ctx, admin, org, volunteer.JobInput{
		MeetID: meet.ID, SeasonID: season.ID, Role: "timer", Title: "Lane timer",
		StartsAt: start, EndsAt: start.Add(2 * time.Hour), Capacity: 3, Location: "Pool deck",
	})
	if err != nil {
		t.Fatalf("create job with wired meet and season lookups: %v", err)
	}
	if _, err := svc.CreateJob(ctx, admin, org, volunteer.JobInput{
		MeetID: uuid.NewString(), Role: "timer", Title: "Ghost meet", StartsAt: start, EndsAt: start.Add(time.Hour), Capacity: 1,
	}); !errors.Is(err, volunteer.ErrMeetNotFound) {
		t.Fatalf("unknown meet must be rejected through the meet lookup, got %v", err)
	}
	if _, err := svc.CreateJob(ctx, admin, org, volunteer.JobInput{
		EventName: "x", SeasonID: uuid.NewString(), Role: "timer", Title: "Ghost season", StartsAt: start, EndsAt: start.Add(time.Hour), Capacity: 1,
	}); !errors.Is(err, volunteer.ErrSeasonNotFound) {
		t.Fatalf("unknown season must be rejected through the season lookup, got %v", err)
	}

	// The swimmer lookup resolves a real swimmer, and rejects one from another household.
	if _, err := svc.SignUp(ctx, admin, org, job.ID, volunteer.SignupInput{HouseholdID: otherHH.ID, VolunteerName: "Intruder", SwimmerID: sw.ID}); err == nil || !strings.Contains(err.Error(), "swimmer is not in this household") {
		t.Fatalf("foreign swimmer should be rejected, got %v", err)
	}
	if _, err := svc.SignUp(ctx, admin, org, job.ID, volunteer.SignupInput{HouseholdID: hh.ID, VolunteerName: "Ghost", SwimmerID: uuid.NewString()}); err == nil || !strings.Contains(err.Error(), "swimmer is not in this household") {
		t.Fatalf("unknown swimmer should be rejected, got %v", err)
	}
	if _, err := svc.SignUp(ctx, admin, org, job.ID, volunteer.SignupInput{HouseholdID: uuid.NewString(), VolunteerName: "Nobody"}); !errors.Is(err, volunteer.ErrHouseholdNotFound) {
		t.Fatalf("unknown household should be rejected through the directory, got %v", err)
	}
	signup, err := svc.SignUp(ctx, admin, org, job.ID, volunteer.SignupInput{HouseholdID: hh.ID, VolunteerName: "Ann Reminder", SwimmerID: sw.ID})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if signup.HouseholdName != "Reminder Family" {
		t.Fatalf("household name not resolved through the directory: %q", signup.HouseholdName)
	}

	pending, err := queue.ListTasks(workers.QueueEmail, workers.StatusPending)
	if err != nil {
		t.Fatal(err)
	}
	reminders := 0
	for _, task := range pending {
		if task.Type == volunteer.TaskTypeShiftReminder {
			reminders++
		}
	}
	if reminders != 1 {
		t.Fatalf("expected exactly one queued shift reminder, got %d of %+v", reminders, pending)
	}

	sent, reason, err := svc.SendShiftReminder(ctx, volunteer.ShiftReminderPayload{OrgID: org, SignupID: signup.ID, StartsAtUnix: start.Unix()})
	if err != nil || !sent || reason != "sent" {
		t.Fatalf("reminder: sent=%v reason=%q err=%v", sent, reason, err)
	}
	msgs := mail.Sent()
	if len(msgs) != 1 {
		t.Fatalf("expected one email, got %d", len(msgs))
	}
	m := msgs[0]
	if m.Req.To != "family@example.test" || m.Req.Tag != "volunteer-reminder" ||
		!strings.Contains(m.Req.Subject, "Lane timer") || !strings.Contains(m.Req.TextBody, "Event: Fall Invitational") {
		t.Fatalf("unexpected reminder email: %+v", m.Req)
	}
}

// TestCov1xWireVolunteerLookupFailuresSurface proves that a failing season or
// household source is reported to callers instead of being treated as "empty".
func TestCov1xWireVolunteerLookupFailuresSurface(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	admin := cov1xAdmin(org)
	ctx := context.Background()

	membersSvc := members.NewServiceWithPool(pool)
	seasonsSvc := seasons.NewServiceWithPool(pool)
	_, svc := wireVolunteer(volunteerDeps{pool: pool, members: membersSvc, seasons: seasonsSvc})

	start := time.Now().UTC().Add(72 * time.Hour)
	input := volunteer.JobInput{EventName: "Swim-a-thon", Role: "timer", Title: "Timer", StartsAt: start, EndsAt: start.Add(time.Hour), Capacity: 2}
	job, err := svc.CreateJob(ctx, admin, org, input)
	if err != nil {
		t.Fatalf("a DB-backed job should be created: %v", err)
	}
	// Persisted through Postgres: a second wiring on the same pool sees it.
	_, other := wireVolunteer(volunteerDeps{pool: pool})
	if got, err := other.GetJob(ctx, admin, org, job.ID); err != nil || got.Title != "Timer" {
		t.Fatalf("job not visible across instances: %+v %v", got, err)
	}

	// Break only the sources the lookups read from (renaming a column makes every
	// load of the table fail; dropping the table would fall through to public.*).
	for _, q := range []string{
		`ALTER TABLE seasons RENAME COLUMN name TO name_gone`,
		`ALTER TABLE households RENAME COLUMN household_name TO household_name_gone`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	input.SeasonID = uuid.NewString()
	if _, err := svc.CreateJob(ctx, admin, org, input); !errors.Is(err, volunteer.ErrSeasonNotFound) {
		t.Fatalf("a season source failure must not let an unverified season through, got %v", err)
	}
	if _, err := svc.SignUp(ctx, admin, org, job.ID, volunteer.SignupInput{HouseholdID: uuid.NewString(), VolunteerName: "X"}); err == nil || errors.Is(err, volunteer.ErrHouseholdNotFound) {
		t.Fatalf("a household source failure must surface as an error, got %v", err)
	}
}

func cov1xBuyoutRequest(org string, p *appmiddleware.Principal) volunteer.BuyoutInvoiceRequest {
	return volunteer.BuyoutInvoiceRequest{OrgID: org, HouseholdID: "hh-1", Description: "Buyout", AmountCents: 5000, DueDate: "2027-01-31", Issue: true, Principal: p}
}

func TestCov1xVolunteerBillerSuccessAndFailures(t *testing.T) {
	t.Parallel()
	const org = "org-cov1x-buyout"
	admin := cov1xAdmin(org)
	svc := billing.NewService()
	b := volunteerBiller{billing: svc}

	res, err := b.CreateBuyoutInvoice(context.Background(), cov1xBuyoutRequest(org, admin))
	if err != nil {
		t.Fatal(err)
	}
	if res.InvoiceStatus != "issued" || !strings.HasPrefix(res.InvoiceNumber, "VOL-") || res.InvoiceItemID == "" {
		t.Fatalf("unexpected result %+v", res)
	}
	draft := cov1xBuyoutRequest(org, admin)
	draft.Issue = false
	if res, err := b.CreateBuyoutInvoice(context.Background(), draft); err != nil || res.InvoiceStatus != "draft" {
		t.Fatalf("draft buyout: %+v %v", res, err)
	}

	// A non-admin is refused by billing and surfaces as the volunteer domain's ErrForbidden.
	parent := &appmiddleware.Principal{UserID: "parent-1", OrgID: org, Roles: []string{"parent"}, HouseholdID: "hh-1"}
	if _, err := b.CreateBuyoutInvoice(context.Background(), cov1xBuyoutRequest(org, parent)); !errors.Is(err, volunteer.ErrForbidden) {
		t.Fatalf("expected volunteer.ErrForbidden, got %v", err)
	}

	// Bad due date: the invoice is never created.
	bad := cov1xBuyoutRequest(org, admin)
	bad.DueDate = "not-a-date"
	if _, err := b.CreateBuyoutInvoice(context.Background(), bad); !errors.Is(err, billing.ErrInvalidFinanceInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}

	// An absurd amount fails at the line item, and the draft is discarded.
	before := len(svc.SnapshotInvoices(org))
	huge := cov1xBuyoutRequest(org, admin)
	huge.AmountCents = 2_000_000_000
	_, err = b.CreateBuyoutInvoice(context.Background(), huge)
	if !errors.Is(err, billing.ErrInvalidFinanceInput) || !strings.Contains(err.Error(), "add item to draft invoice") {
		t.Fatalf("expected add-item failure, got %v", err)
	}
	invs := svc.SnapshotInvoices(org)
	if len(invs) != before+1 {
		t.Fatalf("expected the failed draft to remain as one voided record, got %d vs %d", len(invs), before)
	}
	voided := 0
	for _, inv := range invs {
		if inv.Status == "voided" {
			voided++
		}
	}
	if voided != 1 {
		t.Fatalf("the failed buyout's draft must be voided, got %d voided", voided)
	}
}

func TestCov1xVolunteerBillerKeepsDraftWhenDiscardFailsAndReturnsOriginalError(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org := uuid.NewString()
	hh := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'Buyout Family')`, hh, org); err != nil {
		t.Fatal(err)
	}
	// Once an invoice exists, any further update to it fails, so adding the item
	// fails and so does voiding the draft that is left behind.
	for _, q := range []string{
		`CREATE FUNCTION cov1x_block_invoice_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'invoice is frozen'; END $$`,
		`CREATE TRIGGER cov1x_freeze BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION cov1x_block_invoice_update()`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	b := volunteerBiller{billing: billing.NewServiceWithPool(pool)}
	req := volunteer.BuyoutInvoiceRequest{OrgID: org, HouseholdID: hh, Description: "Buyout", AmountCents: 5000, DueDate: "2027-01-31", Issue: false, Principal: cov1xAdmin(org)}
	_, err := b.CreateBuyoutInvoice(ctx, req)
	if err == nil || !strings.Contains(err.Error(), "add item to draft invoice") {
		t.Fatalf("expected add-item failure, got %v", err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM invoices WHERE org_id = $1`, org).Scan(&status); err != nil || status != "draft" {
		t.Fatalf("the undiscardable draft should still be a draft row: %q %v", status, err)
	}
}
