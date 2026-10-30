package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
	"github.com/go-chi/chi/v5"
)

type fixedPrincipalResolver struct{ p *appmiddleware.Principal }

func (f fixedPrincipalResolver) ResolvePrincipal(string) (*appmiddleware.Principal, bool) {
	return f.p, true
}

// Two same-amount partial refunds of one payment must reach Stripe under
// different idempotency keys, otherwise Stripe replays the first refund.
func TestCreateRefundUsesDistinctStripeIdempotencyKeys(t *testing.T) {
	admin := &appmiddleware.Principal{UserID: "admin-1", OrgID: "org-1", Roles: []string{"admin"}, MFAAuthenticated: true}
	svc := billing.NewService()
	inv, err := svc.CreateInvoice("org-1", admin.UserID, admin, billing.CreateInvoiceInput{HouseholdID: "hh-1", InvoiceNumber: "INV-1"})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, err := svc.AddInvoiceItem("org-1", admin.UserID, admin, inv.ID, billing.AddInvoiceItemInput{Description: "dues", Quantity: 1, UnitAmountCents: 3000, Category: "manual"}); err != nil {
		t.Fatalf("AddInvoiceItem: %v", err)
	}
	if _, _, err := svc.IssueInvoice("org-1", admin.UserID, admin, inv.ID); err != nil {
		t.Fatalf("IssueInvoice: %v", err)
	}
	pay, _, err := svc.RecordPayment("org-1", admin.UserID, admin, inv.ID, billing.RecordPaymentInput{AmountCents: 3000, Method: "stripe", StripePaymentIntentID: "pi_1"})
	if err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}

	stub := stripelib.NewStubClient()
	h := NewBillingHandlersWithAuditAndStripe(svc, nil, stub)
	router := chi.NewRouter()
	router.Use(appmiddleware.Auth(fixedPrincipalResolver{admin}), appmiddleware.RequireOrgScope)
	h.RegisterRoutes(router)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/invoices/"+inv.ID+"/refunds",
			strings.NewReader(`{"payment_id":"`+pay.ID+`","amount_cents":1000,"reason":"partial"}`))
		req.Header.Set("Authorization", "Bearer t")
		req.Header.Set("X-Org-Id", "org-1")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("refund %d: status %d body %s", i, rec.Code, rec.Body.String())
		}
	}
	refunds := stub.CreatedRefunds()
	if len(refunds) != 2 {
		t.Fatalf("expected 2 stripe refunds, got %d", len(refunds))
	}
	if refunds[0].IdempotencyKey == refunds[1].IdempotencyKey {
		t.Fatalf("both refunds used idempotency key %q", refunds[0].IdempotencyKey)
	}
}
