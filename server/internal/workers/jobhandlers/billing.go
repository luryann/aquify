package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/dareaquatics/aquify/server/internal/workers"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const tracerNameBilling = "github.com/dareaquatics/aquify/server/internal/workers/jobhandlers/billing"

// BillingReconcilePayload is the JSON payload for the TypeBillingReconcile task.
type BillingReconcilePayload struct {
	OrgID       string `json:"org_id"`
	TriggeredBy string `json:"triggered_by,omitempty"` // "scheduler" or actor user ID
}

// NightlyIntegrityPayload is the JSON payload for the TypeNightlyIntegrity task.
type NightlyIntegrityPayload struct {
	OrgID string `json:"org_id"`
	Date  string `json:"date"` // YYYY-MM-DD
}

// BillingReconcileHandler returns a Handler that runs the nightly Stripe ↔ ledger
// reconciliation and writes an alert if the mismatch exceeds the configured threshold.
func BillingReconcileHandler(logger *slog.Logger) workers.Handler {
	tracer := otel.Tracer(tracerNameBilling)

	return func(ctx context.Context, task *workers.Task) error {
		ctx, span := tracer.Start(ctx, "job.billing_reconcile")
		defer span.End()

		var p BillingReconcilePayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "decode payload failed")
			return fmt.Errorf("billing reconcile: decode payload: %w", err)
		}

		span.SetAttributes(
			attribute.String("org.id", p.OrgID),
			attribute.String("job.triggered_by", p.TriggeredBy),
		)

		logger.Info("billing reconcile: starting",
			"org_id", p.OrgID,
			"triggered_by", p.TriggeredBy,
		)

		// Scaffold: simulate reconciliation work.
		// Production: call stripe.Client.FetchBalanceCents() →
		// sum billing.Service.LedgerEvents(method=stripe) →
		// compare; if abs(diff) > threshold → emit alert to slog + metrics.
		select {
		case <-ctx.Done():
			span.RecordError(ctx.Err())
			span.SetStatus(codes.Error, "context cancelled")
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}

		span.SetStatus(codes.Ok, "")
		logger.Info("billing reconcile: completed", "org_id", p.OrgID)
		return nil
	}
}

// NightlyIntegrityHandler returns a Handler that checks referential integrity
// and data consistency (e.g. orphaned ledger entries, unpaid invoices that
// are past due, commitments past deadline without a response).
func NightlyIntegrityHandler(logger *slog.Logger) workers.Handler {
	tracer := otel.Tracer(tracerNameBilling)

	return func(ctx context.Context, task *workers.Task) error {
		ctx, span := tracer.Start(ctx, "job.nightly_integrity")
		defer span.End()

		var p NightlyIntegrityPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "decode payload failed")
			return fmt.Errorf("nightly integrity: decode payload: %w", err)
		}

		span.SetAttributes(
			attribute.String("org.id", p.OrgID),
			attribute.String("integrity.date", p.Date),
		)

		logger.Info("nightly integrity: starting",
			"org_id", p.OrgID,
			"date", p.Date,
		)

		// Scaffold: simulate checks.
		// Production checks include:
		//   - Invoice items that reference deleted swimmers
		//   - Orphaned best_times rows where swimmer no longer exists
		//   - Import jobs stuck in "scanning" > 1 hour
		//   - Expired data-export requests not transitioned to "expired"
		//   - Webhook events stuck in "processing" > 30 min
		select {
		case <-ctx.Done():
			span.RecordError(ctx.Err())
			span.SetStatus(codes.Error, "context cancelled")
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}

		span.SetStatus(codes.Ok, "")
		logger.Info("nightly integrity: completed", "org_id", p.OrgID)
		return nil
	}
}
