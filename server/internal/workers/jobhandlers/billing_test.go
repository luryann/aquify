package jobhandlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
	"github.com/dareaquatics/aquify/server/internal/workers"
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
