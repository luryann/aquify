package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/workers"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// BillingAutomationPayload is the payload for TypeBillingRecurringRun and
// TypeBillingLateFees. Date (YYYY-MM-DD) defaults to today (UTC).
type BillingAutomationPayload struct {
	OrgID       string `json:"org_id"`
	Date        string `json:"date,omitempty"`
	TriggeredBy string `json:"triggered_by,omitempty"`
}

func decodeBillingAutomation(task *workers.Task, job string) (BillingAutomationPayload, time.Time, error) {
	var p BillingAutomationPayload
	if err := json.Unmarshal(task.Payload, &p); err != nil {
		return p, time.Time{}, fmt.Errorf("%s: decode payload: %w", job, err)
	}
	if p.OrgID == "" {
		return p, time.Time{}, fmt.Errorf("%s: missing org_id", job)
	}
	day := time.Now().UTC()
	if p.Date != "" {
		parsed, err := time.Parse("2006-01-02", p.Date)
		if err != nil {
			return p, time.Time{}, fmt.Errorf("%s: invalid date %q: %w", job, p.Date, err)
		}
		day = parsed
	}
	return p, day, nil
}

// BillingRecurringRunHandler commits the org's automated billing run (auto-bill
// fee schedules) when billing automation is enabled. Idempotent: charge keys
// make re-runs bill only new, unbilled periods/joiners.
func BillingRecurringRunHandler(logger *slog.Logger, billingService *billing.Service) workers.Handler {
	tracer := otel.Tracer(tracerNameBilling)
	return func(ctx context.Context, task *workers.Task) error {
		_, span := tracer.Start(ctx, "job.billing_recurring_run")
		defer span.End()
		p, day, err := decodeBillingAutomation(task, "billing recurring run")
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "decode payload failed")
			return err
		}
		if billingService == nil {
			return fmt.Errorf("billing recurring run: billing service is not configured")
		}
		span.SetAttributes(attribute.String("org.id", p.OrgID), attribute.String("billing.date", day.Format("2006-01-02")))
		started := time.Now()
		result, err := billingService.RunScheduledBilling(p.OrgID, day)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "billing run failed")
			logger.Error("billing recurring run: failed", "org_id", p.OrgID, "error", err)
			return fmt.Errorf("billing recurring run: %w", err)
		}
		if result == nil {
			logger.Info("billing recurring run: nothing to bill", "org_id", p.OrgID, "date", day.Format("2006-01-02"))
			span.SetStatus(codes.Ok, "")
			return nil
		}
		span.SetAttributes(
			attribute.Int("billing.invoice_count", result.Run.InvoiceCount),
			attribute.Int("billing.net_cents", result.Run.NetCents),
			attribute.Int("billing.skipped_count", result.Run.SkippedCount),
		)
		logger.Info("billing recurring run: completed", "org_id", p.OrgID, "run_id", result.Run.ID,
			"invoices", result.Run.InvoiceCount, "charges", result.Run.ChargeCount, "net_cents", result.Run.NetCents,
			"skipped", result.Run.SkippedCount, "duration_ms", time.Since(started).Milliseconds())
		span.SetStatus(codes.Ok, "")
		return nil
	}
}

// BillingLateFeesHandler runs the org's late-fee sweep (one fee per invoice).
func BillingLateFeesHandler(logger *slog.Logger, billingService *billing.Service) workers.Handler {
	tracer := otel.Tracer(tracerNameBilling)
	return func(ctx context.Context, task *workers.Task) error {
		_, span := tracer.Start(ctx, "job.billing_late_fees")
		defer span.End()
		p, day, err := decodeBillingAutomation(task, "billing late fees")
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "decode payload failed")
			return err
		}
		if billingService == nil {
			return fmt.Errorf("billing late fees: billing service is not configured")
		}
		span.SetAttributes(attribute.String("org.id", p.OrgID), attribute.String("billing.date", day.Format("2006-01-02")))
		result, err := billingService.RunScheduledLateFees(p.OrgID, day)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "late fee sweep failed")
			logger.Error("billing late fees: failed", "org_id", p.OrgID, "error", err)
			return fmt.Errorf("billing late fees: %w", err)
		}
		span.SetAttributes(attribute.Int("billing.late_fee_count", result.AppliedCount), attribute.Int("billing.late_fee_cents", result.TotalCents))
		logger.Info("billing late fees: completed", "org_id", p.OrgID, "enabled", result.Enabled,
			"applied", result.AppliedCount, "total_cents", result.TotalCents)
		span.SetStatus(codes.Ok, "")
		return nil
	}
}
