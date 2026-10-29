package handlers

import (
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
)

func TestNotFoundWebhookRetriesOnlyWithinLagGrace(t *testing.T) {
	t.Parallel()

	now := time.Now().Unix()
	for _, err := range []error{billing.ErrPaymentNotFound, billing.ErrInvoiceNotFound} {
		if isPermanentWebhookError(err, now) {
			t.Fatalf("fresh %v must stay retryable for replication lag", err)
		}
		if !isPermanentWebhookError(err, now-int64((2*time.Hour).Seconds())) {
			t.Fatalf("stale %v must be terminal", err)
		}
		if isPermanentWebhookError(err, 0) {
			t.Fatalf("missing created timestamp must not force a terminal outcome for %v", err)
		}
	}
}

func TestPermanentWebhookErrorsIgnoreCreatedTime(t *testing.T) {
	t.Parallel()

	if !isPermanentWebhookError(billing.ErrRefundExceedsPayment, time.Now().Unix()) {
		t.Fatal("refund exceeding payment must be terminal regardless of age")
	}
}
