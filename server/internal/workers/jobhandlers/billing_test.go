package jobhandlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/luryann/aquify/server/internal/app/billing"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
	"github.com/luryann/aquify/server/internal/workers"
)

func TestBillingReconcileHandler_UsesBillingAndStripeServices(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	billingService := billing.NewService()
	stripeClient := &stripelib.StubClient{BalanceCents: 0}
	task, err := workers.NewTask(workers.TypeBillingReconcile, BillingReconcilePayload{
		OrgID:       "org-billing",
		TriggeredBy: "test",
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	handler := BillingReconcileHandler(logger, billingService, stripeClient, 100)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

func TestNightlyIntegrityHandler_ValidatesBillingSnapshots(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	billingService := billing.NewService()
	task, err := workers.NewTask(workers.TypeNightlyIntegrity, NightlyIntegrityPayload{
		OrgID: "org-billing",
		Date:  "2026-07-01",
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	handler := NightlyIntegrityHandler(logger, billingService)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

func TestNightlyIntegrityHandler_AllowsWrittenOffInvoiceWithPartialPayment(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := billing.NewService()
	p := &appmiddleware.Principal{UserID: "admin", OrgID: "org-integrity", Roles: []string{"admin"}}
	inv, err := svc.CreateInvoice(p.OrgID, p.UserID, p, billing.CreateInvoiceInput{InvoiceNumber: "WO-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddInvoiceItem(p.OrgID, p.UserID, p, inv.ID, billing.AddInvoiceItemInput{Description: "d", Quantity: 1, UnitAmountCents: 10000, Category: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.IssueInvoice(p.OrgID, p.UserID, p, inv.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RecordPayment(p.OrgID, p.UserID, p, inv.ID, billing.RecordPaymentInput{AmountCents: 4000, Method: "cash"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.WriteOffInvoice(p.OrgID, p.UserID, p, inv.ID); err != nil {
		t.Fatal(err)
	}
	task, err := workers.NewTask(workers.TypeNightlyIntegrity, NightlyIntegrityPayload{OrgID: p.OrgID, Date: "2026-07-01"})
	if err != nil {
		t.Fatal(err)
	}
	if err := NightlyIntegrityHandler(logger, svc)(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}
}
