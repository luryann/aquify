package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/meets"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/training"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov1yFinishRSVPInvoiceReportsAMissingInvoiceAndTheFailedCleanup(t *testing.T) {
	t.Parallel()
	b := rsvpFeeBiller{billing: billing.NewService()}
	_, err := b.finishRSVPInvoice(cov1xOrg, cov1xCharge(), engagementSystemPrincipal(cov1xOrg), "no-such-invoice")
	if err == nil || !strings.Contains(err.Error(), "list rsvp invoice items") || !strings.Contains(err.Error(), "void rsvp invoice no-such-invoice") {
		t.Fatalf("expected both the listing failure and the failed compensating void, got %v", err)
	}
}

func TestCov1yReleaseChargeKeepsInvoicesWithMoneyAndSurfacesVoidFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org, hh, sw := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'Release Family')`, []any{hh, org}},
		{`INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender) VALUES ($1, $2, $3, 'R', 'F', '2013-01-01', 'female')`, []any{sw, org, hh}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	c := cov1xCharge()
	c.EventID, c.HouseholdID, c.SwimmerID, c.ActorUserID = uuid.NewString(), hh, sw, ""
	invoiceID, err := rsvpFeeBiller{billing: billing.NewServiceWithPool(pool)}.ChargeRSVP(org, c)
	if err != nil {
		t.Fatal(err)
	}
	// Each phase reads through a fresh service so it sees exactly what Postgres holds.
	release := func() (bool, error) {
		return rsvpFeeBiller{billing: billing.NewServiceWithPool(pool)}.ReleaseCharge(org, invoiceID, "")
	}
	status := func() string {
		var s string
		if err := pool.QueryRow(ctx, `SELECT status FROM invoices WHERE id = $1`, invoiceID).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	// A payment is on record but the invoice row still says "issued" (e.g. the
	// status update was lost): the invoice must not be voided out from under it.
	sys := engagementSystemPrincipal(org)
	if _, _, err := billing.NewServiceWithPool(pool).RecordPayment(org, "", sys, invoiceID, billing.RecordPaymentInput{AmountCents: 500, Method: "cash"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE invoices SET status = 'issued' WHERE id = $1`, invoiceID); err != nil {
		t.Fatal(err)
	}
	if released, err := release(); err != nil || released {
		t.Fatalf("an invoice holding payments must be kept: released=%v err=%v", released, err)
	}
	if got := status(); got != "issued" {
		t.Fatalf("invoice status changed to %q", got)
	}

	// Unpaid again, but the database refuses to update the invoice: the failure is
	// returned, never reported as a successful release.
	if _, err := pool.Exec(ctx, `DELETE FROM payments WHERE invoice_id = $1`, invoiceID); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE FUNCTION cov1y_block_invoice_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'invoice is frozen'; END $$`,
		`CREATE TRIGGER cov1y_freeze BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION cov1y_block_invoice_update()`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if released, err := release(); err == nil || released || !strings.Contains(err.Error(), "invoice is frozen") {
		t.Fatalf("void failure must surface: released=%v err=%v", released, err)
	}
	if got := status(); got != "issued" {
		t.Fatalf("a failed void must leave the invoice issued, is %q", got)
	}
}

func TestCov1yTrainingPracticeSourceReportsStorageFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	src := trainingPracticeSource{training: training.NewServiceWithPool(cov1xClosedCopy(t, pool))}
	if got, err := src.Practices(uuid.NewString()); err == nil {
		t.Fatalf("Practices on a closed database returned %v", got)
	}
	if got, err := src.Attendance(uuid.NewString(), uuid.NewString()); err == nil {
		t.Fatalf("Attendance on a closed database returned %v", got)
	}
}

func TestCov1yCalendarFeedSourcePropagatesServiceFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	closed := cov1xClosedCopy(t, pool)
	from, to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC)
	org := uuid.NewString()

	meetsDown := buildCalendarFeedSource(meets.NewServiceWithPool(closed), nil, nil, nil)
	if items, err := meetsDown(org, nil, from, to); err == nil {
		t.Fatalf("meet listing failure was swallowed: %v", items)
	}
	trainingDown := buildCalendarFeedSource(nil, training.NewServiceWithPool(closed), nil, nil)
	if items, err := trainingDown(org, nil, from, to); err == nil {
		t.Fatalf("practice listing failure was swallowed: %v", items)
	}
	// With neither domain wired the feed is simply empty, not an error.
	if items, err := buildCalendarFeedSource(nil, nil, nil, nil)(org, nil, from, to); err != nil || len(items) != 0 {
		t.Fatalf("empty feed: %v %v", items, err)
	}
}

// Deadlines that carry a time come back from the meet service as RFC 3339 timestamps; the feed must still publish a date-only deadline entry.
func TestCov1yCalendarFeedTrimsTimestampDeadlinesFromPostgres(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org := uuid.NewString()
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, org_id, email, oidc_subject) VALUES ($1, $2, $3, $4)`, admin.UserID, org, admin.UserID+"@example.test", admin.UserID); err != nil {
		t.Fatal(err)
	}
	svc := meets.NewServiceWithPool(pool)
	meet, _, err := svc.CreateMeet(admin, org, meets.CreateMeetInput{Name: "Spring Fling", StartDate: "2027-03-20", CommitmentDeadline: "2027-03-10T17:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.TransitionMeetStatus(admin, org, meet.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.PublishCommitmentRequest(admin, org, meet.ID); err != nil {
		t.Fatal(err)
	}
	listed, err := svc.ListMeets(admin, org)
	if err != nil || len(listed) != 1 || len(listed[0].CommitmentDeadline) <= 10 {
		t.Fatalf("precondition: Postgres should return a timestamp deadline, got %+v %v", listed, err)
	}

	items, err := buildCalendarFeedSource(svc, nil, nil, nil)(org, nil, time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, it := range items {
		got[it.Title] = it.StartDate + "/" + it.EndDate
	}
	if got["Spring Fling"] != "2027-03-20/2027-03-20" || got["Commitment deadline — Spring Fling"] != "2027-03-10/2027-03-10" {
		t.Fatalf("feed items = %v", got)
	}
}
