package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/documents"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov12gActivityFeedForSwimmerUnknownToMembers(t *testing.T) {
	t.Parallel()
	w := newCov3xPerfWorld(t)
	got, err := w.h.buildActivityFeed(cov3xOrg, w.admin, "swimmer-not-in-members")
	if err != nil {
		t.Fatalf("unknown swimmer should yield an empty feed, got error %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no activity, got %+v", got)
	}
}

func TestCov12gActivityFeedNamesMeetWithBlankName(t *testing.T) {
	t.Parallel()
	pool := testutil.IsolatedSchemaPool(t)
	cov3xInsertUser(t, pool, cov3xOrg, cov3xAdminID)
	admin := &appmiddleware.Principal{UserID: cov3xAdminID, OrgID: cov3xOrg, Roles: []string{"admin"}}
	mem := members.NewServiceWithPool(pool)
	hh, err := mem.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov3xOrg, HouseholdName: "Blank", PrimaryEmail: "blank@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := mem.CreateSwimmer(admin, cov3xOrg, members.CreateSwimmerInput{
		HouseholdID: hh.ID, FirstName: "Bea", LastName: "Blank", DateOfBirth: "2012-04-05", Gender: "female", RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	mt := meets.NewServiceWithPool(pool)
	mt.SeedSwimmer(cov3xOrg, sw.ID, hh.ID)
	meet, _, err := mt.CreateMeet(admin, cov3xOrg, meets.CreateMeetInput{Name: "Named", MeetType: "invitational", Course: "SCY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := mt.TransitionMeetStatus(admin, cov3xOrg, meet.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := mt.PublishCommitmentRequest(admin, cov3xOrg, meet.ID); err != nil {
		t.Fatal(err)
	}
	// A meet that lost its name (legacy/imported row) must still render a usable sentence.
	if _, err := pool.Exec(context.Background(), `UPDATE meets SET name = ''`); err != nil {
		t.Fatal(err)
	}

	w := newCov3xPerfWorld(t)
	r := w.routerWith(w.perf, mem, mt, documents.NewService())
	path := "/swimmers/" + sw.ID + "/activity"
	// The first read reloads the meet from the database; the next sees the blank name.
	cov3xWant(t, cov3xDo(r, http.MethodGet, path, "admin", ""), http.StatusOK, "activities")
	rr := cov3xDo(r, http.MethodGet, path, "admin", "")
	cov3xWant(t, rr, http.StatusOK, "Response recorded for meet.")
}

// TestCov12gRefundPersistenceFailureWithoutStripe covers a local refund write
// that fails after preflight passed and no provider refund was issued: the
// failure maps to a plain 500 and nothing is recorded.
func TestCov12gRefundPersistenceFailureWithoutStripe(t *testing.T) {
	t.Parallel()
	pool := testutil.IsolatedSchemaPool(t)
	org, hh := uuid.NewString(), uuid.NewString()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'Refund Family')`, hh, org); err != nil {
		t.Fatal(err)
	}
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
	svc := billing.NewServiceWithPool(pool)
	inv, err := svc.CreateInvoice(org, admin.UserID, admin, billing.CreateInvoiceInput{HouseholdID: hh, InvoiceNumber: "INV-R12"})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, err := svc.AddInvoiceItem(org, admin.UserID, admin, inv.ID, billing.AddInvoiceItemInput{Description: "dues", Quantity: 1, UnitAmountCents: 3000, Category: "manual"}); err != nil {
		t.Fatalf("AddInvoiceItem: %v", err)
	}
	if _, _, err := svc.IssueInvoice(org, admin.UserID, admin, inv.ID); err != nil {
		t.Fatalf("IssueInvoice: %v", err)
	}
	pay, _, err := svc.RecordPayment(org, admin.UserID, admin, inv.ID, billing.RecordPaymentInput{AmountCents: 3000, Method: "check"})
	if err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}
	// Reads keep working but inserting a refund row is rejected.
	if _, err := pool.Exec(ctx, `CREATE FUNCTION cov12g_block_refunds() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'refunds are read-only'; END $$;
		CREATE TRIGGER cov12g_block_refunds BEFORE INSERT ON refunds FOR EACH ROW EXECUTE FUNCTION cov12g_block_refunds()`); err != nil {
		t.Fatal(err)
	}

	h := NewBillingHandlersWithAuditAndStripe(svc, nil, stripelib.NewStubClient())
	router := chi.NewRouter()
	router.Use(appmiddleware.Auth(fixedPrincipalResolver{admin}), appmiddleware.RequireOrgScope)
	h.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/invoices/"+inv.ID+"/refunds",
		strings.NewReader(`{"payment_id":"`+pay.ID+`","amount_cents":1000,"reason":"goodwill"}`))
	req.Header.Set("Authorization", "Bearer t")
	req.Header.Set("X-Org-Id", org)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "read-only") || strings.Contains(rec.Body.String(), "refund_persistence_failed") {
		t.Fatalf("unexpected body (cause leaked or wrong branch): %s", rec.Body.String())
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM refunds`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("no refund may be recorded: n=%d err=%v", n, err)
	}
}
