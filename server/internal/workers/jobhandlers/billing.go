package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
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

// BillingReconcileHandler returns a Handler that runs Stripe-to-ledger
// reconciliation and logs an alert when mismatch exceeds the configured threshold.
func BillingReconcileHandler(logger *slog.Logger, billingService *billing.Service, stripeClient stripelib.Client, alertThresholdCents int) workers.Handler {
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
		if p.OrgID == "" {
			return fmt.Errorf("billing reconcile: missing org_id")
		}
		if billingService == nil {
			return fmt.Errorf("billing reconcile: billing service is not configured")
		}
		if stripeClient == nil {
			return fmt.Errorf("billing reconcile: stripe client is not configured")
		}

		span.SetAttributes(
			attribute.String("org.id", p.OrgID),
			attribute.String("job.triggered_by", p.TriggeredBy),
		)

		logger.Info("billing reconcile: starting",
			"org_id", p.OrgID,
			"triggered_by", p.TriggeredBy,
		)

		stripeTotalCents, err := stripeClient.FetchBalanceCents(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "stripe balance fetch failed")
			return fmt.Errorf("billing reconcile: fetch stripe balance: %w", err)
		}

		report, err := billingService.GetReconciliationReport(
			p.OrgID,
			workerBillingPrincipal(p.OrgID),
			stripeTotalCents,
			alertThresholdCents,
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "reconciliation failed")
			return fmt.Errorf("billing reconcile: compute report: %w", err)
		}

		span.SetAttributes(
			attribute.Int("billing.ledger_stripe_total_cents", report.LedgerStripeTotalCents),
			attribute.Int("billing.stripe_reported_cents", report.StripeReportedCents),
			attribute.Int("billing.mismatch_cents", report.MismatchCents),
			attribute.Bool("billing.alert_triggered", report.AlertTriggered),
		)
		if report.AlertTriggered {
			logger.Warn("billing reconcile: mismatch exceeds threshold",
				"org_id", p.OrgID,
				"ledger_stripe_total_cents", report.LedgerStripeTotalCents,
				"stripe_reported_cents", report.StripeReportedCents,
				"mismatch_cents", report.MismatchCents,
				"alert_threshold_cents", report.AlertThresholdCents,
			)
		}

		span.SetStatus(codes.Ok, "")
		logger.Info("billing reconcile: completed", "org_id", p.OrgID, "mismatch_cents", report.MismatchCents)
		return nil
	}
}

// NightlyIntegrityHandler returns a Handler that checks billing consistency for
// the configured organisation.
func NightlyIntegrityHandler(logger *slog.Logger, billingService *billing.Service) workers.Handler {
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
		if p.OrgID == "" {
			return fmt.Errorf("nightly integrity: missing org_id")
		}
		if billingService == nil {
			return fmt.Errorf("nightly integrity: billing service is not configured")
		}

		if p.Date != "" {
			if _, err := time.Parse("2006-01-02", p.Date); err != nil {
				return fmt.Errorf("nightly integrity: invalid date %q: %w", p.Date, err)
			}
		}

		span.SetAttributes(
			attribute.String("org.id", p.OrgID),
			attribute.String("integrity.date", p.Date),
		)

		logger.Info("nightly integrity: starting",
			"org_id", p.OrgID,
			"date", p.Date,
		)

		if err := validateBillingIntegrity(ctx, billingService, p.OrgID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "integrity check failed")
			return err
		}

		span.SetStatus(codes.Ok, "")
		logger.Info("nightly integrity: completed", "org_id", p.OrgID)
		return nil
	}
}

func validateBillingIntegrity(ctx context.Context, billingService *billing.Service, orgID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	invoices := billingService.SnapshotInvoices(orgID)
	items := billingService.SnapshotInvoiceItems(orgID)
	payments := billingService.SnapshotPayments(orgID)
	refunds := billingService.SnapshotRefunds(orgID)
	ledger := billingService.SnapshotLedgerEvents(orgID)

	invoiceIDs := make(map[string]billing.Invoice, len(invoices))
	for _, invoice := range invoices {
		invoiceIDs[invoice.ID] = invoice
		if invoice.TotalCents != invoice.SubtotalCents+invoice.TaxCents {
			return fmt.Errorf("nightly integrity: invoice %s total mismatch", invoice.ID)
		}
		if invoice.AmountDueCents != invoice.TotalCents-invoice.AmountPaidCents {
			return fmt.Errorf("nightly integrity: invoice %s balance mismatch", invoice.ID)
		}
	}

	for _, item := range items {
		if _, ok := invoiceIDs[item.InvoiceID]; !ok {
			return fmt.Errorf("nightly integrity: invoice item %s references missing invoice %s", item.ID, item.InvoiceID)
		}
		if item.TotalCents != item.Quantity*item.UnitAmountCents {
			return fmt.Errorf("nightly integrity: invoice item %s total mismatch", item.ID)
		}
	}

	paymentIDs := make(map[string]billing.Payment, len(payments))
	for _, payment := range payments {
		paymentIDs[payment.ID] = payment
		if _, ok := invoiceIDs[payment.InvoiceID]; !ok {
			return fmt.Errorf("nightly integrity: payment %s references missing invoice %s", payment.ID, payment.InvoiceID)
		}
	}

	refundIDs := make(map[string]billing.Refund, len(refunds))
	for _, refund := range refunds {
		refundIDs[refund.ID] = refund
		if _, ok := invoiceIDs[refund.InvoiceID]; !ok {
			return fmt.Errorf("nightly integrity: refund %s references missing invoice %s", refund.ID, refund.InvoiceID)
		}
		if _, ok := paymentIDs[refund.PaymentID]; !ok {
			return fmt.Errorf("nightly integrity: refund %s references missing payment %s", refund.ID, refund.PaymentID)
		}
	}

	for _, event := range ledger {
		if event.InvoiceID != "" {
			if _, ok := invoiceIDs[event.InvoiceID]; !ok {
				return fmt.Errorf("nightly integrity: ledger event %s references missing invoice %s", event.ID, event.InvoiceID)
			}
		}
		if event.PaymentID != "" {
			if _, ok := paymentIDs[event.PaymentID]; !ok {
				return fmt.Errorf("nightly integrity: ledger event %s references missing payment %s", event.ID, event.PaymentID)
			}
		}
		if event.RefundID != "" {
			if _, ok := refundIDs[event.RefundID]; !ok {
				return fmt.Errorf("nightly integrity: ledger event %s references missing refund %s", event.ID, event.RefundID)
			}
		}
		if math.Abs(float64(event.AmountCents)) == 0 {
			return fmt.Errorf("nightly integrity: ledger event %s has zero amount", event.ID)
		}
	}

	return nil
}

func workerBillingPrincipal(orgID string) *appmiddleware.Principal {
	return &appmiddleware.Principal{
		UserID: "worker:" + workers.TypeBillingReconcile,
		OrgID:  orgID,
		Roles:  []string{"admin"},
	}
}
