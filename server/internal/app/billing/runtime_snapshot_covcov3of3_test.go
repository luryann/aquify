package billing

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestCov3xRuntimeSnapshotRoundTrip(t *testing.T) {
	src := NewService()
	org, hh := "org-cov3x-snap", "hh-cov3x-snap"
	p := cov3xAdmin(org)

	inv := cov3xInvoice(t, src, org, hh, "SNAP-1", "2026-12-01", 12000)
	pay, _, err := src.RecordPayment(org, p.UserID, p, inv.ID, RecordPaymentInput{AmountCents: 5000, Method: "check", ExternalRef: "chk-77"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := src.CreateRefund(org, p.UserID, p, inv.ID, CreateRefundInput{PaymentID: pay.ID, AmountCents: 1000, Reason: "partial"}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.UpsertHouseholdAutopay(org, p.UserID, p, hh, UpsertHouseholdAutopayInput{PaymentMethodType: "card", PaymentMethodBrand: "visa", PaymentMethodLast4: "4242", PaymentMethodLabel: "Visa 4242", TriggerDaysBeforeDue: 3}); err != nil {
		t.Fatal(err)
	}
	if isNew, err := src.RecordWebhookEvent("stripe", org, "evt_cov3x_snap", "payment_intent.succeeded", json.RawMessage(`{"id":"evt_cov3x_snap"}`)); err != nil || !isNew {
		t.Fatalf("webhook event: %v %v", isNew, err)
	}

	data, err := src.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(data, &shape); err != nil {
		t.Fatalf("snapshot must be one JSON object: %v", err)
	}
	for _, key := range []string{"invoices", "items", "payments", "refunds", "ledger", "webhook_events", "autopay_profiles"} {
		if _, ok := shape[key]; !ok {
			t.Fatalf("snapshot is missing %q: %s", key, data)
		}
	}

	dst := NewService()
	if err := dst.LoadRuntimeSnapshot(data); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(src.SnapshotInvoices(org), dst.SnapshotInvoices(org)) ||
		!reflect.DeepEqual(src.SnapshotInvoiceItems(org), dst.SnapshotInvoiceItems(org)) ||
		!reflect.DeepEqual(src.SnapshotPayments(org), dst.SnapshotPayments(org)) ||
		!reflect.DeepEqual(src.SnapshotRefunds(org), dst.SnapshotRefunds(org)) ||
		!reflect.DeepEqual(src.SnapshotLedgerEvents(org), dst.SnapshotLedgerEvents(org)) {
		t.Fatal("restored service must hold exactly the exported records")
	}
	got, err := dst.GetInvoice(org, p, inv.ID)
	if err != nil || got.AmountPaidCents != 4000 || got.Status != "refunded_partial" {
		t.Fatalf("restored invoice: %+v %v", got, err)
	}
	if events, err := dst.ListWebhookEvents(org, p); err != nil || len(events) != 1 || events[0].EventID != "evt_cov3x_snap" {
		t.Fatalf("restored webhook events: %+v %v", events, err)
	}
	if st, err := dst.GetHouseholdAutopayStatus(org, p, hh); err != nil || !st.Enabled || st.PaymentMethodLast4 != "4242" {
		t.Fatalf("restored autopay: %+v %v", st, err)
	}
	if dst.paymentsByID[pay.ID] == nil || !dst.invoiceNums[invoiceNumKey(org, "SNAP-1")] {
		t.Fatal("derived indexes were not rebuilt")
	}

	// The snapshot is a deep copy: mutating the source afterwards must not leak.
	src.mu.Lock()
	src.invoices[inv.ID].Notes = "changed after export"
	src.mu.Unlock()
	if again, _ := dst.GetInvoice(org, p, inv.ID); again.Notes != "" {
		t.Fatalf("restored state aliases the source: %+v", again)
	}
}

func TestCov3xLoadRuntimeSnapshotEmptyAndInvalid(t *testing.T) {
	svc := NewService()
	org := "org-cov3x-snap2"
	p := cov3xAdmin(org)
	inv := cov3xInvoice(t, svc, org, "hh-1", "SNAP-KEEP", "", 700)

	for _, empty := range [][]byte{nil, {}} {
		if err := svc.LoadRuntimeSnapshot(empty); err != nil {
			t.Fatalf("empty snapshot is a no-op: %v", err)
		}
	}
	err := svc.LoadRuntimeSnapshot([]byte(`{"invoices": [1,2`))
	var syn *json.SyntaxError
	if err == nil || !errors.As(err, &syn) {
		t.Fatalf("malformed snapshot must return the JSON error, got %v", err)
	}
	err = svc.LoadRuntimeSnapshot([]byte(`{"invoices": "nope"}`))
	var typeErr *json.UnmarshalTypeError
	if err == nil || !errors.As(err, &typeErr) {
		t.Fatalf("wrongly typed snapshot must be rejected, got %v", err)
	}
	// Failed and empty loads leave the live state untouched.
	if got, err := svc.GetInvoice(org, p, inv.ID); err != nil || got.InvoiceNumber != "SNAP-KEEP" {
		t.Fatalf("state after rejected snapshots: %+v %v", got, err)
	}
}
